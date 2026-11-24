package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"SIDM/internal/boxes"
	"SIDM/internal/export"
	"SIDM/internal/exports"
	"SIDM/internal/platform"
	"SIDM/internal/versions"
)

// O caminho da tela: cria o dataset com os filtros da Pesquisa, congela
// versões (só as verificadas), lê o resumo e as imagens.
func TestVersoesDeDataset(t *testing.T) {
	env := newTestEnv(t)
	env.withCapacete(t)
	res := env.uploadTo(t, "motos", upFile{"m1.jpg", "moto 1"}, upFile{"m2.jpg", "moto 2"}, upFile{"m3.jpg", "moto 3"})
	assertStatus(t, env.doJSON("PUT", "/images/"+res[0].SHA1, `{"fields": {"day_time": "night"}, "complete": true}`), http.StatusOK)
	assertStatus(t, env.doJSON("PUT", "/images/"+res[1].SHA1, `{"fields": {"day_time": "day"}, "complete": true}`), http.StatusOK)
	assertStatus(t, env.doJSON("PUT", "/images/"+res[2].SHA1, `{"fields": {"day_time": "night"}}`), http.StatusOK) // rascunho
	env.box.Tasks["capacete"] = boxes.Task{Name: "capacete", Labels: []string{"helmet", "no_helmet"}}
	env.box.Put(boxes.Set{Task: "capacete", ImageID: res[0].SHA1, Package: "motos", Width: 32, Height: 32,
		Boxes: []boxes.Box{{Label: "helmet", X: 1, Y: 1, W: 5, H: 5}}})

	post := func(path, body, user string) *httptest.ResponseRecorder {
		h := map[string]string{"Content-Type": "application/json"}
		if user != "" {
			h["X-User"] = user
		}
		return env.do("POST", path, strings.NewReader(body), h)
	}

	// cria: sem X-User 400; filtro que o perfil não tem 400; certo 201
	assertStatus(t, post("/datasets", `{"name": "motos-noite", "profile": "capacete-moto"}`, ""), http.StatusBadRequest)
	assertStatus(t, post("/datasets", `{"name": "x", "profile": "capacete-moto", "query": {"filters": [{"key": "fields.weather", "value": "rain"}]}}`, "nathan"), http.StatusBadRequest)
	assertStatus(t, post("/datasets", `{"name": "x", "profile": "nao-existe"}`, "nathan"), http.StatusBadRequest)
	rec := post("/datasets", `{"name": "motos-noite", "profile": "capacete-moto", "description": "motos à noite",
		"query": {"packages": ["motos"], "filters": [{"key": "fields.day_time", "value": "night"}]}}`, "nathan")
	assertStatus(t, rec, http.StatusCreated)
	assertStatus(t, post("/datasets", `{"name": "motos-noite", "profile": "capacete-moto"}`, "nathan"), http.StatusConflict)

	// versão: sem mensagem 400; certa 201, só a verificada noturna (m1)
	assertStatus(t, post("/datasets/motos-noite/versions", `{"message": ""}`, "nathan"), http.StatusBadRequest)
	assertStatus(t, post("/datasets/motos-noite/versions", `{"message": "v"}`, ""), http.StatusBadRequest)
	assertStatus(t, post("/datasets/nao-existe/versions", `{"message": "v"}`, "nathan"), http.StatusNotFound)
	rec = post("/datasets/motos-noite/versions", `{"message": "primeira versão"}`, "nathan")
	assertStatus(t, rec, http.StatusCreated)
	var v versions.Version
	decode(t, rec, &v)
	if v.Number != 1 || v.Summary.Images.Total != 1 || v.Summary.Images.Packages["motos"] != 1 || v.CreatedBy != "nathan" {
		t.Errorf("v1: %+v", v.Summary.Images)
	}
	if len(v.Summary.Labels) != 1 || v.Summary.Labels[0].Labels["helmet"].Boxes != 1 {
		t.Errorf("labels: %+v", v.Summary.Labels)
	}

	// a imagem 2 vira noturna: a v2 tem 2; a v1 continua com 1
	assertStatus(t, env.doJSON("PUT", "/images/"+res[1].SHA1, `{"fields": {"day_time": "night"}, "complete": true}`), http.StatusOK)
	assertStatus(t, post("/datasets/motos-noite/versions", `{"message": "a m2 era noturna"}`, "guilherme"), http.StatusCreated)

	rec = env.do("GET", "/datasets/motos-noite/versions", nil, nil)
	assertStatus(t, rec, http.StatusOK)
	var list platform.Page[versions.Version]
	decode(t, rec, &list)
	if list.Total != 2 || list.Items[0].Number != 2 || list.Items[0].Summary.Images.Total != 2 || list.Items[1].Summary.Images.Total != 1 {
		t.Errorf("versões: %+v", list.Items)
	}

	rec = env.do("GET", "/datasets/motos-noite/versions/v1/items", nil, nil)
	assertStatus(t, rec, http.StatusOK)
	var items platform.Page[versions.Item]
	decode(t, rec, &items)
	if items.Total != 1 || items.Items[0].SHA1 != res[0].SHA1 || len(items.Items[0].Boxes["capacete"].Boxes) != 1 {
		t.Errorf("itens da v1: %+v", items)
	}

	// o diff: a m2 entrou na v2; o detalhe diz o que ela tem
	rec = env.do("GET", "/datasets/motos-noite/diff", nil, nil)
	assertStatus(t, rec, http.StatusOK)
	var d versions.Diff
	decode(t, rec, &d)
	if d.From == nil || d.From.Number != 1 || d.To.Number != 2 || len(d.Added) != 1 || d.Added[0].SHA1 != res[1].SHA1 || d.Unchanged != 1 {
		t.Errorf("diff: %+v", d)
	}
	rec = env.do("GET", "/datasets/motos-noite/diff/"+res[1].SHA1+"?from=v1&to=2", nil, nil)
	assertStatus(t, rec, http.StatusOK)
	var item versions.ItemDiff
	decode(t, rec, &item)
	if item.Status != versions.StatusAdded || len(item.Fields) == 0 {
		t.Errorf("detalhe: %+v", item)
	}
	rec = env.do("GET", "/datasets/motos-noite/diff?summary=1", nil, nil)
	assertStatus(t, rec, http.StatusOK)
	var short versions.Diff
	decode(t, rec, &short)
	if len(short.Added) != 0 || short.Counts.Added != 1 || short.Counts.Unchanged != 1 {
		t.Errorf("summary=1: %+v", short)
	}
	// o agora: GET .../versions/atual, diff até ele e a pesquisa nele
	rec = env.do("GET", "/datasets/motos-noite/versions/atual", nil, nil)
	assertStatus(t, rec, http.StatusOK)
	var now versions.Version
	decode(t, rec, &now)
	if now.Number != 0 || now.Summary.Images.Total != 2 {
		t.Errorf("agora: %+v", now)
	}
	rec = env.do("GET", "/datasets/motos-noite/diff?to=atual&summary=1", nil, nil)
	assertStatus(t, rec, http.StatusOK)
	assertStatus(t, env.do("GET", "/datasets/motos-noite/versions/atual/stats", nil, nil), http.StatusOK)
	assertStatus(t, env.do("GET", "/datasets/motos-noite/versions/atual/items", nil, nil), http.StatusBadRequest)
	assertStatus(t, env.do("GET", "/datasets/motos-noite/diff?from=2&to=2", nil, nil), http.StatusBadRequest)
	assertStatus(t, env.do("GET", "/datasets/motos-noite/diff?from=x", nil, nil), http.StatusBadRequest)
	assertStatus(t, env.do("GET", "/datasets/motos-noite/diff?to=9", nil, nil), http.StatusNotFound)
	assertStatus(t, env.do("GET", "/datasets/motos-noite/diff/zzz", nil, nil), http.StatusNotFound)

	// a lista de datasets traz a mais nova
	rec = env.do("GET", "/datasets", nil, nil)
	var ds platform.Page[versions.Dataset]
	decode(t, rec, &ds)
	if ds.Total != 1 || ds.Items[0].Latest == nil || ds.Items[0].Latest.Number != 2 || ds.Items[0].Latest.Images != 2 {
		t.Errorf("datasets: %+v", ds.Items)
	}

	// trocar a query não mexe nas versões; com versões não apaga
	assertStatus(t, env.doJSON("PATCH", "/datasets/motos-noite", `{"profile": "capacete-moto", "query": {}}`), http.StatusOK)
	assertStatus(t, env.doJSON("PATCH", "/datasets/motos-noite", `{"name": "outro", "query": {}}`), http.StatusBadRequest)
	assertStatus(t, env.do("GET", "/datasets/motos-noite/versions/1", nil, nil), http.StatusOK)
	assertStatus(t, env.do("GET", "/datasets/motos-noite/versions/9", nil, nil), http.StatusNotFound)
	assertStatus(t, env.do("GET", "/datasets/motos-noite/versions/x", nil, nil), http.StatusBadRequest)
	assertStatus(t, env.do("DELETE", "/datasets/motos-noite", nil, nil), http.StatusConflict)
	assertStatus(t, env.do("DELETE", "/datasets/motos-noite?versoes=apagar", nil, nil), http.StatusNoContent)
	assertStatus(t, env.do("GET", "/datasets/motos-noite", nil, nil), http.StatusNotFound)
}

// Baixar uma versão: o zip sai com a anotação CONGELADA (mudar as caixas
// depois não muda a v1), e o "agora" (number 0) com a de hoje.
func TestExportDeVersao(t *testing.T) {
	env := newTestEnv(t)
	n1, n2, _ := seedExport(t, env)
	assertStatus(t, env.do("POST", "/datasets", strings.NewReader(`{"name": "noite", "profile": "veiculos",
		"query": {"filters": [{"key": "fields.day_time", "value": "night"}]}}`),
		map[string]string{"Content-Type": "application/json", "X-User": "nathan"}), http.StatusCreated)
	assertStatus(t, env.do("POST", "/datasets/noite/versions", strings.NewReader(`{"message": "v1"}`),
		map[string]string{"Content-Type": "application/json", "X-User": "nathan"}), http.StatusCreated)

	// depois da v1: n1 troca as caixas e n2 vira de dia
	putTask(env, exportTask, n1, "moto")
	assertStatus(t, env.doJSON("PUT", "/images/"+n2, completeBody(func(b map[string]any) { b["day_time"] = "day" })), http.StatusOK)

	v1, code, msg := preview(t, env, `{"profile": "yolo", "tasks": ["veiculos"], "version": {"dataset": "noite", "number": 1}}`)
	if code != http.StatusOK {
		t.Fatalf("preview v1: %d %s", code, msg)
	}
	if v1.Version == nil || v1.Version.Number != 1 || v1.Total != 3 || v1.Boxes["carro"] != 1 || v1.Boxes["moto"] != 0 {
		t.Errorf("v1 (congelada): total %d, caixas %v, versão %+v", v1.Total, v1.Boxes, v1.Version)
	}
	now, code, msg := preview(t, env, `{"profile": "yolo", "tasks": ["veiculos"], "version": {"dataset": "noite", "number": 0}}`)
	if code != http.StatusOK {
		t.Fatalf("preview agora: %d %s", code, msg)
	}
	if now.Total != 2 || now.Boxes["moto"] != 1 || now.Boxes["carro"] != 0 {
		t.Errorf("agora: total %d, caixas %v", now.Total, now.Boxes)
	}
	// sem tarefa no pedido: as da versão
	if all, code, _ := preview(t, env, `{"profile": "rf-detr", "version": {"dataset": "noite", "number": 1}}`); code != http.StatusOK || len(all.Tasks) != 2 {
		t.Errorf("sem tarefa: %d %v", code, all.Tasks)
	}

	// erros: tarefa fora da versão, versão e dataset que não existem
	if _, code, msg := preview(t, env, `{"profile": "yolo", "tasks": ["nao-existe"], "version": {"dataset": "noite", "number": 1}}`); code != http.StatusBadRequest || !strings.Contains(msg, "versão 1") {
		t.Errorf("tarefa fora: %d %s", code, msg)
	}
	if _, code, _ := preview(t, env, `{"profile": "yolo", "version": {"dataset": "noite", "number": 9}}`); code != http.StatusNotFound {
		t.Errorf("versão 9: %d", code)
	}
	if _, code, _ := preview(t, env, `{"profile": "yolo", "version": {"dataset": "nao-existe", "number": 0}}`); code != http.StatusNotFound {
		t.Errorf("dataset: %d", code)
	}

	// o zip: nome com a versão e o export_info.json dizendo qual
	form := url.Values{"request": {`{"profile": "yolo", "tasks": ["veiculos"], "version": {"dataset": "noite", "number": 1}}`}}
	rec := env.do("POST", "/exports/download", strings.NewReader(form.Encode()),
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
	assertStatus(t, rec, http.StatusOK)
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, `filename="noite-v1-yolo.zip"`) {
		t.Errorf("Content-Disposition = %q", cd)
	}
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var info struct {
		Info struct {
			Version exports.VersionInfo `json:"version"`
		} `json:"info"`
		Result export.Result `json:"result"`
	}
	images := 0
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, "images/") && strings.HasSuffix(f.Name, ".jpg") {
			images++
		}
		if f.Name == "export_info.json" {
			rc, _ := f.Open()
			_ = json.NewDecoder(rc).Decode(&info)
			rc.Close()
		}
	}
	if images != 3 || info.Info.Version.Dataset != "noite" || info.Info.Version.Number != 1 || info.Info.Version.ID == "" || len(info.Result.Missing) != 0 {
		t.Errorf("zip: %d imagens, info %+v, faltando %v", images, info.Info.Version, info.Result.Missing)
	}
}
