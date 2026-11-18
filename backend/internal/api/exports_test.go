package api

import (
	"SIDM/internal/catalog"
	"SIDM/internal/exports"
	"SIDM/internal/platform"
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"testing"

	"SIDM/internal/boxes"
	"SIDM/internal/export"
)

const exportTask = "veiculos"

// seedExport, na tarefa "veiculos": 3 verificadas com caixas (2 de noite, 1
// de dia), 1 verificada fora da tarefa, 1 pendente com caixas (não entra) e
// 1 de outro pacote. Mais uma verificada só em OUTRA tarefa ("capacete").
func seedExport(t *testing.T, env *testEnv) (night1, night2, day string) {
	t.Helper()
	env.box.Tasks[exportTask] = boxes.Task{Name: exportTask}
	env.box.Tasks["capacete"] = boxes.Task{Name: "capacete"}
	verify := func(content, dayTime string) string {
		sha := env.seedPending(t, content)
		assertStatus(t, env.doJSON("PUT", "/images/"+sha, completeBody(func(b map[string]any) { b["day_time"] = dayTime })), http.StatusOK)
		return sha
	}
	put := func(sha string, labels ...string) {
		putTask(env, exportTask, sha, labels...)
	}
	night1, night2, day = verify("n1", "night"), verify("n2", "night"), verify("d1", "day")
	put(night1, "placa", "carro")
	put(night2, "Placa")
	put(day, "moto")
	verify("sem-caixa", "night")
	pending := env.seedPending(t, "pendente")
	put(pending, "placa")

	env.cat.Packages["outro"] = catalog.Package{Name: "outro"}
	other := env.uploadTo(t, "outro", upFile{"o.jpg", "outro"})[0].SHA1
	assertStatus(t, env.doJSON("PUT", "/images/"+other, completeBody()), http.StatusOK)
	put(other, "placa")

	putTask(env, "capacete", verify("so-capacete", "night"), "com_capacete")
	return
}

func putTask(env *testEnv, task, sha string, labels ...string) {
	set := boxes.Set{Task: task, ImageID: sha, Package: env.img.Images[sha].Package, Width: 100, Height: 50, Boxes: []boxes.Box{}}
	for _, l := range labels {
		set.Boxes = append(set.Boxes, boxes.Box{Label: l, X: 10, Y: 10, W: 20, H: 10})
	}
	env.box.Sets[boxes.Key(task, sha)] = set
}

func preview(t *testing.T, env *testEnv, body string) (exports.Preview, int, string) {
	t.Helper()
	rec := env.doJSON("POST", "/exports/preview", body)
	var p exports.Preview
	if rec.Code == http.StatusOK {
		decode(t, rec, &p)
		return p, rec.Code, ""
	}
	return p, rec.Code, errorMsg(t, rec)
}

func TestExportPreviewPadrao(t *testing.T) {
	env := newTestEnv(t)
	seedExport(t, env)

	p, code, msg := preview(t, env, `{"profile":"rf-detr","task":"veiculos"}`)
	if code != http.StatusOK {
		t.Fatalf("%d %s", code, msg)
	}
	// 3 do pacote + 1 do outro = 4 verificadas na tarefa; a pendente não
	// entra; "sem-caixa" e "so-capacete" não estão nesta tarefa
	if p.Total != 4 || p.Skipped.NotAnnotated != 2 || p.Format != "coco" || p.Task != exportTask || p.Negatives != 0 {
		t.Errorf("preview = %+v", p)
	}
	if strings.Join(p.Classes, ",") != "Placa,carro,moto,placa" {
		t.Errorf("sem regras, as labels vêm como estão: %v", p.Classes)
	}
	sum := 0
	for _, n := range p.Images {
		sum += n
	}
	if sum != 4 {
		t.Errorf("imagens por grupo = %v", p.Images)
	}
}

func TestExportPreviewComRegras(t *testing.T) {
	env := newTestEnv(t)
	seedExport(t, env)
	prof := map[string]any{
		"name": "placas-noite", "format": "yolo", "split": map[string]int{"train": 80, "valid": 20, "test": 0},
		"packages": []string{testPkg},
		"filters":  map[string]string{"day_time": "night"},
		"labels":   map[string]any{"rename": map[string]string{"Placa": "placa"}, "include": []string{"placa"}},
	}
	prof["task"] = exportTask // a tarefa pode vir no próprio perfil
	body, _ := json.Marshal(map[string]any{"profile": prof})
	p, code, msg := preview(t, env, string(body))
	if code != http.StatusOK {
		t.Fatalf("%d %s", code, msg)
	}
	// noite, só este pacote: n1 (placa, carro) e n2 (Placa -> placa); sem-caixa pulada
	if p.Total != 2 || p.Boxes["placa"] != 2 || p.Boxes["carro"] != 0 || strings.Join(p.Classes, ",") != "placa" {
		t.Errorf("preview = %+v", p)
	}
	if p.Skipped.NotAnnotated != 2 || p.Images["test"] != 0 {
		t.Errorf("skipped=%+v images=%v", p.Skipped, p.Images)
	}

	// só "moto": as de noite ficam sem label nenhuma
	prof["labels"] = map[string]any{"include": []string{"moto"}}
	body, _ = json.Marshal(map[string]any{"profile": prof})
	p, _, _ = preview(t, env, string(body))
	if p.Total != 0 || p.Skipped.WithoutLabels != 2 {
		t.Errorf("só moto: %+v", p)
	}
}

func TestExportOutraTarefaENegativas(t *testing.T) {
	env := newTestEnv(t)
	n1, _, _ := seedExport(t, env)
	// n1 também na tarefa capacete, anotada sem nenhuma caixa (negativa)
	putTask(env, "capacete", n1)

	p, code, msg := preview(t, env, `{"profile":"yolo","task":"capacete"}`)
	if code != http.StatusOK {
		t.Fatalf("%d %s", code, msg)
	}
	if p.Total != 2 || p.Negatives != 1 || p.Boxes["com_capacete"] != 1 || strings.Join(p.Classes, ",") != "com_capacete" {
		t.Errorf("preview = %+v", p)
	}
	// "task" do pedido vale mais que a do perfil
	body := `{"profile":{"name":"x","format":"coco","split":{"train":70,"valid":15,"test":15},"task":"veiculos"},"task":"capacete"}`
	if p, _, _ := preview(t, env, body); p.Task != "capacete" || p.Total != 2 {
		t.Errorf("task do pedido: %+v", p)
	}
}

func TestExportErros(t *testing.T) {
	env := newTestEnv(t)
	env.box.Tasks["veiculos"] = boxes.Task{Name: "veiculos"}
	cases := map[string]struct {
		body string
		code int
	}{
		"perfil inexistente": {`{"profile":"nao-existe","task":"veiculos"}`, http.StatusNotFound},
		"sem tarefa":         {`{"profile":"yolo"}`, http.StatusBadRequest},
		"tarefa inexistente": {`{"profile":"yolo","task":"sumiu"}`, http.StatusBadRequest},
		"sem perfil":         {`{}`, http.StatusBadRequest},
		"split inválido":     {`{"profile":{"name":"x","format":"coco","split":{"train":50,"valid":10,"test":10}}}`, http.StatusBadRequest},
		"campo errado":       {`{"profile":{"name":"x","formato":"coco"}}`, http.StatusBadRequest},
		"pacote inexistente": {`{"profile":{"name":"x","format":"coco","split":{"train":70,"valid":15,"test":15},"packages":["sumiu"]},"task":"veiculos"}`, http.StatusBadRequest},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, code, msg := preview(t, env, tc.body); code != tc.code {
				t.Errorf("code = %d (%s), want %d", code, msg, tc.code)
			}
		})
	}
	// nada para exportar
	assertStatus(t, env.doJSON("POST", "/exports/download", `{"profile":"yolo","task":"veiculos"}`), http.StatusBadRequest)
}

func TestExportDownloadYOLOPorFormulario(t *testing.T) {
	env := newTestEnv(t)
	n1, n2, day := seedExport(t, env)

	// o front manda por formulário, para o navegador baixar direto para o disco
	form := url.Values{"request": {`{"profile":"yolo","task":"veiculos"}`}}
	rec := env.do("POST", "/exports/download", strings.NewReader(form.Encode()),
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
	assertStatus(t, rec, http.StatusOK)
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, `filename="yolo-`) {
		t.Errorf("Content-Disposition = %q", cd)
	}
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		files[f.Name] = string(b)
	}
	for _, sha := range []string{n1, n2, day} {
		split := export.SplitOf(sha, export.Split{Train: 70, Valid: 15, Test: 15})
		if split == "valid" {
			split = "val"
		}
		img := env.img.Images[sha]
		if _, ok := files["images/"+split+"/"+img.FileName]; !ok {
			t.Errorf("%s deveria estar em images/%s/ (arquivos: %d)", img.FileName, split, len(files))
		}
		label := "labels/" + split + "/" + strings.TrimSuffix(img.FileName, ".jpg") + ".txt"
		if files[label] == "" {
			t.Errorf("faltou %s", label)
		}
	}
	if !strings.Contains(files["data.yaml"], "nc: 4") || files["export_info.json"] == "" {
		t.Errorf("data.yaml:\n%s", files["data.yaml"])
	}
}

func TestPerfisDeExport(t *testing.T) {
	env := newTestEnv(t)
	prof := `{"name":"meu-yolo","format":"yolo","split":{"train":80,"valid":20,"test":0},"labels":{"include":["placa"]}}`

	assertStatus(t, env.doJSON("POST", "/export-profiles", prof), http.StatusCreated)
	assertStatus(t, env.doJSON("POST", "/export-profiles", prof), http.StatusConflict)
	if env.exp.Profiles["meu-yolo"].CreatedBy == "" {
		t.Error("deveria guardar quem criou")
	}

	var list platform.Page[export.Profile]
	decode(t, env.do("GET", "/export-profiles", nil, nil), &list)
	names := []string{}
	for _, p := range list.Items {
		names = append(names, p.Name)
	}
	if strings.Join(names, ",") != "rf-detr,yolo,voc,voc-pastas,meu-yolo" || !list.Items[0].Builtin {
		t.Errorf("lista = %v", names)
	}

	assertStatus(t, env.do("GET", "/export-profiles/rf-detr", nil, nil), http.StatusOK)
	assertStatus(t, env.doJSON("PUT", "/export-profiles/meu-yolo", strings.Replace(prof, "80", "70", 1)), http.StatusBadRequest) // não soma 100
	assertStatus(t, env.doJSON("PUT", "/export-profiles/meu-yolo", strings.Replace(strings.Replace(prof, "80", "70", 1), `"test":0`, `"test":10`, 1)), http.StatusOK)
	if env.exp.Profiles["meu-yolo"].Split.Test != 10 {
		t.Error("PUT deveria ter atualizado")
	}

	// os padrões não podem ser criados, alterados nem apagados
	assertStatus(t, env.doJSON("POST", "/export-profiles", `{"name":"yolo","format":"yolo","split":{"train":70,"valid":15,"test":15},"builtin":true}`), http.StatusBadRequest)
	assertStatus(t, env.doJSON("PUT", "/export-profiles/yolo", `{"name":"yolo","format":"yolo","split":{"train":70,"valid":15,"test":15}}`), http.StatusBadRequest)
	assertStatus(t, env.do("DELETE", "/export-profiles/rf-detr", nil, nil), http.StatusBadRequest)

	assertStatus(t, env.do("DELETE", "/export-profiles/meu-yolo", nil, nil), http.StatusOK)
	assertStatus(t, env.do("DELETE", "/export-profiles/meu-yolo", nil, nil), http.StatusNotFound)
}

func TestExportVOCPorPastas(t *testing.T) {
	env := newTestEnv(t)
	n1, _, day := seedExport(t, env)

	p, code, msg := preview(t, env, `{"profile":"voc-pastas","task":"veiculos"}`)
	if code != http.StatusOK {
		t.Fatalf("%d %s", code, msg)
	}
	// completeBody: caminhão, 1920x1080, mercosul, dianteira; 3 de noite (2 + a do outro pacote) e 1 de dia
	night, dayDir := "Truck/08-1920x1080/Mercosul/Dianteiro/Noturno", "Truck/08-1920x1080/Mercosul/Dianteiro/Diurno"
	if p.Layout != "folders" || p.Total != 4 || p.Folders[night] != 3 || p.Folders[dayDir] != 1 {
		t.Errorf("preview = %+v", p)
	}
	// nome no padrão antigo: sha1 + 12 códigos + placa
	legacy := regexp.MustCompile(`^Truck/08-1920x1080/Mercosul/Dianteiro/(Noturno|Diurno)/[0-9a-f]{40}F08[A-Z_]{5}\d[A-Z_]\d\d[A-Z0-9]{7}\.jpg$`)
	if len(p.Examples) != 3 || !legacy.MatchString(p.Examples[0]) {
		t.Errorf("exemplos = %v", p.Examples)
	}

	form := url.Values{"request": {`{"profile":"voc-pastas","task":"veiculos"}`}}
	rec := env.do("POST", "/exports/download", strings.NewReader(form.Encode()),
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
	assertStatus(t, rec, http.StatusOK)
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, `filename="voc-pastas-`) {
		t.Errorf("nome do zip = %q", cd)
	}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	legacyName := func(sha string) string {
		for name := range names {
			if strings.Contains(name, "/"+sha) && strings.HasSuffix(name, ".jpg") {
				return name
			}
		}
		return ""
	}
	n1Name := legacyName(n1)
	if !legacy.MatchString(n1Name) || !strings.HasPrefix(n1Name, night+"/") {
		t.Errorf("nome de n1 no zip = %q", n1Name)
	}
	for _, f := range []string{
		strings.TrimSuffix(n1Name, ".jpg") + ".xml",
		dayDir + "/" + path.Base(legacyName(day)),
		"labels.txt", "export_info.json",
	} {
		if !names[f] {
			t.Errorf("faltou %s no zip", f)
		}
	}

	// pastas customizadas: só um nível, com nome próprio e pasta de desconhecido
	body := `{"profile":{"name":"x","format":"voc","split":{"train":100,"valid":0,"test":0},"layout":"folders",
		"folders":[{"field":"weather","names":{"rain":"Chuva"}},{"field":"plate_category","unknown":"sem-categoria"}]},"task":"veiculos"}`
	if p, code, msg := preview(t, env, body); code != http.StatusOK || p.Folders["Chuva/private"] != 4 {
		t.Errorf("custom: %d %s %+v", code, msg, p.Folders)
	}
	// pastas no COCO: recusado
	body = `{"profile":{"name":"x","format":"coco","split":{"train":70,"valid":15,"test":15},"layout":"folders","folders":[{"field":"weather"}]},"task":"veiculos"}`
	if _, code, _ := preview(t, env, body); code != http.StatusBadRequest {
		t.Errorf("pastas no coco: %d", code)
	}

	var fields struct {
		Items []struct {
			Field  string            `json:"field"`
			Values map[string]string `json:"values"`
		} `json:"items"`
	}
	decode(t, env.do("GET", "/export-folder-fields", nil, nil), &fields)
	if len(fields.Items) == 0 || fields.Items[0].Field != "vehicle_type" || fields.Items[0].Values["car"] != "car" {
		t.Errorf("campos = %+v", fields.Items)
	}
}

func TestPadraoInlineMantemONome(t *testing.T) {
	env := newTestEnv(t)
	seedExport(t, env)
	// a tela manda o perfil inteiro; sem mudanças, é o padrão
	b, _ := export.Builtin("yolo")
	b.Task = exportTask
	body, _ := json.Marshal(map[string]any{"profile": b})
	if p, code, msg := preview(t, env, string(body)); code != http.StatusOK || p.Profile != "yolo" {
		t.Errorf("sem mudanças: %d %s %q", code, msg, p.Profile)
	}
	b.Split = export.Split{Train: 80, Valid: 20}
	body, _ = json.Marshal(map[string]any{"profile": b})
	if p, _, _ := preview(t, env, string(body)); p.Profile != "personalizado" {
		t.Errorf("com mudança: %q", p.Profile)
	}
}
