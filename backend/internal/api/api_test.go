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
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"SIDM/internal/files"
	"SIDM/internal/store"
)

// memStore guarda tudo em maps. Os campos *Err permitem simular falhas do banco.
type memStore struct {
	mu sync.Mutex

	equipments map[string]bool
	cameras    map[string]bool
	images     map[string]store.Image
	history    []store.HistoryEntry
	lastFilter store.ImageFilter

	pingErr    error
	listErr    error
	replaceErr error
}

func newMemStore() *memStore {
	return &memStore{
		equipments: map[string]bool{},
		cameras:    map[string]bool{},
		images:     map[string]store.Image{},
	}
}

func (m *memStore) Ping(ctx context.Context) error { return m.pingErr }

func (m *memStore) ListEquipments(ctx context.Context) ([]string, error) {
	return m.names(m.equipments)
}

func (m *memStore) ListCameras(ctx context.Context) ([]string, error) {
	return m.names(m.cameras)
}

func (m *memStore) names(set map[string]bool) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.listErr != nil {
		return nil, m.listErr
	}
	out := []string{}
	for n := range set {
		out = append(out, n)
	}
	sort.Strings(out)
	return out, nil
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
	if set[name] {
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

func (m *memStore) FindImage(ctx context.Context, sha1 string) (*store.Image, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	img, ok := m.images[sha1]
	if !ok {
		return nil, store.ErrNotFound
	}
	return &img, nil
}

func (m *memStore) ReplaceImage(ctx context.Context, img *store.Image) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.replaceErr != nil {
		return m.replaceErr
	}
	m.images[img.SHA1] = *img
	return nil
}

// ListImages filtra só por status (o suficiente para os testes da API;
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
		if f.Status == "" || img.Status == f.Status {
			out = append(out, img)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SHA1 < out[j].SHA1 })
	return out, int64(len(out)), nil
}

func (m *memStore) ExistingImages(ctx context.Context, ids []string) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]string{}
	for _, id := range ids {
		if img, ok := m.images[id]; ok {
			out[id] = img.Status
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

type upFile struct{ name, content string }

// upload manda arquivos como multipart no campo "files".
func (e *testEnv) upload(t *testing.T, fs ...upFile) []uploadResult {
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

	rec := e.do("POST", "/images", &body, map[string]string{"Content-Type": mw.FormDataContentType()})
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

	var resp map[string]json.RawMessage
	decode(t, rec, &resp)

	for _, field := range []string{
		"direction", "resolution", "capture_angle", "day_time", "image_range", "weather",
		"conditions", "plate_type", "plate_category", "vehicle_type", "vehicle_color",
		"equipments", "cameras",
	} {
		if _, ok := resp[field]; !ok {
			t.Errorf("faltou %q na resposta", field)
		}
	}

	var eqs []string
	json.Unmarshal(resp["equipments"], &eqs)
	if len(eqs) != 1 || eqs[0] != "8003-MF4739" {
		t.Errorf("equipments = %v", eqs)
	}
}

func TestCodesBancoFora(t *testing.T) {
	env := newTestEnv(t)
	env.db.listErr = errBoom
	rec := env.do("GET", "/codes", nil, nil)
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
