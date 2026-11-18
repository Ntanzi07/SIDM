package api

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

	"SIDM/internal/boxes"
	"SIDM/internal/catalog"
	"SIDM/internal/export"
	"SIDM/internal/files"
	"SIDM/internal/images"
	"SIDM/internal/importer"
	"SIDM/internal/platform"
)

// memStore guarda tudo em maps. Os campos *Err permitem simular falhas do banco.
type memStore struct {
	mu sync.Mutex

	profiles   map[string]importer.Profile
	exportProf map[string]export.Profile

	pingErr error
}

func newMemStore() *memStore {
	return &memStore{
		exportProf: map[string]export.Profile{},
		profiles:   map[string]importer.Profile{},
	}
}

func (m *memStore) Ping(ctx context.Context) error { return m.pingErr }

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

func (m *memStore) CreateProfile(ctx context.Context, p *importer.Profile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.profiles[p.Name]; ok {
		return platform.ErrDuplicate
	}
	m.profiles[p.Name] = *p
	return nil
}

func (m *memStore) FindProfile(ctx context.Context, name string) (*importer.Profile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.profiles[name]
	if !ok {
		return nil, platform.ErrNotFound
	}
	return &p, nil
}

func (m *memStore) ReplaceProfile(ctx context.Context, p *importer.Profile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	old, ok := m.profiles[p.Name]
	if !ok {
		return platform.ErrNotFound
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
		return platform.ErrNotFound
	}
	if _, taken := m.profiles[newName]; taken {
		return platform.ErrDuplicate
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
		return platform.ErrNotFound
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

// TaskRenamed: os perfis de export guardam a tarefa (boxes.TaskListener).
func (m *memStore) TaskRenamed(ctx context.Context, oldName, newName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, p := range m.exportProf {
		if p.Task == oldName {
			p.Task = newName
			m.exportProf[name] = p
		}
	}
	return nil
}

func (m *memStore) CreateExportProfile(ctx context.Context, p *export.Profile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.exportProf[p.Name]; ok {
		return platform.ErrDuplicate
	}
	m.exportProf[p.Name] = *p
	return nil
}

func (m *memStore) FindExportProfile(ctx context.Context, name string) (*export.Profile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.exportProf[name]
	if !ok {
		return nil, platform.ErrNotFound
	}
	return &p, nil
}

func (m *memStore) ReplaceExportProfile(ctx context.Context, p *export.Profile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.exportProf[p.Name]; !ok {
		return platform.ErrNotFound
	}
	m.exportProf[p.Name] = *p
	return nil
}

func (m *memStore) DeleteExportProfile(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.exportProf[name]; !ok {
		return platform.ErrNotFound
	}
	delete(m.exportProf, name)
	return nil
}

func (m *memStore) ListExportProfiles(ctx context.Context) ([]export.Profile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []export.Profile{}
	for _, p := range m.exportProf {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

type testEnv struct {
	db      *memStore
	cat     *catalog.Memory // pacotes, equipamentos e câmeras
	box     *boxes.Memory   // tarefas e caixas
	img     *images.Memory  // imagens e histórico
	srv     *Server
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
	cat := catalog.NewMemory()
	cat.Equipments["8003-MF4739"] = true
	cat.Cameras["hikvision-233"] = true
	cat.Packages[testPkg] = catalog.Package{Name: testPkg}
	img := images.NewMemory()
	imgSvc := images.New(img, fs, nil, db)
	svc := catalog.New(cat, fs, imgSvc)
	imgSvc.SetCatalog(svc)
	box := boxes.NewMemory()
	bx := boxes.New(box, imgSvc, fs, svc, db)
	svc.Listen(bx)
	imgSvc.Listen(bx)
	srv := New(db, fs, svc, bx, imgSvc)
	return &testEnv{db: db, cat: cat, box: box, img: img, srv: srv, files: fs, root: root, handler: srv.Routes()}
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
func (e *testEnv) upload(t *testing.T, fs ...upFile) []images.UploadResult {
	t.Helper()
	return e.uploadTo(t, testPkg, fs...)
}

// uploadTo manda arquivos como multipart no campo "files" para um pacote.
func (e *testEnv) uploadTo(t *testing.T, pkg string, fs ...upFile) []images.UploadResult {
	t.Helper()
	return e.uploadToWithQuery(t, pkg, "", fs...)
}

// uploadToWithQuery é o uploadTo com parâmetros extras na URL (ex: "&profile=x").
func (e *testEnv) uploadToWithQuery(t *testing.T, pkg, extra string, fs ...upFile) []images.UploadResult {
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
	var res []images.UploadResult
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

type namePage struct {
	Total int64 `json:"total"`
	Items []struct {
		Name string `json:"name"`
	} `json:"items"`
}

func TestSearchEquipments(t *testing.T) {
	env := newTestEnv(t)
	for _, n := range []string{"8004-MF1234", "8005-MF4700", "9000-ABC", "7000-mf9999"} {
		env.cat.Equipments[n] = true
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
	if env.cat.LastSearch.Limit != 10 {
		t.Errorf("limit padrão = %d, want 10 (as primeiras opções do autocomplete)", env.cat.LastSearch.Limit)
	}
	env.do("GET", "/cameras?limit=9999", nil, nil)
	if env.cat.LastSearch.Limit != 50 {
		t.Errorf("limit máximo = %d, want 50", env.cat.LastSearch.Limit)
	}
	env.do("GET", "/cameras?q=%20%20hik%20%20", nil, nil)
	if env.cat.LastSearch.Q != "hik" {
		t.Errorf("busca deveria ignorar espaços nas pontas: %q", env.cat.LastSearch.Q)
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
	env.cat.Err = errBoom
	rec := env.do("GET", "/equipments?q=80", nil, nil)
	assertStatus(t, rec, http.StatusInternalServerError)
	if strings.Contains(rec.Body.String(), "banco caiu") {
		t.Error("detalhe interno do erro vazou para o cliente")
	}
}

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
	if env.cat.Packages["Noturnas-Setembro"].CreatedBy != "nathan" {
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

	var resp page[catalog.PackageSummary]
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
		env.cat.Packages[n] = catalog.Package{Name: n}
	}

	var resp page[catalog.PackageSummary]
	decode(t, env.do("GET", "/packages?q=noturnas", nil, nil), &resp)
	if resp.Total != 2 || len(resp.Items) != 2 {
		t.Errorf("busca por 'noturnas': total=%d items=%d, want 2", resp.Total, len(resp.Items))
	}

	decode(t, env.do("GET", "/packages?limit=1&skip=1", nil, nil), &resp)
	if resp.Total != 4 || len(resp.Items) != 1 {
		t.Errorf("paginação: total=%d items=%d, want 4 e 1", resp.Total, len(resp.Items))
	}

	env.do("GET", "/packages", nil, nil)
	if env.cat.LastSearch.Limit != 20 {
		t.Errorf("limit padrão = %d, want 20", env.cat.LastSearch.Limit)
	}
}
