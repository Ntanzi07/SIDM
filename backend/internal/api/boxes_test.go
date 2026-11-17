package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"strings"
	"testing"

	"SIDM/internal/store"
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
	DryRun        bool           `json:"dry_run"`
	FileImages    int            `json:"file_images"`
	FileBoxes     int            `json:"file_boxes"`
	Labels        map[string]int `json:"labels"`
	Matched       int            `json:"matched"`
	NotFound      []string       `json:"not_found"`
	NotFoundCount int            `json:"not_found_count"`
	Ambiguous     []string       `json:"ambiguous"`
	Empty         int            `json:"empty"`
	SizeMismatch  []sizeMismatch `json:"size_mismatch"`
	Replaced      int            `json:"replaced"`
	Applied       int            `json:"applied"`
	AppliedBoxes  int            `json:"applied_boxes"`
}

func (e *testEnv) importBoxes(t *testing.T, pkg, query, fileName, content string) (boxReport, int) {
	t.Helper()
	rec := e.postFiles(t, "/packages/"+pkg+"/boxes/import"+query, upFile{fileName, content})
	var rep boxReport
	if rec.Code == http.StatusOK {
		decode(t, rec, &rep)
	}
	return rep, rec.Code
}

func TestImportarCaixasDoCVAT(t *testing.T) {
	env := newTestEnv(t)
	// a: casa pelo sha1 (o nome que o zip do SIDM manda para o CVAT)
	a := env.seedNamed(t, "a.jpg", realJPEG(t, 64, 48, 1))
	// b: verificada (nome v2, que também começa pelo sha1)
	b := env.seedNamed(t, "b.jpg", realJPEG(t, 64, 48, 2))
	assertStatus(t, env.doJSON("PUT", "/images/"+b, completeBody()), http.StatusOK)
	// c: casa pelo nome original (alguém subiu no CVAT com o nome original)
	c := env.seedNamed(t, "foto-c.jpg", realJPEG(t, 64, 48, 3))
	// d: nome original repetido no pacote -> ambíguo
	env.seedNamed(t, "dup.jpg", realJPEG(t, 64, 48, 4))
	env.seedNamed(t, "dup.jpg", realJPEG(t, 64, 48, 5))
	// e: tamanho diferente no CVAT; f: sem caixa nenhuma
	e := env.seedNamed(t, "e.jpg", realJPEG(t, 64, 48, 6))
	f := env.seedNamed(t, "f.jpg", realJPEG(t, 64, 48, 7))
	// imagem de OUTRO pacote, com o sha1 no nome: não pode casar
	env.db.packages["outro"] = store.Package{Name: "outro"}
	other := env.uploadTo(t, "outro", upFile{"x.jpg", realJPEG(t, 64, 48, 8)})[0].SHA1

	coco := cocoJSON(
		cocoImg{a + ".jpg", 64, 48, [][4]float64{{10, 10, 20, 8}, {0, 0, 64, 40}}},
		cocoImg{env.db.images[b].FileName, 64, 48, [][4]float64{{5, 5, 10, 5}}},
		cocoImg{"pasta/foto-c.jpg", 64, 48, [][4]float64{{1, 1, 5, 5}}},
		cocoImg{"dup.jpg", 64, 48, [][4]float64{{1, 1, 5, 5}}},
		cocoImg{e + ".jpg", 1280, 720, [][4]float64{{1, 1, 5, 5}}},
		cocoImg{f + ".jpg", 64, 48, nil},
		cocoImg{other + ".jpg", 64, 48, [][4]float64{{1, 1, 5, 5}}},
		cocoImg{"nao-existe.jpg", 64, 48, [][4]float64{{1, 1, 5, 5}}},
	)

	t.Run("ensaio não grava", func(t *testing.T) {
		rep, code := env.importBoxes(t, testPkg, "", "task.json", coco)
		if code != http.StatusOK || !rep.DryRun || rep.Applied != 0 || len(env.db.boxSets) != 0 {
			t.Fatalf("code=%d rep=%+v boxSets=%d", code, rep, len(env.db.boxSets))
		}
		if rep.FileImages != 8 || rep.Matched != 5 || rep.NotFoundCount != 2 || len(rep.Ambiguous) != 1 || rep.Empty != 1 {
			t.Errorf("casamento: %+v", rep)
		}
		if len(rep.SizeMismatch) != 1 || rep.SizeMismatch[0].CVAT != "1280x720" || rep.SizeMismatch[0].SIDM != "64x48" {
			t.Errorf("tamanho: %+v", rep.SizeMismatch)
		}
		// só as caixas das imagens que vão ser gravadas: a(2) + b(1) + c(1)
		if rep.Labels["placa"] != 3 || rep.Labels["carro"] != 1 {
			t.Errorf("labels = %v", rep.Labels)
		}
	})

	t.Run("aplicar", func(t *testing.T) {
		env.db.mu.Lock()
		before := len(env.db.boxSets)
		env.db.mu.Unlock()
		rep, code := env.importBoxes(t, testPkg, "?dry_run=false", "task.json", coco)
		if code != http.StatusOK || rep.Applied != 3 || rep.AppliedBoxes != 4 || rep.Replaced != 0 || before != 0 {
			t.Fatalf("code=%d rep=%+v", code, rep)
		}
		set := env.db.boxSets[a]
		if set.Package != testPkg || set.Width != 64 || len(set.Boxes) != 2 || set.ImportedBy == "" ||
			set.Source.Tool != "cvat" || set.Source.File != "task.json" || set.Source.FileName != a+".jpg" {
			t.Errorf("set = %+v", set)
		}
		if bx := set.Boxes[1]; bx.Label != "carro" || !bx.Occluded || bx.Attributes["caractere"] != "A" || bx.W != 64 {
			t.Errorf("caixa = %+v", bx)
		}
		for _, sha := range []string{b, c} {
			if _, ok := env.db.boxSets[sha]; !ok {
				t.Errorf("%s deveria ter caixas", sha)
			}
		}
		for _, sha := range []string{e, f, other} {
			if _, ok := env.db.boxSets[sha]; ok {
				t.Errorf("%s NÃO deveria ter caixas", sha)
			}
		}
	})

	t.Run("reimportar substitui", func(t *testing.T) {
		rep, _ := env.importBoxes(t, testPkg, "?dry_run=false", "v2.json",
			cocoJSON(cocoImg{a + ".jpg", 64, 48, [][4]float64{{2, 2, 3, 3}}}))
		if rep.Replaced != 1 || rep.Applied != 1 {
			t.Errorf("rep = %+v", rep)
		}
		if set := env.db.boxSets[a]; len(set.Boxes) != 1 || set.Source.File != "v2.json" {
			t.Errorf("deveria ter só a caixa nova: %+v", set)
		}
	})

	t.Run("consultar", func(t *testing.T) {
		rec := env.do("GET", "/images/"+a+"/boxes", nil, nil)
		assertStatus(t, rec, http.StatusOK)
		var set store.BoxSet
		decode(t, rec, &set)
		if set.ImageID != a || len(set.Boxes) != 1 {
			t.Errorf("set = %+v", set)
		}
		assertStatus(t, env.do("GET", "/images/"+f+"/boxes", nil, nil), http.StatusNotFound)

		rec = env.do("GET", "/packages/"+testPkg+"/boxes", nil, nil)
		assertStatus(t, rec, http.StatusOK)
		var sum struct {
			Images int64            `json:"images"`
			Labels map[string]int64 `json:"labels"`
		}
		decode(t, rec, &sum)
		if sum.Images != 3 || sum.Labels["placa"] != 3 {
			t.Errorf("resumo = %+v", sum)
		}
	})

	t.Run("apagar a imagem apaga as caixas", func(t *testing.T) {
		assertStatus(t, env.do("DELETE", "/images/"+c, nil, nil), http.StatusOK)
		if _, ok := env.db.boxSets[c]; ok {
			t.Error("as caixas deveriam ter sumido")
		}
	})
}

func TestCaixasAcompanhamOPacote(t *testing.T) {
	env := newTestEnv(t)
	a := env.seedNamed(t, "a.jpg", realJPEG(t, 32, 32, 1))
	env.importBoxes(t, testPkg, "?dry_run=false", "t.json", cocoJSON(cocoImg{a + ".jpg", 32, 32, [][4]float64{{1, 1, 2, 2}}}))

	assertStatus(t, env.doJSON("PATCH", "/packages/"+testPkg, `{"name":"novo"}`), http.StatusOK)
	if env.db.boxSets[a].Package != "novo" {
		t.Error("renomear o pacote deveria levar as caixas junto")
	}
	assertStatus(t, env.do("DELETE", "/packages/novo?confirm=novo", nil, nil), http.StatusOK)
	if len(env.db.boxSets) != 0 {
		t.Error("apagar o pacote deveria apagar as caixas")
	}
}

func TestImportarCaixasErros(t *testing.T) {
	env := newTestEnv(t)
	if _, code := env.importBoxes(t, "nao-existe", "", "t.json", cocoJSON()); code != http.StatusNotFound {
		t.Errorf("pacote inexistente: %d", code)
	}
	rec := env.postFiles(t, "/packages/"+testPkg+"/boxes/import", upFile{"t.json", "não é json"})
	assertStatus(t, rec, http.StatusBadRequest)
	rec = env.postFiles(t, "/packages/"+testPkg+"/boxes/import", upFile{"t.json", `{"foo":1}`})
	assertStatus(t, rec, http.StatusBadRequest)
	if !strings.Contains(errorMsg(t, rec), "COCO 1.0") {
		t.Errorf("a mensagem deveria dizer o formato esperado: %q", errorMsg(t, rec))
	}
	assertStatus(t, env.postFiles(t, "/packages/"+testPkg+"/boxes/import"), http.StatusBadRequest)
	assertStatus(t, env.do("GET", "/packages/nao-existe/boxes", nil, nil), http.StatusNotFound)
}

func TestLeadingSHA1(t *testing.T) {
	sha := strings.Repeat("ab", 20)
	cases := map[string]bool{
		sha + ".jpg":                  true,
		strings.ToUpper(sha) + ".jpg": true,
		sha + "_v2_8003_x.jpg":        true,
		sha:                           true,
		sha + "c.jpg":                 false, // 41 hex: não é um sha1
		"10_CHEVROLET_AGILE.jpg":      false,
		"zz" + sha[2:] + ".jpg":       false,
	}
	for name, want := range cases {
		if _, got := leadingSHA1(name); got != want {
			t.Errorf("leadingSHA1(%q) = %v, want %v", name, got, want)
		}
	}
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
	if !names[a+".jpg"] || !names[env.db.images[b].FileName] || len(names) != 2 {
		t.Errorf("arquivos no zip = %v", names)
	}
	// o que sai no zip volta casando no import
	for name := range names {
		if _, ok := leadingSHA1(name); !ok {
			t.Errorf("%s deveria começar pelo sha1", name)
		}
	}

	env.db.packages["vazio"] = store.Package{Name: "vazio"}
	assertStatus(t, env.do("GET", "/packages/vazio/cvat.zip", nil, nil), http.StatusNotFound)
	assertStatus(t, env.do("GET", "/packages/nao-existe/cvat.zip", nil, nil), http.StatusNotFound)
}
