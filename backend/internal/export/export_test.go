package export

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"testing"

	"SIDM/internal/boxes"
)

func TestSplitOfFixoEProporcional(t *testing.T) {
	s := Split{70, 15, 15}
	counts := map[string]int{}
	const n = 20000
	for i := 0; i < n; i++ {
		sha := fmt.Sprintf("%040x", i*7919)
		got := SplitOf(sha, s)
		if again := SplitOf(strings.ToUpper(sha), s); again != got {
			t.Fatal("o grupo tem que ser sempre o mesmo para a mesma imagem")
		}
		counts[got]++
	}
	for split, want := range map[string]float64{SplitTrain: 0.70, SplitValid: 0.15, SplitTest: 0.15} {
		if got := float64(counts[split]) / n; math.Abs(got-want) > 0.015 {
			t.Errorf("%s: %.3f, want ~%.2f", split, got, want)
		}
	}
	// sem test: ninguém cai em test
	for i := 0; i < 2000; i++ {
		if SplitOf(fmt.Sprintf("%040x", i), Split{80, 20, 0}) == SplitTest {
			t.Fatal("test com 0% não pode receber imagem")
		}
	}
}

func TestSplitEstavelAoMudarSoOTest(t *testing.T) {
	// Quem estava em train continua em train se só valid/test mudam de tamanho.
	for i := 0; i < 5000; i++ {
		sha := fmt.Sprintf("%040x", i)
		if SplitOf(sha, Split{70, 15, 15}) == SplitTrain && SplitOf(sha, Split{70, 20, 10}) != SplitTrain {
			t.Fatal("mudar só valid/test não pode tirar imagem do train")
		}
	}
}

func TestLabels(t *testing.T) {
	l := NewLabels(LabelRules{Rename: map[string]string{"Placa": "placa", "CARRO": "carro"}})
	for in, want := range map[string]string{"Placa": "placa", "placa": "placa", "CARRO": "carro", "moto": "moto"} {
		if got, ok := l.Map(in); !ok || got != want {
			t.Errorf("Map(%q) = %q, %v", in, got, ok)
		}
	}
	if got := l.Classes(map[string]int{"placa": 3, "carro": 1, "moto": 2}); strings.Join(got, ",") != "carro,moto,placa" {
		t.Errorf("sem include, ordem alfabética: %v", got)
	}

	l = NewLabels(LabelRules{Rename: map[string]string{"Placa": "placa"}, Include: []string{"placa", "carro"}})
	if _, ok := l.Map("moto"); ok {
		t.Error("moto não está no include")
	}
	if got, ok := l.Map("Placa"); !ok || got != "placa" {
		t.Error("renomeia antes de filtrar")
	}
	if got := l.Classes(nil); strings.Join(got, ",") != "placa,carro" {
		t.Errorf("com include, a ordem é a do perfil: %v", got)
	}
}

func TestValidate(t *testing.T) {
	ok := Profile{Name: "yolo-noite", Format: FormatYOLO, Split: Split{80, 20, 0}, Filters: Filters{DayTime: "night"}}
	ok.Normalize()
	if err := ok.Validate(); err != nil {
		t.Fatalf("válido deu erro: %v", err)
	}
	cases := map[string]Profile{
		"nome com espaço":     {Name: "meu perfil", Format: FormatCOCO, Split: Split{70, 15, 15}},
		"nome de padrão":      {Name: "rf-detr", Format: FormatCOCO, Split: Split{70, 15, 15}},
		"formato":             {Name: "x", Format: "voc", Split: Split{70, 15, 15}},
		"não soma 100":        {Name: "x", Format: FormatCOCO, Split: Split{70, 20, 15}},
		"train zero":          {Name: "x", Format: FormatCOCO, Split: Split{0, 50, 50}},
		"negativo":            {Name: "x", Format: FormatCOCO, Split: Split{110, -10, 0}},
		"yolo sem valid":      {Name: "x", Format: FormatYOLO, Split: Split{90, 0, 10}},
		"filtro desconhecido": {Name: "x", Format: FormatCOCO, Split: Split{70, 15, 15}, Filters: Filters{DayTime: "N"}},
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			p.Normalize()
			var ve *ValidationError
			if err := p.Validate(); !errors.As(err, &ve) {
				t.Errorf("esperava erro de validação, veio %v", err)
			}
		})
	}
	for _, b := range Builtins() {
		if err := b.Validate(); err != nil {
			t.Errorf("perfil padrão %s inválido: %v", b.Name, err)
		}
	}
}

func TestRectRotacionada(t *testing.T) {
	// 90°: uma caixa 40x10 centrada em (50,50) vira 10x40.
	x, y, w, h := rect(boxes.Box{X: 30, Y: 45, W: 40, H: 10, Rotation: 90}, 200, 200)
	if math.Abs(x-45) > 1e-6 || math.Abs(y-30) > 1e-6 || math.Abs(w-10) > 1e-6 || math.Abs(h-40) > 1e-6 {
		t.Errorf("rect = %v %v %v %v", x, y, w, h)
	}
	// passa da borda: corta
	x, y, w, h = rect(boxes.Box{X: -5, Y: 90, W: 20, H: 20}, 100, 100)
	if x != 0 || y != 90 || w != 15 || h != 10 {
		t.Errorf("recorte = %v %v %v %v", x, y, w, h)
	}
}

func items() []Item {
	open := func(content string) func() (io.ReadCloser, error) {
		return func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(content)), nil }
	}
	return []Item{
		{SHA1: "a", FileName: "a_v2_x.jpg", Width: 200, Height: 100, Split: SplitTrain, Open: open("A"),
			Boxes: []boxes.Box{{Label: "placa", X: 10, Y: 20, W: 40, H: 10}, {Label: "carro", X: 0, Y: 0, W: 200, H: 100}}},
		{SHA1: "b", FileName: "b.jpg", Width: 100, Height: 100, Split: SplitValid, Open: open("B"),
			Boxes: []boxes.Box{{Label: "carro", X: 50, Y: 50, W: 50, H: 50}}},
		{SHA1: "c", FileName: "c.jpg", Width: 100, Height: 100, Split: SplitTest,
			Open:  func() (io.ReadCloser, error) { return nil, errors.New("sumiu") },
			Boxes: []boxes.Box{{Label: "carro", X: 1, Y: 1, W: 5, H: 5}}},
	}
}

func readZip(t *testing.T, data []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		out[f.Name] = string(b)
	}
	return out
}

func TestWriteCOCO(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	p, _ := Builtin("rf-detr")
	res, err := Write(zw, p, items(), []string{"carro", "placa"}, map[string]string{"profile": "rf-detr"})
	if err != nil {
		t.Fatal(err)
	}
	zw.Close()
	files := readZip(t, buf.Bytes())

	if files["train/a_v2_x.jpg"] != "A" || files["valid/b.jpg"] != "B" {
		t.Errorf("imagens no lugar errado: %v", keys(files))
	}
	for _, sp := range []string{"train", "valid", "test"} {
		if _, ok := files[sp+"/_annotations.coco.json"]; !ok {
			t.Errorf("faltou %s/_annotations.coco.json (o RF-DETR espera as três pastas)", sp)
		}
	}
	var train struct {
		Categories []struct {
			ID   int
			Name string
		}
		Images []struct {
			ID            int
			FileName      string `json:"file_name"`
			Width, Height int
		}
		Annotations []struct {
			ImageID    int       `json:"image_id"`
			CategoryID int       `json:"category_id"`
			BBox       []float64 `json:"bbox"`
		}
	}
	if err := json.Unmarshal([]byte(files["train/_annotations.coco.json"]), &train); err != nil {
		t.Fatal(err)
	}
	if len(train.Categories) != 2 || train.Categories[0].ID != 1 || train.Categories[0].Name != "carro" {
		t.Errorf("categorias = %+v", train.Categories)
	}
	if len(train.Images) != 1 || train.Images[0].FileName != "a_v2_x.jpg" || train.Images[0].Width != 200 {
		t.Errorf("images = %+v", train.Images)
	}
	if len(train.Annotations) != 2 || train.Annotations[0].CategoryID != 2 || fmt.Sprint(train.Annotations[0].BBox) != "[10 20 40 10]" {
		t.Errorf("annotations = %+v", train.Annotations)
	}

	if strings.Join(res.Missing, ",") != "c.jpg" || !strings.Contains(files["FALTANDO.txt"], "c.jpg") {
		t.Errorf("imagem que não abriu: missing=%v", res.Missing)
	}
	if res.Images[SplitTrain] != 1 || res.Boxes["carro"] != 2 || !strings.Contains(files["export_info.json"], "rf-detr") {
		t.Errorf("res = %+v", res)
	}
}

func TestWriteYOLO(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	p, _ := Builtin("yolo")
	if _, err := Write(zw, p, items(), []string{"carro", "placa"}, nil); err != nil {
		t.Fatal(err)
	}
	zw.Close()
	files := readZip(t, buf.Bytes())

	if files["images/train/a_v2_x.jpg"] != "A" || files["images/val/b.jpg"] != "B" {
		t.Errorf("imagens: %v", keys(files))
	}
	// placa em (10,20) 40x10 numa imagem 200x100 -> centro (30,25) -> 0.15 0.25 0.2 0.1
	want := "1 0.150000 0.250000 0.200000 0.100000\n0 0.500000 0.500000 1.000000 1.000000\n"
	if got := files["labels/train/a_v2_x.txt"]; got != want {
		t.Errorf("labels = %q\nwant %q", got, want)
	}
	yaml := files["data.yaml"]
	if strings.Contains(yaml, "path:") {
		t.Error("data.yaml não pode ter path (o Ultralytics resolveria a partir da pasta de datasets dele)")
	}
	for _, dir := range []string{"images/test/", "labels/test/", "images/val/", "labels/train/"} {
		if _, ok := files[dir]; !ok {
			t.Errorf("faltou a pasta %s (mesmo vazia)", dir)
		}
	}
	for _, line := range []string{"train: images/train", "val: images/val", "test: images/test", "nc: 2", `0: "carro"`, `1: "placa"`} {
		if !strings.Contains(yaml, line) {
			t.Errorf("data.yaml sem %q:\n%s", line, yaml)
		}
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
