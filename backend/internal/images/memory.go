package images

import (
	"SIDM/internal/catalog"
	"SIDM/internal/importer"
	"SIDM/internal/platform"
	"context"
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
		if (f.Status == "" || img.Status == f.Status) && (f.Package == "" || img.Package == f.Package) &&
			(f.Equipment == "" || img.Equipment == f.Equipment) && (f.Camera == "" || img.Camera == f.Camera) &&
			(f.DayTime == "" || img.DayTime == f.DayTime) && (f.Weather == "" || img.Weather == f.Weather) &&
			(f.Direction == "" || img.Direction == f.Direction) {
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
			out = append(out, img)
		}
	}
	return out, nil
}

func imageField(img *Image, reg catalog.Registry) *string {
	if reg == catalog.Cameras {
		return &img.Camera
	}
	return &img.Equipment
}

func (m *Memory) CountUsing(ctx context.Context, reg catalog.Registry, name string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for _, img := range m.Images {
		if *imageField(&img, reg) == name {
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
		if f := imageField(&img, reg); *f == oldName && img.Status == StatusPending {
			*f, img.UpdatedAt = newName, now
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
