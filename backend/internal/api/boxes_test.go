package api

import (
	"SIDM/internal/boxes"
	"SIDM/internal/catalog"
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

// realJPEG gera um JPEG de verdade (o import confere o tamanho da imagem).
// seed muda a cor, para cada imagem ter um sha1 diferente.
func realJPEG(t *testing.T, w, h int, seed uint8) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{seed, uint8(x), uint8(y), 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

type cocoImg struct {
	name  string
	w, h  int
	boxes [][4]float64 // label = "placa" na 1ª, "carro" nas outras
}

// cocoJSON monta um export COCO 1.0 no formato do CVAT.
func cocoJSON(imgs ...cocoImg) string {
	doc := map[string]any{
		"licenses":   []any{map[string]any{"name": "", "id": 0, "url": ""}},
		"info":       map[string]any{"contributor": ""},
		"categories": []any{map[string]any{"id": 1, "name": "placa"}, map[string]any{"id": 2, "name": "carro"}},
	}
	var images, anns []any
	n := 0
	for i, img := range imgs {
		images = append(images, map[string]any{"id": i + 1, "width": img.w, "height": img.h, "file_name": img.name})
		for j, b := range img.boxes {
			n++
			cat := 2
			if j == 0 {
				cat = 1
			}
			anns = append(anns, map[string]any{
				"id": n, "image_id": i + 1, "category_id": cat, "bbox": b[:], "iscrowd": 0, "segmentation": []any{},
				"attributes": map[string]any{"occluded": j == 1, "rotation": 0.0, "caractere": "A"},
			})
		}
	}
	doc["images"], doc["annotations"] = images, anns
	out, _ := json.Marshal(doc)
	return string(out)
}

type boxReport struct {
	DryRun        bool                 `json:"dry_run"`
	Task          string               `json:"task"`
	FileImages    int                  `json:"file_images"`
	FileBoxes     int                  `json:"file_boxes"`
	Labels        map[string]int       `json:"labels"`
	NewLabels     []string             `json:"new_labels"`
	Matched       int                  `json:"matched"`
	Packages      map[string]int       `json:"packages"`
	NotFound      []string             `json:"not_found"`
	NotFoundCount int                  `json:"not_found_count"`
	Ambiguous     []string             `json:"ambiguous"`
	Empty         int                  `json:"empty"`
	SizeMismatch  []boxes.SizeMismatch `json:"size_mismatch"`
	WillApply     int                  `json:"will_apply"`
	Replaced      int                  `json:"replaced"`
	Applied       int                  `json:"applied"`
	AppliedBoxes  int                  `json:"applied_boxes"`

	AttributeProblems     []string `json:"attribute_problems"`
	AttributeProblemCount int      `json:"attribute_problem_count"`
	MissingRequired       int      `json:"missing_required"`
	UntypedLabels         []string `json:"untyped_labels"`
	EditedReplaced        int      `json:"edited_replaced"`
}

func (e *testEnv) importBoxes(t *testing.T, task, query, fileName, content string) (boxReport, int) {
	t.Helper()
	rec := e.postFiles(t, "/tasks/"+task+"/import"+query, upFile{fileName, content})
	var rep boxReport
	if rec.Code == http.StatusOK {
		decode(t, rec, &rep)
	}
	return rep, rec.Code
}

func (e *testEnv) boxSet(task, sha string) (boxes.Set, bool) {
	return e.box.Get(task, sha)
}

func TestImportarCaixasDoCVAT(t *testing.T) {
	env := newTestEnv(t)
	assertStatus(t, env.doJSON("POST", "/tasks", `{"name":"veiculos","labels":["placa"]}`), http.StatusCreated)

	// a: casa pelo sha1 (o nome que o zip do SIDM manda para o CVAT)
	a := env.seedNamed(t, "a.jpg", realJPEG(t, 64, 48, 1))
	// b: verificada (nome v2, que também começa pelo sha1)
	b := env.seedNamed(t, "b.jpg", realJPEG(t, 64, 48, 2))
	assertStatus(t, env.doJSON("PUT", "/images/"+b, completeBody()), http.StatusOK)
	// c: casa pelo nome original (alguém subiu no CVAT com o nome original)
	c := env.seedNamed(t, "foto-c.jpg", realJPEG(t, 64, 48, 3))
	// d: nome original repetido -> ambíguo
	env.seedNamed(t, "dup.jpg", realJPEG(t, 64, 48, 4))
	env.seedNamed(t, "dup.jpg", realJPEG(t, 64, 48, 5))
	// e: tamanho diferente no CVAT; f: sem caixa nenhuma
	e := env.seedNamed(t, "e.jpg", realJPEG(t, 64, 48, 6))
	f := env.seedNamed(t, "f.jpg", realJPEG(t, 64, 48, 7))
	// imagem de OUTRO pacote: o import procura no banco todo, então casa
	env.cat.Packages["outro"] = catalog.Package{Name: "outro"}
	other := env.uploadTo(t, "outro", upFile{"x.jpg", realJPEG(t, 64, 48, 8)})[0].SHA1

	coco := cocoJSON(
		cocoImg{a + ".jpg", 64, 48, [][4]float64{{10, 10, 20, 8}, {0, 0, 64, 40}}},
		cocoImg{env.img.Images[b].FileName, 64, 48, [][4]float64{{5, 5, 10, 5}}},
		cocoImg{"pasta/foto-c.jpg", 64, 48, [][4]float64{{1, 1, 5, 5}}},
		cocoImg{"dup.jpg", 64, 48, [][4]float64{{1, 1, 5, 5}}},
		cocoImg{e + ".jpg", 1280, 720, [][4]float64{{1, 1, 5, 5}}},
		cocoImg{f + ".jpg", 64, 48, nil},
		cocoImg{other + ".jpg", 64, 48, [][4]float64{{1, 1, 5, 5}}},
		cocoImg{"nao-existe.jpg", 64, 48, [][4]float64{{1, 1, 5, 5}}},
	)

	t.Run("ensaio não grava", func(t *testing.T) {
		rep, code := env.importBoxes(t, "veiculos", "", "task.json", coco)
		if code != http.StatusOK || !rep.DryRun || rep.Applied != 0 || len(env.box.Sets) != 0 || rep.Task != "veiculos" {
			t.Fatalf("code=%d rep=%+v boxSets=%d", code, rep, len(env.box.Sets))
		}
		if rep.FileImages != 8 || rep.Matched != 6 || rep.NotFoundCount != 1 || len(rep.Ambiguous) != 1 || rep.Empty != 1 || rep.WillApply != 4 {
			t.Errorf("casamento: %+v", rep)
		}
		if rep.Packages[testPkg] != 3 || rep.Packages["outro"] != 1 {
			t.Errorf("por pacote: %v", rep.Packages)
		}
		if len(rep.SizeMismatch) != 1 || rep.SizeMismatch[0].CVAT != "1280x720" || rep.SizeMismatch[0].SIDM != "64x48" {
			t.Errorf("tamanho: %+v", rep.SizeMismatch)
		}
		// só as caixas das imagens que vão ser gravadas: a(2) + b(1) + c(1) + other(1)
		if rep.Labels["placa"] != 4 || rep.Labels["carro"] != 1 {
			t.Errorf("labels = %v", rep.Labels)
		}
		if strings.Join(rep.NewLabels, ",") != "carro" {
			t.Errorf("labels novas = %v", rep.NewLabels)
		}
		if task := env.box.Tasks["veiculos"]; len(task.Labels) != 1 {
			t.Error("o ensaio não pode mexer nas labels da tarefa")
		}
	})

	t.Run("aplicar", func(t *testing.T) {
		rep, code := env.importBoxes(t, "veiculos", "?dry_run=false", "task.json", coco)
		if code != http.StatusOK || rep.Applied != 4 || rep.AppliedBoxes != 5 || rep.Replaced != 0 {
			t.Fatalf("code=%d rep=%+v", code, rep)
		}
		set, _ := env.boxSet("veiculos", a)
		if set.Task != "veiculos" || set.Package != testPkg || set.Width != 64 || len(set.Boxes) != 2 || set.ImportedBy == "" ||
			set.Source.Tool != "cvat" || set.Source.File != "task.json" || set.Source.FileName != a+".jpg" {
			t.Errorf("set = %+v", set)
		}
		if bx := set.Boxes[1]; bx.Label != "carro" || !bx.Occluded || bx.Attributes["caractere"] != "A" || bx.W != 64 {
			t.Errorf("caixa = %+v", bx)
		}
		if _, ok := env.boxSet("veiculos", c); !ok {
			t.Error("c deveria casar pelo nome original (único no banco)")
		}
		if set, ok := env.boxSet("veiculos", other); !ok || set.Package != "outro" {
			t.Errorf("a do outro pacote deveria entrar com o pacote dela: %+v", set)
		}
		for _, sha := range []string{e, f} {
			if _, ok := env.boxSet("veiculos", sha); ok {
				t.Errorf("%s NÃO deveria estar na tarefa", sha)
			}
		}
		if labels := env.box.Tasks["veiculos"].Labels; strings.Join(labels, ",") != "placa,carro" {
			t.Errorf("labels da tarefa = %v (a nova vai no fim)", labels)
		}
	})

	t.Run("reimportar substitui", func(t *testing.T) {
		rep, _ := env.importBoxes(t, "veiculos", "?dry_run=false", "v2.json",
			cocoJSON(cocoImg{a + ".jpg", 64, 48, [][4]float64{{2, 2, 3, 3}}}))
		if rep.Replaced != 1 || rep.Applied != 1 {
			t.Errorf("rep = %+v", rep)
		}
		if set, _ := env.boxSet("veiculos", a); len(set.Boxes) != 1 || set.Source.File != "v2.json" {
			t.Errorf("deveria ter só a caixa nova: %+v", set)
		}
	})

	t.Run("outra tarefa não mexe nesta, e negativas", func(t *testing.T) {
		assertStatus(t, env.doJSON("POST", "/tasks", `{"name":"capacete"}`), http.StatusCreated)
		doc := cocoJSON(cocoImg{a + ".jpg", 64, 48, [][4]float64{{1, 1, 4, 4}}}, cocoImg{f + ".jpg", 64, 48, nil})
		rep, _ := env.importBoxes(t, "capacete", "?dry_run=false&negatives=true", "cap.json", doc)
		if rep.Applied != 2 || rep.Empty != 1 || rep.WillApply != 2 {
			t.Errorf("rep = %+v", rep)
		}
		if set, ok := env.boxSet("capacete", f); !ok || len(set.Boxes) != 0 {
			t.Errorf("f deveria entrar como negativa: %+v", set)
		}
		if set, _ := env.boxSet("veiculos", a); len(set.Boxes) != 1 || set.Source.File != "v2.json" {
			t.Errorf("a tarefa veiculos não podia mudar: %+v", set)
		}
	})

	t.Run("consultar", func(t *testing.T) {
		rec := env.do("GET", "/images/"+a+"/boxes", nil, nil)
		assertStatus(t, rec, http.StatusOK)
		var resp struct {
			Items []boxes.Set `json:"items"`
		}
		decode(t, rec, &resp)
		if len(resp.Items) != 2 || resp.Items[0].Task != "capacete" || resp.Items[1].Task != "veiculos" {
			t.Errorf("caixas da imagem = %+v", resp.Items)
		}
		decode(t, env.do("GET", "/images/"+e+"/boxes", nil, nil), &resp)
		if len(resp.Items) != 0 {
			t.Errorf("e não tem caixas: %+v", resp.Items)
		}

		rec = env.do("GET", "/packages/"+testPkg+"/boxes", nil, nil)
		assertStatus(t, rec, http.StatusOK)
		var sum struct {
			Tasks []boxes.PackageCount `json:"tasks"`
		}
		decode(t, rec, &sum)
		if len(sum.Tasks) != 2 || sum.Tasks[0].Images != 2 || sum.Tasks[1].Task != "veiculos" || sum.Tasks[1].Images != 3 {
			t.Errorf("resumo = %+v", sum)
		}
	})

	t.Run("apagar a imagem apaga as caixas", func(t *testing.T) {
		assertStatus(t, env.do("DELETE", "/images/"+a, nil, nil), http.StatusOK)
		for _, task := range []string{"veiculos", "capacete"} {
			if _, ok := env.boxSet(task, a); ok {
				t.Errorf("as caixas em %s deveriam ter sumido", task)
			}
		}
	})
}

func TestCaixasAcompanhamOPacote(t *testing.T) {
	env := newTestEnv(t)
	env.box.Tasks["t"] = boxes.Task{Name: "t"}
	a := env.seedNamed(t, "a.jpg", realJPEG(t, 32, 32, 1))
	env.importBoxes(t, "t", "?dry_run=false", "t.json", cocoJSON(cocoImg{a + ".jpg", 32, 32, [][4]float64{{1, 1, 2, 2}}}))

	assertStatus(t, env.doJSON("PATCH", "/packages/"+testPkg, `{"name":"novo"}`), http.StatusOK)
	if set, _ := env.boxSet("t", a); set.Package != "novo" {
		t.Error("renomear o pacote deveria levar as caixas junto")
	}
	assertStatus(t, env.do("DELETE", "/packages/novo?confirm=novo", nil, nil), http.StatusOK)
	if len(env.box.Sets) != 0 {
		t.Error("apagar o pacote deveria apagar as caixas")
	}
}

func TestImportarCaixasErros(t *testing.T) {
	env := newTestEnv(t)
	env.box.Tasks["t"] = boxes.Task{Name: "t"}
	if _, code := env.importBoxes(t, "nao-existe", "", "t.json", cocoJSON()); code != http.StatusNotFound {
		t.Errorf("tarefa inexistente: %d", code)
	}
	rec := env.postFiles(t, "/tasks/t/import", upFile{"t.json", "não é json"})
	assertStatus(t, rec, http.StatusBadRequest)
	rec = env.postFiles(t, "/tasks/t/import", upFile{"t.json", `{"foo":1}`})
	assertStatus(t, rec, http.StatusBadRequest)
	if !strings.Contains(errorMsg(t, rec), "COCO 1.0") {
		t.Errorf("a mensagem deveria dizer o formato esperado: %q", errorMsg(t, rec))
	}
	assertStatus(t, env.postFiles(t, "/tasks/t/import"), http.StatusBadRequest)
	assertStatus(t, env.do("GET", "/packages/nao-existe/boxes", nil, nil), http.StatusNotFound)
}

func TestZipParaOCVAT(t *testing.T) {
	env := newTestEnv(t)
	a := env.seedNamed(t, "a.jpg", realJPEG(t, 16, 16, 1))
	b := env.seedNamed(t, "b.jpg", realJPEG(t, 16, 16, 2))
	assertStatus(t, env.doJSON("PUT", "/images/"+b, completeBody()), http.StatusOK)

	rec := env.do("GET", "/packages/"+testPkg+"/cvat.zip", nil, nil)
	assertStatus(t, rec, http.StatusOK)
	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("Content-Type = %q", ct)
	}
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
		if f.Method != zip.Store {
			t.Errorf("%s: JPEG não precisa de compressão", f.Name)
		}
	}
	// no disco (e no zip) toda imagem é <sha1>.jpg, verificada ou não
	if !names[a+".jpg"] || !names[b+".jpg"] || len(names) != 2 {
		t.Errorf("arquivos no zip = %v", names)
	}
	// o que sai no zip volta casando no import (o nome começa pelo sha1)
	for name := range names {
		if !regexp.MustCompile(`^[0-9a-f]{40}`).MatchString(name) {
			t.Errorf("%s deveria começar pelo sha1", name)
		}
	}

	env.cat.Packages["vazio"] = catalog.Package{Name: "vazio"}
	assertStatus(t, env.do("GET", "/packages/vazio/cvat.zip", nil, nil), http.StatusNotFound)
	assertStatus(t, env.do("GET", "/packages/nao-existe/cvat.zip", nil, nil), http.StatusNotFound)
}

// helmetCOCO: um COCO do CVAT com caixas "helmet" (tipo com caixa do
// capacete-moto) e "pessoa" (sem tipo no perfil), com os atributos dados.
func helmetCOCO(name string, attrs ...map[string]any) string {
	doc := map[string]any{
		"categories": []any{map[string]any{"id": 1, "name": "helmet"}, map[string]any{"id": 2, "name": "pessoa"}},
		"images":     []any{map[string]any{"id": 1, "width": 32, "height": 32, "file_name": name}},
	}
	anns := []any{}
	for i, a := range attrs {
		cat := 1
		if i == len(attrs)-1 {
			cat = 2 // a última é "pessoa"
		}
		a["occluded"], a["rotation"] = false, 0.0
		anns = append(anns, map[string]any{"id": i + 1, "image_id": 1, "category_id": cat, "bbox": []float64{1, 1, 5, 5}, "attributes": a})
	}
	doc["annotations"] = anns
	out, _ := json.Marshal(doc)
	return string(out)
}

func TestAtributosDasCaixas(t *testing.T) {
	env := newTestEnv(t)
	env.withCapacete(t)
	assertStatus(t, env.doJSON("POST", "/tasks", `{"name":"capacete","labels":["helmet","pessoa"]}`), http.StatusCreated)
	sha := env.uploadTo(t, "motos", upFile{"m.jpg", realJPEG(t, 32, 32, 9)})[0].SHA1
	coco := helmetCOCO(sha+".jpg",
		map[string]any{"Cor": "Branco", "visor": "true"}, // pelo nome na tela; sim/não em texto
		map[string]any{"color": "azul", "tamanho": "G"},  // opção que não existe; campo que não existe
		map[string]any{"qualquer": "coisa"},              // pessoa: sem tipo, fica crua
	)

	// ensaio: o relatório já confere os atributos, e nada é gravado
	rep, code := env.importBoxes(t, "capacete", "", "c.json", coco)
	if code != http.StatusOK || rep.AttributeProblemCount != 2 || rep.MissingRequired != 1 || strings.Join(rep.UntypedLabels, ",") != "pessoa" {
		t.Fatalf("%d %+v", code, rep)
	}
	if _, ok := env.boxSet("capacete", sha); ok {
		t.Fatal("o ensaio gravou")
	}

	rep, _ = env.importBoxes(t, "capacete", "?dry_run=false", "c.json", coco)
	set, _ := env.boxSet("capacete", sha)
	if rep.Applied != 1 || fmt.Sprint(set.Boxes[0].Attributes) != "map[color:white visor:true]" ||
		set.Boxes[1].Attributes != nil || set.Boxes[2].Attributes["qualquer"] != "coisa" {
		t.Fatalf("caixas = %+v", set.Boxes)
	}

	put := func(version string, body string) *httptest.ResponseRecorder {
		return env.doJSON("PUT", "/images/"+sha+"/boxes/capacete", `{"version":"`+version+`","boxes":`+body+`}`)
	}
	version := set.ImportedAt.Format(time.RFC3339Nano)

	// atributo inválido: 400 com o caminho
	rec := put(version, `[{"attributes":{"color":"black"}},{"attributes":{"color":"azul"}},{"attributes":{}}]`)
	assertStatus(t, rec, http.StatusBadRequest)
	if !strings.Contains(rec.Body.String(), `"boxes[1].attributes.color"`) {
		t.Errorf("sem o caminho: %s", rec.Body.String())
	}
	// número de caixas errado: 400
	assertStatus(t, put(version, `[{"attributes":{}}]`), http.StatusBadRequest)

	// válido: grava; a caixa sem tipo fica como estava
	rec = put(version, `[{"attributes":{"color":"black","visor":false}},{"attributes":{"color":"white"}},{"attributes":{"x":"y"}}]`)
	assertStatus(t, rec, http.StatusOK)
	set, _ = env.boxSet("capacete", sha)
	if set.UpdatedAt == nil || set.Boxes[1].Attributes["color"] != "white" || set.Boxes[0].Attributes["visor"] != false ||
		set.Boxes[2].Attributes["qualquer"] != "coisa" {
		t.Fatalf("depois da edição: %+v", set)
	}
	// versão velha: 409
	assertStatus(t, put(version, `[{"attributes":{}},{"attributes":{}},{"attributes":{}}]`), http.StatusConflict)
	// tarefa sem caixas desta imagem: 404
	assertStatus(t, env.doJSON("PUT", "/images/"+sha+"/boxes/outra", `{"version":"`+version+`","boxes":[]}`), http.StatusNotFound)

	// reimport: avisa e sobrescreve (o CVAT é a fonte)
	rep, _ = env.importBoxes(t, "capacete", "?dry_run=false", "c.json", coco)
	set, _ = env.boxSet("capacete", sha)
	if rep.EditedReplaced != 1 || set.UpdatedAt != nil || set.Boxes[1].Attributes != nil {
		t.Errorf("reimport: %+v %+v", rep, set)
	}
}
