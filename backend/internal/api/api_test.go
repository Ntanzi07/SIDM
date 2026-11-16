package api

// Infra comum dos testes da API: um banco em memória (memStore) que implementa
// a interface Store, e helpers para fazer requests sem subir servidor de verdade
// (httptest.NewRecorder captura a resposta do handler).

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"SIDM/internal/files"
	"SIDM/internal/importer"
	"SIDM/internal/store"
)

type searchCall struct {
	q           string
	limit, skip int64
}

// memStore guarda tudo em maps. Os campos *Err permitem simular falhas do banco.
type memStore struct {
	mu sync.Mutex

	equipments map[string]bool // nome -> ativo (a chave existir = está cadastrado)
	cameras    map[string]bool
	packages   map[string]store.Package
	profiles   map[string]importer.Profile
	images     map[string]store.Image
	history    []store.HistoryEntry
	lastFilter store.ImageFilter
	lastSearch searchCall

	pingErr       error
	listErr       error
	replaceErr    error
	beforeReplace func()
}

func newMemStore() *memStore {
	return &memStore{
		equipments: map[string]bool{},
		cameras:    map[string]bool{},
		packages:   map[string]store.Package{},
		profiles:   map[string]importer.Profile{},
		images:     map[string]store.Image{},
	}
}

func (m *memStore) Ping(ctx context.Context) error { return m.pingErr }

func (m *memStore) SearchEquipments(ctx context.Context, q string, limit, skip int64) ([]string, int64, error) {
	return m.search(m.equipments, q, limit, skip)
}

func (m *memStore) SearchCameras(ctx context.Context, q string, limit, skip int64) ([]string, int64, error) {
	return m.search(m.cameras, q, limit, skip)
}

// search imita o store de verdade: contém, sem diferenciar maiúscula,
// em ordem alfabética, paginado.
func (m *memStore) search(set map[string]bool, q string, limit, skip int64) ([]string, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastSearch = searchCall{q: q, limit: limit, skip: skip}
	if m.listErr != nil {
		return nil, 0, m.listErr
	}
	all := []string{}
	for n, active := range set {
		if active && strings.Contains(strings.ToLower(n), strings.ToLower(q)) {
			all = append(all, n)
		}
	}
	sort.Strings(all)
	return paginate(all, limit, skip), int64(len(all)), nil
}

func paginate[T any](all []T, limit, skip int64) []T {
	if skip >= int64(len(all)) {
		return []T{}
	}
	end := skip + limit
	if end > int64(len(all)) {
		end = int64(len(all))
	}
	return all[skip:end]
}

func (m *memStore) CreateEquipment(ctx context.Context, name string) error {
	return m.create(m.equipments, name)
}

func (m *memStore) CreateCamera(ctx context.Context, name string) error {
	return m.create(m.cameras, name)
}

func (m *memStore) create(set map[string]bool, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := set[name]; ok {
		return store.ErrDuplicate
	}
	set[name] = true
	return nil
}

func (m *memStore) EquipmentExists(ctx context.Context, name string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.equipments[name], nil
}

func (m *memStore) CameraExists(ctx context.Context, name string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cameras[name], nil
}

func (m *memStore) CreatePackage(ctx context.Context, name, createdBy string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.packages[name]; ok {
		return store.ErrDuplicate
	}
	m.packages[name] = store.Package{Name: name, CreatedBy: createdBy}
	return nil
}

func (m *memStore) FindPackage(ctx context.Context, name string) (*store.Package, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.packages[name]
	if !ok {
		return nil, store.ErrNotFound
	}
	return &p, nil
}

func (m *memStore) ListPackages(ctx context.Context, q string, limit, skip int64) ([]store.PackageSummary, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastSearch = searchCall{q: q, limit: limit, skip: skip}
	if m.listErr != nil {
		return nil, 0, m.listErr
	}
	out := []store.PackageSummary{}
	for _, p := range m.packages {
		if !strings.Contains(strings.ToLower(p.Name), strings.ToLower(q)) {
			continue
		}
		sum := store.PackageSummary{Package: p}
		for _, img := range m.images {
			if img.Package != p.Name {
				continue
			}
			sum.Total++
			if img.Status == store.StatusVerified {
				sum.Verified++
			} else {
				sum.Pending++
				if img.ClaimedUntil != nil && img.ClaimedUntil.After(time.Now()) {
					sum.InProgress++
				}
			}
		}
		out = append(out, sum)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return paginate(out, limit, skip), int64(len(out)), nil
}

func (m *memStore) InsertImage(ctx context.Context, img *store.Image) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.images[img.SHA1]; ok {
		return store.ErrDuplicate
	}
	m.images[img.SHA1] = *img
	return nil
}

func (m *memStore) DeleteImage(ctx context.Context, sha1 string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.images, sha1)
	return nil
}

func (m *memStore) DeleteImageHistory(ctx context.Context, sha1 string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := m.history[:0]
	for _, h := range m.history {
		if h.ImageID != sha1 {
			kept = append(kept, h)
		}
	}
	m.history = kept
	return nil
}

func (m *memStore) FindImage(ctx context.Context, sha1 string) (*store.Image, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	img, ok := m.images[sha1]
	if !ok {
		return nil, store.ErrNotFound
	}
	return &img, nil
}

func (m *memStore) ReplaceImage(ctx context.Context, img *store.Image, prevUpdatedAt time.Time) error {
	if m.beforeReplace != nil {
		m.beforeReplace() // simula outra pessoa gravando no meio
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.replaceErr != nil {
		return m.replaceErr
	}
	cur, ok := m.images[img.SHA1]
	if !ok {
		return store.ErrNotFound
	}
	if !cur.UpdatedAt.Equal(prevUpdatedAt) {
		return store.ErrConflict
	}
	m.images[img.SHA1] = *img
	return nil
}

// ClaimNext segue as mesmas regras do store de verdade (ver store/queue.go).
func (m *memStore) ClaimNext(ctx context.Context, pkg, user, skip string, lease time.Duration, now time.Time) (*store.Image, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if img, ok := m.images[skip]; ok && img.Package == pkg && img.ClaimedBy == user {
		img.QueuedAt, img.ClaimedBy, img.ClaimedUntil = now, "", nil
		m.images[skip] = img
	}
	for id, img := range m.images {
		if img.Package == pkg && img.Status == store.StatusPending && img.ClaimedBy == user {
			img.ClaimedBy, img.ClaimedUntil = "", nil
			m.images[id] = img
		}
	}

	best := m.oldestFree(pkg, now, skip)
	if best == nil && skip != "" {
		best = m.oldestFree(pkg, now, "") // a pulada era a única: ela volta
	}
	if best == nil {
		return nil, store.ErrNotFound
	}
	until := now.Add(lease)
	best.ClaimedBy, best.ClaimedUntil = user, &until
	m.images[best.SHA1] = *best
	out := *best
	return &out, nil
}

// oldestFree: a pendente livre mais antiga da fila, ignorando `except`.
func (m *memStore) oldestFree(pkg string, now time.Time, except string) *store.Image {
	var best *store.Image
	for _, img := range m.images {
		img := img
		free := img.ClaimedUntil == nil || !img.ClaimedUntil.After(now)
		if img.Package != pkg || img.Status != store.StatusPending || !free || img.SHA1 == except {
			continue
		}
		if best == nil || img.QueuedAt.Before(best.QueuedAt) ||
			(img.QueuedAt.Equal(best.QueuedAt) && img.SHA1 < best.SHA1) {
			best = &img
		}
	}
	return best
}

func (m *memStore) ReleaseClaim(ctx context.Context, sha1, user string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if img, ok := m.images[sha1]; ok && img.ClaimedBy == user {
		img.ClaimedBy, img.ClaimedUntil = "", nil
		m.images[sha1] = img
	}
	return nil
}

// ListImages filtra por status, pacote, equipamento e câmera (o suficiente para os testes da API;
// os filtros de verdade são testados contra o Mongo no pacote store).
func (m *memStore) ListImages(ctx context.Context, f store.ImageFilter) ([]store.Image, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastFilter = f
	if m.listErr != nil {
		return nil, 0, m.listErr
	}
	out := []store.Image{}
	for _, img := range m.images {
		if (f.Status == "" || img.Status == f.Status) && (f.Package == "" || img.Package == f.Package) &&
			(f.Equipment == "" || img.Equipment == f.Equipment) && (f.Camera == "" || img.Camera == f.Camera) {
			out = append(out, img)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SHA1 < out[j].SHA1 })
	if f.Limit > 0 {
		return paginate(out, f.Limit, f.Skip), int64(len(out)), nil
	}
	return out, int64(len(out)), nil
}

func (m *memStore) ExistingImages(ctx context.Context, ids []string) (map[string]store.ExistingImage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]store.ExistingImage{}
	for _, id := range ids {
		if img, ok := m.images[id]; ok {
			out[id] = store.ExistingImage{Package: img.Package, Status: img.Status}
		}
	}
	return out, nil
}

func (m *memStore) InsertHistory(ctx context.Context, entries []store.HistoryEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.history = append(m.history, entries...)
	return nil
}

func (m *memStore) CreateProfile(ctx context.Context, p *importer.Profile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.profiles[p.Name]; ok {
		return store.ErrDuplicate
	}
	m.profiles[p.Name] = *p
	return nil
}

func (m *memStore) FindProfile(ctx context.Context, name string) (*importer.Profile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.profiles[name]
	if !ok {
		return nil, store.ErrNotFound
	}
	return &p, nil
}

func (m *memStore) ReplaceProfile(ctx context.Context, p *importer.Profile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	old, ok := m.profiles[p.Name]
	if !ok {
		return store.ErrNotFound
	}
	p.CreatedBy = old.CreatedBy
	m.profiles[p.Name] = *p
	return nil
}

func (m *memStore) RenameProfile(ctx context.Context, oldName, newName string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.profiles[oldName]
	if !ok {
		return store.ErrNotFound
	}
	if _, taken := m.profiles[newName]; taken {
		return store.ErrDuplicate
	}
	delete(m.profiles, oldName)
	p.Name, p.UpdatedAt = newName, now
	m.profiles[newName] = p
	return nil
}

func (m *memStore) DeleteProfile(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.profiles[name]; !ok {
		return store.ErrNotFound
	}
	delete(m.profiles, name)
	return nil
}

func (m *memStore) ListProfiles(ctx context.Context, q string, limit, skip int64) ([]importer.Profile, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []importer.Profile{}
	for _, p := range m.profiles {
		if strings.Contains(strings.ToLower(p.Name), strings.ToLower(q)) {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return paginate(out, limit, skip), int64(len(out)), nil
}

func (m *memStore) FindImagesByKeys(ctx context.Context, pkg, by string, keys []string) ([]store.Image, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	want := map[string]bool{}
	for _, k := range keys {
		want[k] = true
	}
	out := []store.Image{}
	for _, img := range m.images {
		k := img.OriginalName
		if by == importer.MatchBySHA1 {
			k = img.SHA1
		}
		if img.Package == pkg && want[k] {
			out = append(out, img)
		}
	}
	return out, nil
}

func (m *memStore) registry(reg store.Registry) map[string]bool {
	if reg == store.Cameras {
		return m.cameras
	}
	return m.equipments
}

func (m *memStore) FindRegistry(ctx context.Context, reg store.Registry, name string) (*store.RegistryEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	active, ok := m.registry(reg)[name]
	if !ok {
		return nil, store.ErrNotFound
	}
	return &store.RegistryEntry{Name: name, Active: active}, nil
}

func (m *memStore) SetRegistryActive(ctx context.Context, reg store.Registry, name string, active bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	set := m.registry(reg)
	if _, ok := set[name]; !ok {
		return store.ErrNotFound
	}
	set[name] = active
	return nil
}

func (m *memStore) DeleteRegistry(ctx context.Context, reg store.Registry, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	set := m.registry(reg)
	if _, ok := set[name]; !ok {
		return store.ErrNotFound
	}
	delete(set, name)
	return nil
}

func imageField(img *store.Image, reg store.Registry) *string {
	if reg == store.Cameras {
		return &img.Camera
	}
	return &img.Equipment
}

func (m *memStore) CountImagesUsing(ctx context.Context, reg store.Registry, name string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for _, img := range m.images {
		if *imageField(&img, reg) == name {
			n++
		}
	}
	return n, nil
}

func (m *memStore) RenameOnPending(ctx context.Context, reg store.Registry, oldName, newName string, now time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for id, img := range m.images {
		if f := imageField(&img, reg); *f == oldName && img.Status == store.StatusPending {
			*f, img.UpdatedAt = newName, now
			m.images[id] = img
			n++
		}
	}
	return n, nil
}

func (m *memStore) RenamePackage(ctx context.Context, oldName, newName string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.packages[oldName]
	if !ok {
		return 0, store.ErrNotFound
	}
	if _, exists := m.packages[newName]; exists {
		return 0, store.ErrDuplicate
	}
	delete(m.packages, oldName)
	p.Name = newName
	m.packages[newName] = p

	var n int64
	for id, img := range m.images {
		if img.Package == oldName {
			img.Package = newName
			img.Path = "dataset/" + newName + "/" + strings.TrimPrefix(img.Path, "dataset/"+oldName+"/")
			m.images[id] = img
			n++
		}
	}
	return n, nil
}

func (m *memStore) DeletePackage(ctx context.Context, name string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.packages[name]; !ok {
		return 0, store.ErrNotFound
	}
	deleted := map[string]bool{}
	for id, img := range m.images {
		if img.Package == name {
			deleted[id] = true
			delete(m.images, id)
		}
	}
	kept := m.history[:0]
	for _, h := range m.history {
		if !deleted[h.ImageID] {
			kept = append(kept, h)
		}
	}
	m.history = kept
	delete(m.packages, name)
	return int64(len(deleted)), nil
}

// ---------- helpers ----------

type testEnv struct {
	db      *memStore
	files   *files.Storage
	root    string
	handler http.Handler
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	root := t.TempDir()
	fs, err := files.NewStorage(root)
	if err != nil {
		t.Fatal(err)
	}
	db := newMemStore()
	db.equipments["8003-MF4739"] = true
	db.cameras["hikvision-233"] = true
	db.packages[testPkg] = store.Package{Name: testPkg}
	return &testEnv{db: db, files: fs, root: root, handler: New(db, fs).Routes()}
}

// do executa um request e devolve a resposta gravada.
func (e *testEnv) do(method, path string, body io.Reader, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, body)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec
}

func (e *testEnv) doJSON(method, path, body string) *httptest.ResponseRecorder {
	return e.do(method, path, strings.NewReader(body), map[string]string{"Content-Type": "application/json"})
}

// Pacote que já vem criado em todo testEnv.
const testPkg = "pacote-teste"

type upFile struct{ name, content string }

// upload manda arquivos para o pacote de teste.
func (e *testEnv) upload(t *testing.T, fs ...upFile) []uploadResult {
	t.Helper()
	return e.uploadTo(t, testPkg, fs...)
}

// uploadTo manda arquivos como multipart no campo "files" para um pacote.
func (e *testEnv) uploadTo(t *testing.T, pkg string, fs ...upFile) []uploadResult {
	t.Helper()
	return e.uploadToWithQuery(t, pkg, "", fs...)
}

// uploadToWithQuery é o uploadTo com parâmetros extras na URL (ex: "&profile=x").
func (e *testEnv) uploadToWithQuery(t *testing.T, pkg, extra string, fs ...upFile) []uploadResult {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for _, f := range fs {
		w, err := mw.CreateFormFile("files", f.name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(f.content))
	}
	mw.Close()

	rec := e.do("POST", "/images?package="+pkg+extra, &body, map[string]string{"Content-Type": mw.FormDataContentType()})
	if rec.Code != http.StatusOK {
		t.Fatalf("upload: status %d: %s", rec.Code, rec.Body.String())
	}
	var res []uploadResult
	decode(t, rec, &res)
	return res
}

// seedPending sobe uma imagem e devolve o sha1 dela.
func (e *testEnv) seedPending(t *testing.T, content string) string {
	t.Helper()
	res := e.upload(t, upFile{"original.jpg", content})
	if res[0].Status != "created" {
		t.Fatalf("seed: %+v", res[0])
	}
	return res[0].SHA1
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
		t.Fatalf("resposta não é JSON válido: %v\n%s", err, rec.Body.String())
	}
}

func errorMsg(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var e struct {
		Error string `json:"error"`
	}
	decode(t, rec, &e)
	return e.Error
}

// pkgFile é o caminho real de um arquivo dentro da pasta do pacote de teste.
func (e *testEnv) pkgFile(name string) string {
	return filepath.Join(e.root, "dataset", testPkg, name)
}

func sha1Of(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d. body: %s", rec.Code, want, rec.Body.String())
	}
}

var errBoom = errors.New("banco caiu")

// ---------- /health e /codes ----------

func TestHealth(t *testing.T) {
	env := newTestEnv(t)
	rec := env.do("GET", "/health", nil, nil)
	assertStatus(t, rec, http.StatusOK)
	if rec.Body.String() != "ok" {
		t.Errorf("body = %q", rec.Body.String())
	}

	env.db.pingErr = errBoom
	assertStatus(t, env.do("GET", "/health", nil, nil), http.StatusServiceUnavailable)
}

func TestCodes(t *testing.T) {
	env := newTestEnv(t)
	rec := env.do("GET", "/codes", nil, nil)
	assertStatus(t, rec, http.StatusOK)

	var resp map[string]map[string]string
	decode(t, rec, &resp)

	for _, field := range []string{
		"direction", "resolution", "capture_angle", "day_time", "image_range", "weather",
		"conditions", "plate_type", "plate_category", "vehicle_type", "vehicle_color",
	} {
		if len(resp[field]) == 0 {
			t.Errorf("faltou a tabela %q", field)
		}
	}
	if resp["day_time"]["D"] != "day" {
		t.Errorf("day_time = %v", resp["day_time"])
	}
	if _, ok := resp["equipments"]; ok {
		t.Error("equipamentos não devem vir no /codes: têm busca própria e paginada")
	}
}

// ---------- busca de equipamentos e câmeras (autocomplete) ----------

type namePage struct {
	Total int64      `json:"total"`
	Items []nameItem `json:"items"`
}

func TestSearchEquipments(t *testing.T) {
	env := newTestEnv(t)
	for _, n := range []string{"8004-MF1234", "8005-MF4700", "9000-ABC", "7000-mf9999"} {
		env.db.equipments[n] = true
	}

	cases := []struct {
		name      string
		query     string
		wantTotal int64
		wantNames string
	}{
		{"sem busca: todos em ordem", "", 5, "[7000-mf9999 8003-MF4739 8004-MF1234 8005-MF4700 9000-ABC]"},
		{"contém, no meio do nome", "?q=mf47", 2, "[8003-MF4739 8005-MF4700]"},
		{"ignora maiúscula", "?q=MF9", 1, "[7000-mf9999]"},
		{"nada encontrado", "?q=xyz", 0, "[]"},
		{"limit", "?limit=2", 5, "[7000-mf9999 8003-MF4739]"},
		{"segunda página", "?limit=2&skip=2", 5, "[8004-MF1234 8005-MF4700]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := env.do("GET", "/equipments"+tc.query, nil, nil)
			assertStatus(t, rec, http.StatusOK)
			var p namePage
			decode(t, rec, &p)

			names := make([]string, len(p.Items))
			for i, it := range p.Items {
				names[i] = it.Name
			}
			if p.Total != tc.wantTotal || fmt.Sprint(names) != tc.wantNames {
				t.Errorf("total=%d items=%v; want %d %s", p.Total, names, tc.wantTotal, tc.wantNames)
			}
		})
	}
}

func TestSearchLimites(t *testing.T) {
	env := newTestEnv(t)

	env.do("GET", "/cameras", nil, nil)
	if env.db.lastSearch.limit != 10 {
		t.Errorf("limit padrão = %d, want 10 (as primeiras opções do autocomplete)", env.db.lastSearch.limit)
	}
	env.do("GET", "/cameras?limit=9999", nil, nil)
	if env.db.lastSearch.limit != 50 {
		t.Errorf("limit máximo = %d, want 50", env.db.lastSearch.limit)
	}
	env.do("GET", "/cameras?q=%20%20hik%20%20", nil, nil)
	if env.db.lastSearch.q != "hik" {
		t.Errorf("busca deveria ignorar espaços nas pontas: %q", env.db.lastSearch.q)
	}

	long := strings.Repeat("a", 65)
	assertStatus(t, env.do("GET", "/cameras?q="+long, nil, nil), http.StatusBadRequest)
}

func TestSearchVaziaRetornaArray(t *testing.T) {
	env := newTestEnv(t)
	rec := env.do("GET", "/cameras?q=nada", nil, nil)
	if !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Errorf("sem resultados deve ser [] e não null: %s", rec.Body.String())
	}
}

func TestSearchBancoFora(t *testing.T) {
	env := newTestEnv(t)
	env.db.listErr = errBoom
	rec := env.do("GET", "/equipments?q=80", nil, nil)
	assertStatus(t, rec, http.StatusInternalServerError)
	if strings.Contains(rec.Body.String(), "banco caiu") {
		t.Error("detalhe interno do erro vazou para o cliente")
	}
}

// ---------- cadastro ----------

func TestCreateEquipmentAndCamera(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		body     string
		status   int
		wantName string
	}{
		{"equipamento sanitizado", "/equipments", `{"name":"8004 MF1234"}`, http.StatusCreated, "8004-MF1234"},
		{"câmera sanitizada", "/cameras", `{"name":"  Intelbras  VIP 3230 "}`, http.StatusCreated, "Intelbras-VIP-3230"},
		{"duplicado", "/equipments", `{"name":"8003 MF4739"}`, http.StatusConflict, ""},
		{"caractere inválido", "/equipments", `{"name":"8003/MF"}`, http.StatusBadRequest, ""},
		{"vazio", "/cameras", `{"name":"   "}`, http.StatusBadRequest, ""},
		{"JSON quebrado", "/cameras", `{"name":`, http.StatusBadRequest, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestEnv(t)
			rec := env.doJSON("POST", tc.path, tc.body)
			assertStatus(t, rec, tc.status)
			if tc.wantName != "" {
				var resp map[string]string
				decode(t, rec, &resp)
				if resp["name"] != tc.wantName {
					t.Errorf("name = %q, want %q", resp["name"], tc.wantName)
				}
			} else if errorMsg(t, rec) == "" {
				t.Error("erro sem mensagem")
			}
		})
	}
}

func TestCreateCorpoGrandeDemais(t *testing.T) {
	env := newTestEnv(t)
	big := `{"name":"` + strings.Repeat("a", 2000) + `"}`
	assertStatus(t, env.doJSON("POST", "/equipments", big), http.StatusBadRequest)
}

// ---------- pacotes ----------

func TestCreatePackage(t *testing.T) {
	env := newTestEnv(t)
	rec := env.do("POST", "/packages", strings.NewReader(`{"name":"Noturnas Setembro"}`),
		map[string]string{"Content-Type": "application/json", "X-User": "nathan"})
	assertStatus(t, rec, http.StatusCreated)

	var resp map[string]string
	decode(t, rec, &resp)
	if resp["name"] != "Noturnas-Setembro" {
		t.Errorf("name = %q", resp["name"])
	}
	if env.db.packages["Noturnas-Setembro"].CreatedBy != "nathan" {
		t.Error("created_by deveria vir do X-User")
	}
	if info, err := os.Stat(filepath.Join(env.root, "dataset", "Noturnas-Setembro")); err != nil || !info.IsDir() {
		t.Error("a pasta do pacote deveria ser criada já na criação")
	}
}

func TestCreatePackageInvalido(t *testing.T) {
	cases := map[string]struct {
		body   string
		status int
	}{
		"duplicado":            {`{"name":"` + testPkg + `"}`, http.StatusConflict},
		"tentando subir pasta": {`{"name":"../../etc"}`, http.StatusBadRequest},
		"com barra":            {`{"name":"a/b"}`, http.StatusBadRequest},
		"vazio":                {`{"name":""}`, http.StatusBadRequest},
		"JSON quebrado":        {`{`, http.StatusBadRequest},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			env := newTestEnv(t)
			assertStatus(t, env.doJSON("POST", "/packages", tc.body), tc.status)
		})
	}
}

func TestListPackagesComProgresso(t *testing.T) {
	env := newTestEnv(t)
	shaA := env.seedPending(t, "a")
	env.seedPending(t, "b")
	env.seedPending(t, "c")
	assertStatus(t, env.doJSON("PUT", "/images/"+shaA, completeBody()), http.StatusOK)

	rec := env.do("GET", "/packages", nil, nil)
	assertStatus(t, rec, http.StatusOK)

	var resp page[store.PackageSummary]
	decode(t, rec, &resp)
	if resp.Total != 1 || len(resp.Items) != 1 {
		t.Fatalf("pacotes = %+v", resp)
	}
	p := resp.Items[0]
	if p.Name != testPkg || p.Total != 3 || p.Verified != 1 || p.Pending != 2 {
		t.Errorf("progresso errado: %+v (want total 3, verified 1, pending 2)", p)
	}
}

func TestListPackagesBuscaEPaginacao(t *testing.T) {
	env := newTestEnv(t)
	for _, n := range []string{"Noturnas-Setembro", "Noturnas-Outubro", "Diurnas-2026"} {
		env.db.packages[n] = store.Package{Name: n}
	}

	var resp page[store.PackageSummary]
	decode(t, env.do("GET", "/packages?q=noturnas", nil, nil), &resp)
	if resp.Total != 2 || len(resp.Items) != 2 {
		t.Errorf("busca por 'noturnas': total=%d items=%d, want 2", resp.Total, len(resp.Items))
	}

	decode(t, env.do("GET", "/packages?limit=1&skip=1", nil, nil), &resp)
	if resp.Total != 4 || len(resp.Items) != 1 {
		t.Errorf("paginação: total=%d items=%d, want 4 e 1", resp.Total, len(resp.Items))
	}

	env.do("GET", "/packages", nil, nil)
	if env.db.lastSearch.limit != 20 {
		t.Errorf("limit padrão = %d, want 20", env.db.lastSearch.limit)
	}
}
