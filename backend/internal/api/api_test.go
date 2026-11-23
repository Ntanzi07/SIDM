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
	"strings"
	"testing"

	"SIDM/internal/annotation"
	"SIDM/internal/boxes"
	"SIDM/internal/catalog"
	"SIDM/internal/exports"
	"SIDM/internal/files"
	"SIDM/internal/images"
	"SIDM/internal/imports"
	"SIDM/internal/platform"
	"SIDM/internal/versions"
)

type testEnv struct {
	pingErr error           // o /health falha com este erro
	cat     *catalog.Memory // pacotes, equipamentos e câmeras
	box     *boxes.Memory   // tarefas e caixas
	img     *images.Memory  // imagens e histórico
	imgSvc  *images.Service
	imp     *imports.Memory    // perfis de import
	exp     *exports.Memory    // perfis de export
	ann     *annotation.Memory // perfis de anotação
	ver     *versions.Memory   // datasets e versões
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
	cat := catalog.NewMemory()
	cat.Equipments["8003-MF4739"] = true
	cat.Cameras["hikvision-233"] = true
	cat.Packages[testPkg] = catalog.Package{Name: testPkg}
	img := images.NewMemory()
	imp := imports.NewMemory()
	ann := annotation.NewMemory()
	annSvc := annotation.New(ann)
	imgSvc := images.New(img, fs, nil, imports.Profiles{Repo: imp}, annSvc)
	svc := catalog.New(cat, fs, imgSvc, annSvc)
	imgSvc.SetCatalog(svc)
	annSvc.SetPackages(svc)
	box := boxes.NewMemory()
	bx := boxes.New(box, imgSvc, fs, svc, imgSvc)
	svc.Listen(bx)
	imgSvc.Listen(bx)
	impSvc := imports.New(imp, imgSvc, svc, annSvc)
	exp := exports.NewMemory()
	expSvc := exports.New(exp, imgSvc, bx, svc, fs, annSvc)
	bx.Listen(expSvc)
	ver := versions.NewMemory()
	verSvc := versions.New(ver, imgSvc, bx, annSvc)
	env := &testEnv{cat: cat, box: box, img: img, imgSvc: imgSvc, imp: imp, exp: exp, ann: ann, ver: ver, files: fs, root: root}
	env.srv = New(func(context.Context) error { return env.pingErr }, svc, imgSvc, bx, impSvc, expSvc,
		annSvc, verSvc)
	env.handler = env.srv.Routes()
	return env
}

// fv: o valor de um campo como texto ("" se não tem).
func fv(fields map[string]any, key string) string {
	s, _ := fields[key].(string)
	return s
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

func (e *testEnv) pkgFile2(pkg, name string) string {
	return filepath.Join(e.root, "dataset", pkg, name)
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

	env.pingErr = errBoom
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

	var resp platform.Page[catalog.PackageSummary]
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

	var resp platform.Page[catalog.PackageSummary]
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
