package images

import (
	"SIDM/internal/annotation"
	"SIDM/internal/catalog"
	"SIDM/internal/importer"
	"SIDM/internal/platform"
	"context"
	"maps"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// Memory é o módulo em memória, para os testes (deste e dos outros
// módulos). Os campos são exportados para os testes montarem o cenário
// direto; os *Err e BeforeReplace simulam falhas e concorrência.
type Memory struct {
	mu sync.Mutex

	Images  map[string]Image
	History []HistoryEntry

	LastFilter    ImageFilter // o filtro da última ListImages
	ListErr       error       // ListImages falha
	ReplaceErr    error       // ReplaceImage falha
	BeforeReplace func()      // roda antes de ReplaceImage (simula outra pessoa salvando no meio)
}

func NewMemory() *Memory {
	return &Memory{Images: map[string]Image{}}
}

// Update muda uma imagem com o lock (para os testes simularem outra pessoa
// gravando no meio).
func (m *Memory) Update(sha1 string, change func(img *Image)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	img := m.Images[sha1]
	change(&img)
	m.Images[sha1] = img
}

func paginate[T any](all []T, limit, skip int64) []T {
	if skip >= int64(len(all)) {
		return []T{}
	}
	return all[skip:min(skip+limit, int64(len(all)))]
}

// PackageProgress: as imagens de cada pacote pedido, por status.
func (m *Memory) PackageProgress(ctx context.Context, names []string) (map[string]catalog.Progress, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	out := map[string]catalog.Progress{}
	for _, img := range m.Images {
		if !want[img.Package] {
			continue
		}
		p := out[img.Package]
		p.Total++
		if img.Status == StatusVerified {
			p.Verified++
		} else {
			p.Pending++
			if img.ClaimedUntil != nil && img.ClaimedUntil.After(time.Now()) {
				p.InProgress++
			}
		}
		out[img.Package] = p
	}
	return out, nil
}

func (m *Memory) InsertImage(ctx context.Context, img *Image) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.Images[img.SHA1]; ok {
		return platform.ErrDuplicate
	}
	m.Images[img.SHA1] = *img
	return nil
}

func (m *Memory) DeleteImage(ctx context.Context, sha1 string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Images, sha1)
	return nil
}

func (m *Memory) DeleteImageHistory(ctx context.Context, sha1 string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := m.History[:0]
	for _, h := range m.History {
		if h.ImageID != sha1 {
			kept = append(kept, h)
		}
	}
	m.History = kept
	return nil
}

func (m *Memory) FindImage(ctx context.Context, sha1 string) (*Image, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	img, ok := m.Images[sha1]
	if !ok {
		return nil, platform.ErrNotFound
	}
	img.ensureValues()
	return &img, nil
}

func (m *Memory) ReplaceImage(ctx context.Context, img *Image, prevUpdatedAt time.Time) error {
	if m.BeforeReplace != nil {
		m.BeforeReplace() // simula outra pessoa gravando no meio
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ReplaceErr != nil {
		return m.ReplaceErr
	}
	cur, ok := m.Images[img.SHA1]
	if !ok {
		return platform.ErrNotFound
	}
	if !cur.UpdatedAt.Equal(prevUpdatedAt) {
		return ErrConflict
	}
	m.Images[img.SHA1] = *img
	return nil
}

// ClaimNext segue as mesmas regras do store de verdade (ver store/queue.go).
func (m *Memory) ClaimNext(ctx context.Context, pkg, user, skip string, lease time.Duration, now time.Time) (*Image, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if img, ok := m.Images[skip]; ok && img.Package == pkg && img.ClaimedBy == user {
		img.QueuedAt, img.ClaimedBy, img.ClaimedUntil = now, "", nil
		m.Images[skip] = img
	}
	for id, img := range m.Images {
		if img.Package == pkg && img.Status == StatusPending && img.ClaimedBy == user {
			img.ClaimedBy, img.ClaimedUntil = "", nil
			m.Images[id] = img
		}
	}

	best := m.oldestFree(pkg, now, skip)
	if best == nil && skip != "" {
		best = m.oldestFree(pkg, now, "") // a pulada era a única: ela volta
	}
	if best == nil {
		return nil, platform.ErrNotFound
	}
	until := now.Add(lease)
	best.ClaimedBy, best.ClaimedUntil = user, &until
	m.Images[best.SHA1] = *best
	out := *best
	out.ensureValues()
	return &out, nil
}

func (m *Memory) ReleaseClaim(ctx context.Context, sha1, user string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if img, ok := m.Images[sha1]; ok && img.ClaimedBy == user {
		img.ClaimedBy, img.ClaimedUntil = "", nil
		m.Images[sha1] = img
	}
	return nil
}

// ListImages filtra por status, pacote, equipamento e câmera (o suficiente para os testes da API;
// os filtros de verdade são testados contra o Mongo no pacote store).
func (m *Memory) ListImages(ctx context.Context, f ImageFilter) ([]Image, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LastFilter = f
	if m.ListErr != nil {
		return nil, 0, m.ListErr
	}
	out := []Image{}
	for _, img := range m.Images {
		if f.match(&img) {
			img.ensureValues()
			out = append(out, img)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SHA1 < out[j].SHA1 })
	if f.Limit > 0 {
		return paginate(out, f.Limit, f.Skip), int64(len(out)), nil
	}
	return out, int64(len(out)), nil
}

func (m *Memory) ExistingImages(ctx context.Context, ids []string) (map[string]ExistingImage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]ExistingImage{}
	for _, id := range ids {
		if img, ok := m.Images[id]; ok {
			out[id] = ExistingImage{Package: img.Package, Status: img.Status}
		}
	}
	return out, nil
}

func (m *Memory) InsertHistory(ctx context.Context, entries []HistoryEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.History = append(m.History, entries...)
	return nil
}

func (m *Memory) FindImagesByKeys(ctx context.Context, pkg, by string, keys []string) ([]Image, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	want := map[string]bool{}
	for _, k := range keys {
		want[k] = true
	}
	out := []Image{}
	for _, img := range m.Images {
		k := img.OriginalName
		if by == importer.MatchBySHA1 {
			k = img.SHA1
		}
		if (pkg == "" || img.Package == pkg) && want[k] {
			img.ensureValues()
			out = append(out, img)
		}
	}
	return out, nil
}

// matchFields: igual ao Mongo, numa lista basta conter o valor.
// match: a imagem passa no filtro (o mesmo que o toBSON do MongoRepo).
func (f ImageFilter) match(img *Image) bool {
	return (f.Status == "" || img.Status == f.Status) && (f.Package == "" || img.Package == f.Package) &&
		(len(f.Packages) == 0 || slices.Contains(f.Packages, img.Package)) &&
		matchFields(img.Fields, f.Fields) && matchObject(img.Objects, "", f.Object) && matchObjects(img.Objects, f.Objects)
}

func matchFields(have map[string]any, want map[string]any) bool {
	for k, v := range want {
		switch x := have[k].(type) {
		case []string:
			if s, ok := v.(string); !ok || !slices.Contains(x, s) {
				return false
			}
		case nil:
			return false
		default:
			if x != v {
				return false
			}
		}
	}
	return true
}

// matchObject: algum objeto (do tipo, se typ não é vazio) tem TODOS os
// valores ($elemMatch).
func matchObject(objs []annotation.Object, typ string, want map[string]any) bool {
	if len(want) == 0 && typ == "" {
		return true
	}
	for _, o := range objs {
		if (typ == "" || o.Type == typ) && matchFields(o.Fields, want) {
			return true
		}
	}
	return false
}

func matchObjects(objs []annotation.Object, want []ObjectFilter) bool {
	for _, w := range want {
		if !matchObject(objs, w.Type, w.Fields) {
			return false
		}
	}
	return true
}

func (m *Memory) CountUsing(ctx context.Context, reg catalog.Registry, name string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for _, img := range m.Images {
		if img.Fields[reg.ImageField()] == name {
			n++
		}
	}
	return n, nil
}

func (m *Memory) RenameOnPending(ctx context.Context, reg catalog.Registry, oldName, newName string, now time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for id, img := range m.Images {
		if img.Fields[reg.ImageField()] == oldName && img.Status == StatusPending {
			img.Fields = maps.Clone(img.Fields)
			img.Fields[reg.ImageField()], img.UpdatedAt = newName, now
			m.Images[id] = img
			n++
		}
	}
	return n, nil
}

func (m *Memory) RenamePackageImages(ctx context.Context, oldName, newName string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for id, img := range m.Images {
		if img.Package == oldName {
			img.Package = newName
			img.Path = "dataset/" + newName + "/" + strings.TrimPrefix(img.Path, "dataset/"+oldName+"/")
			m.Images[id] = img
			n++
		}
	}
	return n, nil
}

func (m *Memory) DeletePackageImages(ctx context.Context, name string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	deleted := map[string]bool{}
	for id, img := range m.Images {
		if img.Package == name {
			deleted[id] = true
			delete(m.Images, id)
		}
	}
	kept := m.History[:0]
	for _, h := range m.History {
		if !deleted[h.ImageID] {
			kept = append(kept, h)
		}
	}
	m.History = kept
	return int64(len(deleted)), nil
}

// VerifiedIDs: quais destes sha1 estão verificados.
func (m *Memory) VerifiedIDs(ctx context.Context, ids []string) (map[string]bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]bool{}
	for _, id := range ids {
		if m.Images[id].Status == StatusVerified {
			out[id] = true
		}
	}
	return out, nil
}

// oldestFree: a pendente livre mais antiga da fila, ignorando `except`.
func (m *Memory) oldestFree(pkg string, now time.Time, except string) *Image {
	var best *Image
	for _, img := range m.Images {
		img := img
		free := img.ClaimedUntil == nil || !img.ClaimedUntil.After(now)
		if img.Package != pkg || img.Status != StatusPending || !free || img.SHA1 == except {
			continue
		}
		if best == nil || img.QueuedAt.Before(best.QueuedAt) ||
			(img.QueuedAt.Equal(best.QueuedAt) && img.SHA1 < best.SHA1) {
			best = &img
		}
	}
	return best
}

// Stats faz a mesma conta do MongoRepo.Stats, imagem por imagem.
func (m *Memory) Stats(ctx context.Context, f ImageFilter, spec StatsSpec) (*Stats, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ListErr != nil {
		return nil, m.ListErr
	}
	st := emptyStats(spec)
	// cada valor de uma lista conta; um valor solto conta uma vez ($unwind)
	add := func(c Counts, v any) {
		switch x := v.(type) {
		case nil:
		case []string:
			for _, s := range x {
				c[s]++
			}
		case []any:
			for _, e := range x {
				c[countKey(e)]++
			}
		default:
			c[countKey(x)]++
		}
	}
	for _, img := range m.Images {
		if !f.match(&img) {
			continue
		}
		st.Total++
		st.Packages[img.Package]++
		st.Status[img.Status]++
		for _, k := range spec.Fields {
			add(st.Fields[k], img.Fields[k])
			if !hasValue(img.Fields[k]) {
				st.Missing[k]++
			}
		}
		for t, keys := range spec.Objects {
			o, n := st.Objects[t], int64(0)
			for _, obj := range img.Objects {
				if obj.Type != t {
					continue
				}
				n++
				for _, k := range keys {
					add(o.Fields[k], obj.Fields[k])
					if !hasValue(obj.Fields[k]) {
						o.Missing[k]++
					}
				}
			}
			o.Count += n
			if n > 0 {
				o.Images++
			}
		}
	}
	return st, nil
}

func (m *Memory) MatchingIDs(ctx context.Context, f ImageFilter) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ListErr != nil {
		return nil, m.ListErr
	}
	ids := []string{}
	for _, img := range m.Images {
		if f.match(&img) {
			ids = append(ids, img.SHA1)
		}
	}
	sort.Strings(ids)
	return ids, nil
}
