package annotation

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"SIDM/internal/platform"
)

type env struct {
	svc *Service
	mem *Memory
	h   http.Handler
}

func newEnv() *env {
	mem := NewMemory()
	svc := New(mem)
	mux := http.NewServeMux()
	for _, rt := range svc.Routes() {
		mux.HandleFunc(rt.Method+" "+rt.Path, rt.Handler)
	}
	return &env{mem: mem, svc: svc, h: mux}
}

func (e *env) do(method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("X-User", "nathan")
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func status(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, esperado %d: %s", rec.Code, want, rec.Body.String())
	}
}

func profileJSON(t *testing.T) string {
	b, err := json.Marshal(capacete())
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestHTTPCiclo(t *testing.T) {
	e := newEnv()
	body := profileJSON(t)

	status(t, e.do("POST", "/annotation-profiles", body), http.StatusCreated)
	status(t, e.do("POST", "/annotation-profiles", body), http.StatusConflict)
	if e.mem.Profiles["capacete-moto"].CreatedBy != "nathan" {
		t.Error("faltou quem criou")
	}

	rec := e.do("GET", "/annotation-profiles", "")
	status(t, rec, http.StatusOK)
	var list platform.Page[Profile]
	json.Unmarshal(rec.Body.Bytes(), &list)
	if list.Total != 2 || list.Items[0].Name != DefaultProfile || !list.Items[0].Builtin || list.Items[1].Name != "capacete-moto" {
		t.Errorf("lista = %+v", list)
	}

	status(t, e.do("GET", "/annotation-profiles/veiculos", ""), http.StatusOK)
	status(t, e.do("GET", "/annotation-profiles/nao-existe", ""), http.StatusNotFound)

	changed := strings.Replace(body, `"max_len":20`, `"max_len":30`, 1)
	status(t, e.do("PUT", "/annotation-profiles/capacete-moto", changed), http.StatusOK)
	if e.mem.Profiles["capacete-moto"].Fields[2].MaxLen != 30 || e.mem.Profiles["capacete-moto"].CreatedBy != "nathan" {
		t.Errorf("PUT não gravou: %+v", e.mem.Profiles["capacete-moto"].Fields[2])
	}
	status(t, e.do("PUT", "/annotation-profiles/outro", changed), http.StatusBadRequest) // nome diferente

	status(t, e.do("DELETE", "/annotation-profiles/capacete-moto", ""), http.StatusOK)
	status(t, e.do("DELETE", "/annotation-profiles/capacete-moto", ""), http.StatusNotFound)
}

func TestHTTPPadraoNaoMuda(t *testing.T) {
	e := newEnv()
	b, _ := json.Marshal(Vehicles())
	status(t, e.do("POST", "/annotation-profiles", string(b)), http.StatusBadRequest)
	status(t, e.do("DELETE", "/annotation-profiles/veiculos", ""), http.StatusBadRequest)

	// do padrão, só as caixas dos tipos mudam (o resto que vier é ignorado)
	p := Vehicles()
	p.Description = "trocada"
	p.Fields = p.Fields[:1]
	p.Objects[0].BoxLabels = []BoxLabel{{Label: " motorcycle ", Fields: map[string]any{"type": "motorcycle"}}}
	b, _ = json.Marshal(p)
	status(t, e.do("PUT", "/annotation-profiles/veiculos", string(b)), http.StatusOK)
	rec := e.do("GET", "/annotation-profiles/veiculos", "")
	var got Profile
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Description != Vehicles().Description || len(got.Fields) != len(Vehicles().Fields) || !got.Builtin ||
		len(got.Objects[0].BoxLabels) != 1 || got.Objects[0].BoxLabels[0].Label != "motorcycle" {
		t.Errorf("padrão com caixas: %+v", got)
	}
	rec = e.do("GET", "/annotation-profiles", "")
	if strings.Count(rec.Body.String(), `"name":"veiculos"`) != 1 {
		t.Errorf("o registro das caixas apareceu como outro perfil: %s", rec.Body.String())
	}
	// o filho não repete a caixa do pai
	status(t, e.do("POST", "/annotation-profiles", `{"name": "moto", "extends": "veiculos", "objects": [{"key": "vehicle", "fields": [], "box_labels": [{"label": "motorcycle"}]}]}`), http.StatusBadRequest)
	// valor que o campo não aceita
	p.Objects[0].BoxLabels[0].Fields = map[string]any{"type": "aviao"}
	b, _ = json.Marshal(p)
	status(t, e.do("PUT", "/annotation-profiles/veiculos", string(b)), http.StatusBadRequest)
	// tirar todas as caixas
	p.Objects[0].BoxLabels = nil
	b, _ = json.Marshal(p)
	status(t, e.do("PUT", "/annotation-profiles/veiculos", string(b)), http.StatusOK)
	if prof, _ := e.svc.FindProfile(t.Context(), "veiculos"); len(prof.Objects[0].BoxLabels) != 0 {
		t.Errorf("caixas não saíram: %+v", prof.Objects[0].BoxLabels)
	}
}

func TestHTTPInvalidoListaProblemas(t *testing.T) {
	e := newEnv()
	rec := e.do("POST", "/annotation-profiles", `{"name": "x", "fields": [{"key": "a", "type": "lista"}, {"key": "a", "type": "bool"}]}`)
	status(t, rec, http.StatusBadRequest)
	var resp struct {
		Error    string    `json:"error"`
		Problems []Problem `json:"problems"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Problems) != 2 || resp.Problems[0].Path != "fields[0].type" || resp.Problems[1].Path != "fields[1].key" {
		t.Errorf("%+v", resp)
	}
	// campo com nome errado no JSON não passa calado
	status(t, e.do("POST", "/annotation-profiles", `{"name": "x", "feilds": []}`), http.StatusBadRequest)
}

func TestHTTPCheck(t *testing.T) {
	e := newEnv()
	rec := e.do("POST", "/annotation-profiles/veiculos/check",
		`{"fields": {"direction": "rear"}, "objects": [{"type": "vehicle", "fields": {"plate": "abc1d23"}}], "complete": true}`)
	status(t, rec, http.StatusOK)
	var v Values
	json.Unmarshal(rec.Body.Bytes(), &v)
	if v.Objects[0].Fields["plate"] != "ABC1D23" {
		t.Errorf("%+v", v)
	}
	status(t, e.do("POST", "/annotation-profiles/veiculos/check", `{"fields": {"direction": "lado"}}`), http.StatusBadRequest)
	status(t, e.do("POST", "/annotation-profiles/nao-existe/check", `{}`), http.StatusNotFound)
}
