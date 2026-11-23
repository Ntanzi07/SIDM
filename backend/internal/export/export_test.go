package export

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"strings"
	"testing"

	"SIDM/internal/annotation"
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

// vehicles é o perfil de anotação padrão, o dos perfis daqui.
var vehicles = annotation.Vehicles()

// motos: um perfil de anotação diferente (número, sim/não, objeto sem caixa
// "rider" e um com caixa).
var motos = annotation.Profile{
	Name: "motos",
	Fields: []annotation.Field{
		{Key: "day_time", Type: annotation.TypeChoice, Options: []annotation.Option{{Value: "day", Code: "D"}, {Value: "night", Code: "N"}}},
		{Key: "riders", Type: annotation.TypeNumber, Integer: true},
		{Key: "rain", Label: "Chuva", Type: annotation.TypeBool},
	},
	Objects: []annotation.ObjectType{
		{Key: "rider", Label: "Pessoa", Fields: []annotation.Field{
			{Key: "helmet", Label: "Capacete", Type: annotation.TypeChoice, Options: []annotation.Option{{Value: "yes", Label: "Com"}, {Value: "no", Label: "Sem"}}},
		}},
		{Key: "helmet_box", Box: true, Fields: []annotation.Field{{Key: "color", Type: annotation.TypeText}}},
	},
}

func TestValidate(t *testing.T) {
	ok := Profile{Name: "yolo-noite", Format: FormatYOLO, Split: Split{80, 20, 0}, Filters: Filters{DayTime: "night"}}
	ok.Normalize()
	if err := ok.Validate(&vehicles); err != nil {
		t.Fatalf("válido deu erro: %v", err)
	}
	cases := map[string]Profile{
		"nome com espaço":                       {Name: "meu perfil", Format: FormatCOCO, Split: Split{70, 15, 15}},
		"nome de padrão":                        {Name: "rf-detr", Format: FormatCOCO, Split: Split{70, 15, 15}},
		"formato":                               {Name: "x", Format: "pascal", Split: Split{70, 15, 15}},
		"pastas fora do voc":                    {Name: "x", Format: FormatCOCO, Split: Split{70, 15, 15}, Layout: LayoutFolders, Folders: []FolderLevel{{Field: "day_time"}}},
		"pastas sem nível":                      {Name: "x", Format: FormatVOC, Split: Split{100, 0, 0}, Layout: LayoutFolders},
		"layout desconhecido":                   {Name: "x", Format: FormatVOC, Split: Split{70, 15, 15}, Layout: "arvore"},
		"campo desconhecido":                    {Name: "x", Format: FormatVOC, Split: Split{100, 0, 0}, Layout: LayoutFolders, Folders: []FolderLevel{{Field: "cor"}}},
		"campo repetido":                        {Name: "x", Format: FormatVOC, Split: Split{100, 0, 0}, Layout: LayoutFolders, Folders: []FolderLevel{{Field: "day_time"}, {Field: "day_time"}}},
		"nome de pasta ruim":                    {Name: "x", Format: FormatVOC, Split: Split{100, 0, 0}, Layout: LayoutFolders, Folders: []FolderLevel{{Field: "day_time", Names: map[string]string{"day": "a/b"}}}},
		"valor desconhecido":                    {Name: "x", Format: FormatVOC, Split: Split{100, 0, 0}, Layout: LayoutFolders, Folders: []FolderLevel{{Field: "day_time", Names: map[string]string{"D": "Dia"}}}},
		"split com pasta":                       {Name: "x", Format: FormatVOC, Split: Split{0, 50, 50}, Layout: LayoutFolders, Folders: []FolderLevel{{Field: "split"}}},
		"não soma 100":                          {Name: "x", Format: FormatCOCO, Split: Split{70, 20, 15}},
		"train zero":                            {Name: "x", Format: FormatCOCO, Split: Split{0, 50, 50}},
		"negativo":                              {Name: "x", Format: FormatCOCO, Split: Split{110, -10, 0}},
		"yolo sem valid":                        {Name: "x", Format: FormatYOLO, Split: Split{90, 0, 10}},
		"filtro desconhecido":                   {Name: "x", Format: FormatCOCO, Split: Split{70, 15, 15}, Filters: Filters{DayTime: "N"}},
		"filtro de campo que o perfil não tem":  {Name: "x", Format: FormatCOCO, Split: Split{70, 15, 15}, Filters: Filters{Fields: map[string]string{"riders": "2"}}},
		"filtro de objeto que o perfil não tem": {Name: "x", Format: FormatCOCO, Split: Split{70, 15, 15}, Filters: Filters{Objects: map[string]map[string]string{"rider": {"helmet": "yes"}}}},
		"filtro por data":                       {Name: "x", Format: FormatCOCO, Split: Split{70, 15, 15}, Filters: Filters{Fields: map[string]string{"captured_at": "2026"}}},
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			p.Normalize()
			var ve *ValidationError
			if err := p.Validate(&vehicles); !errors.As(err, &ve) {
				t.Errorf("esperava erro de validação, veio %v", err)
			}
		})
	}
	// pastas sem nível "split": a divisão não importa (só tem que somar 100)
	pastas := Profile{Name: "x", Format: FormatVOC, Split: Split{0, 0, 100}, Layout: LayoutFolders, Folders: []FolderLevel{{Field: "equipment"}}}
	pastas.Normalize()
	if err := pastas.Validate(&vehicles); err != nil {
		t.Errorf("pastas sem split: %v", err)
	}
	for _, b := range Builtins() {
		if err := b.Validate(&vehicles); err != nil {
			t.Errorf("perfil padrão %s inválido: %v", b.Name, err)
		}
	}
	// "task" (o formato antigo) vira "tasks"
	one := Profile{Task: " placa ", Tasks: []string{"parabrisa", "placa", ""}}
	one.Normalize()
	if one.Task != "" || strings.Join(one.Tasks, ",") != "placa,parabrisa" {
		t.Errorf("tarefas = %q %v", one.Task, one.Tasks)
	}
	// os filtros antigos viram fields/objects
	if f := ok.Filters; f.DayTime != "" || f.Fields["day_time"] != "night" {
		t.Errorf("filtros = %+v", f)
	}
	old := Profile{Filters: Filters{VehicleType: "truck", VehicleColor: "white", Equipment: " "}}
	old.Normalize()
	if fmt.Sprint(old.Filters.Objects) != "map[vehicle:map[color:white type:truck]]" || old.Filters.Fields != nil {
		t.Errorf("filtros antigos = %+v", old.Filters)
	}
}

// Num perfil de anotação que não é o de veículos: os filtros, as pastas e o
// nome usam os campos dele.
func TestPerfilDeAnotacaoProprio(t *testing.T) {
	p := Profile{
		Name: "motos-voc", Format: FormatVOC, Split: Split{100, 0, 0}, Layout: LayoutFolders, AnnotationProfile: "motos",
		Folders:  []FolderLevel{{Field: "rider_helmet", Names: map[string]string{"yes": "ComCapacete"}}, {Field: "riders"}, {Field: "rain"}},
		FileName: "{sha1}{day_time}-{riders}",
		Filters:  Filters{Fields: map[string]string{"riders": "2", "rain": "false"}, Objects: map[string]map[string]string{"rider": {"helmet": "yes"}}},
	}
	p.Normalize()
	if err := p.Validate(&motos); err != nil {
		t.Fatal(err)
	}
	if got := p.Dir(Meta{"rider_helmet": "yes", "riders": "2"}, SplitTrain); got != "ComCapacete/2/Desconhecido" {
		t.Errorf("Dir = %q", got)
	}
	if got := p.FileNameOf(Meta{"sha1": "abc", "day_time": "night", "riders": "2"}, ".jpg"); got != "abcN-2.jpg" {
		t.Errorf("nome = %q", got)
	}
	fields, objects := p.FilterValues()
	if fields["riders"] != 2.0 || fields["rain"] != false || objects["rider"]["helmet"] != "yes" {
		t.Errorf("filtros com tipo: %v %v", fields, objects)
	}

	for name, mod := range map[string]func(p *Profile){
		"pasta de veículo":    func(p *Profile) { p.Folders = []FolderLevel{{Field: "vehicle_type"}} },
		"objeto com caixa":    func(p *Profile) { p.Folders = []FolderLevel{{Field: "helmet_box_color"}} },
		"nome de veículo":     func(p *Profile) { p.FileName = "{sha1}{plate}" },
		"número inválido":     func(p *Profile) { p.Filters.Fields["riders"] = "dois" },
		"sim/não inválido":    func(p *Profile) { p.Filters.Fields["rain"] = "talvez" },
		"opção inexistente":   func(p *Profile) { p.Filters.Objects["rider"]["helmet"] = "talvez" },
		"perfil trocado":      func(p *Profile) { p.AnnotationProfile = "veiculos" },
		"valor de pasta ruim": func(p *Profile) { p.Folders[0].Names = map[string]string{"talvez": "X"} },
	} {
		t.Run(name, func(t *testing.T) {
			q := p
			q.Folders = append([]FolderLevel(nil), p.Folders...)
			q.Filters = Filters{Fields: maps.Clone(p.Filters.Fields), Objects: map[string]map[string]string{"rider": maps.Clone(p.Filters.Objects["rider"])}}
			mod(&q)
			if err := q.Validate(&motos); err == nil {
				t.Error("deveria ser inválido")
			}
		})
	}

	// os metadados do perfil: sem os de veículo, com os dele
	byKey := map[string]MetaField{}
	for _, m := range MetaFields(&motos) {
		byKey[m.Key] = m
	}
	if _, ok := byKey["vehicle_type"]; ok {
		t.Error("vehicle_type não existe neste perfil")
	}
	if m := byKey["rider_helmet"]; !m.Folder || m.Labels["yes"] != "Com" || m.Label != "Pessoa: Capacete" {
		t.Errorf("rider_helmet = %+v", m)
	}
	if m := byKey["helmet"]; !m.Alias {
		t.Errorf("helmet (sem o tipo) deveria ser atalho: %+v", m)
	}
	if _, ok := byKey["helmet_box_color"]; ok {
		t.Error("objeto com caixa não vira metadado da imagem")
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

func TestDir(t *testing.T) {
	p, _ := Builtin("voc-pastas")
	p.Bind(&vehicles)
	m := Meta{"vehicle_type": "motorcycle", "resolution": "752x480", "plate_type": "mercosul", "direction": "rear", "day_time": "night"}
	if got := p.Dir(m, SplitTrain); got != "motorCycle/02-752x480/Mercosul/Traseiro/Noturno" {
		t.Errorf("Dir = %q", got)
	}
	// sem metadado: "Desconhecido" (ou o nome escolhido); valor livre é limpo
	p.Folders = []FolderLevel{{Field: "direction"}, {Field: "weather", Unknown: "sem-clima"}, {Field: "equipment"}, {Field: "split"}, {Field: "resolution"}}
	m = Meta{"equipment": `EQ:1/A`, "resolution": "999x1"}
	if got := p.Dir(m, SplitValid); got != "Desconhecido/sem-clima/EQ_1_A/valid/999x1" {
		t.Errorf("Dir = %q", got)
	}
	specs := fieldSpecs(&vehicles)
	if v := specs["resolution"].Values; v["640x480"] != "01-640x480" {
		t.Errorf("valores da resolução = %v", v)
	}
	if specs["equipment"].Values != nil || len(specs["split"].Values) != 3 {
		t.Error("campo aberto deveria ter valores nil; split tem 3")
	}
	// os nomes de antes dos perfis continuam valendo
	for _, k := range []string{"vehicle_type", "vehicle_color", "plate_type", "plate_category", "direction", "equipment", "package"} {
		if !specs[k].Folder {
			t.Errorf("%s deveria servir de pasta", k)
		}
	}
}

func TestWriteVOC(t *testing.T) {
	t.Run("por divisão", func(t *testing.T) {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		p, _ := Builtin("voc")
		if _, err := Write(zw, p, items(), []string{"carro", "placa"}, nil); err != nil {
			t.Fatal(err)
		}
		zw.Close()
		files := readZip(t, buf.Bytes())
		if files["train/a_v2_x.jpg"] != "A" || files["valid/b.jpg"] != "B" || files["labels.txt"] != "carro\nplaca\n" {
			t.Errorf("arquivos: %v", keys(files))
		}
		if _, ok := files["test/"]; !ok {
			t.Error("a pasta test/ tem que existir mesmo vazia")
		}
		var ann struct {
			Folder   string `xml:"folder"`
			Filename string `xml:"filename"`
			Size     struct {
				Width  int `xml:"width"`
				Height int `xml:"height"`
				Depth  int `xml:"depth"`
			} `xml:"size"`
			Objects []struct {
				Name      string `xml:"name"`
				Truncated int    `xml:"truncated"`
				Box       struct {
					XMin int `xml:"xmin"`
					YMin int `xml:"ymin"`
					XMax int `xml:"xmax"`
					YMax int `xml:"ymax"`
				} `xml:"bndbox"`
			} `xml:"object"`
		}
		x := files["train/a_v2_x.xml"]
		if !strings.HasPrefix(x, "<?xml") {
			t.Errorf("xml sem cabeçalho:\n%s", x)
		}
		if err := xml.Unmarshal([]byte(x), &ann); err != nil {
			t.Fatal(err)
		}
		if ann.Folder != "train" || ann.Filename != "a_v2_x.jpg" || ann.Size.Width != 200 || ann.Size.Depth != 3 || len(ann.Objects) != 2 {
			t.Fatalf("anotação = %+v", ann)
		}
		// placa (10,20) 40x10 -> 10,20,50,30; carro na imagem toda -> truncado
		if o := ann.Objects[0]; o.Name != "placa" || o.Box.XMin != 10 || o.Box.YMin != 20 || o.Box.XMax != 50 || o.Box.YMax != 30 || o.Truncated != 0 {
			t.Errorf("placa = %+v", o)
		}
		if o := ann.Objects[1]; o.Name != "carro" || o.Box.XMax != 200 || o.Truncated != 1 {
			t.Errorf("carro = %+v", o)
		}
	})

	t.Run("por pastas", func(t *testing.T) {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		p, _ := Builtin("voc-pastas")
		list := items()
		list[0].Dir = "Car/01-640x480/Br/Traseiro/Noturno"
		list[1].Dir = "Bus/02-752x480/Mercosul/Dianteiro/Diurno"
		res, err := Write(zw, p, list, []string{"carro", "placa"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		zw.Close()
		files := readZip(t, buf.Bytes())
		for _, f := range []string{
			"Car/01-640x480/Br/Traseiro/Noturno/a_v2_x.jpg", "Car/01-640x480/Br/Traseiro/Noturno/a_v2_x.xml",
			"Bus/02-752x480/Mercosul/Dianteiro/Diurno/b.jpg", "Bus/02-752x480/Mercosul/Dianteiro/Diurno/b.xml",
		} {
			if _, ok := files[f]; !ok {
				t.Errorf("faltou %s (tem: %v)", f, keys(files))
			}
		}
		if _, ok := files["train/"]; ok {
			t.Error("por pastas não tem train/")
		}
		if res.Folders["Car/01-640x480/Br/Traseiro/Noturno"] != 1 || len(res.Missing) != 1 {
			t.Errorf("result = %+v", res)
		}
		if !strings.Contains(files["Bus/02-752x480/Mercosul/Dianteiro/Diurno/b.xml"], "<folder>Bus/02-752x480/Mercosul/Dianteiro/Diurno</folder>") {
			t.Error("folder errado no xml")
		}
	})
}

func TestFileNameOf(t *testing.T) {
	p := &Profile{FileName: TemplateLegacy}
	p.Bind(&vehicles)
	sha := "02d9314a6112e8b95f97415dc96e5bf0682f3a3f"
	m := NameMeta(sha, sha+"_v2_x.jpg", "IMG_1.JPG", Meta{
		"direction": "front", "resolution": "640x480", "capture_angle": "lateral", "day_time": "day",
		"vehicle_type": "bus", "image_range": "narrow", "plate_type": "br", "plate_category": "rental",
		"weather": "clear", "condition": "no_damage", "plate": "koy7386",
	})
	// o exemplo do padrão antigo
	if got := p.FileNameOf(m, ".jpg"); got != sha+"F01LDBEB1S01KOY7386.jpg" {
		t.Errorf("legado = %q", got)
	}
	// sem dado: "_" na largura do campo
	if got := p.FileNameOf(NameMeta(sha, "", "", Meta{"direction": "rear"}), ".jpg"); got != sha+"T"+strings.Repeat("_", 11)+strings.Repeat("_", 7)+".jpg" {
		t.Errorf("sem dados = %q", got)
	}
	// vazio: o nome guardado
	if got := (&Profile{}).FileNameOf(m, ".jpg"); got != sha+"_v2_x.jpg" {
		t.Errorf("padrão = %q", got)
	}
	// texto fixo, campos livres limpos
	p.FileName = "{original}-{sha1}_{equipment}"
	m["equipment"] = "EQ/1"
	if got := p.FileNameOf(m, ".png"); got != "IMG_1-"+sha+"_EQ_1.png" {
		t.Errorf("custom = %q", got)
	}

	for name, tmpl := range map[string]string{
		"sem sha1":           "{plate}{direction}",
		"campo desconhecido": "{sha1}{cor}",
		"barra":              "{sha1}/x",
		"chave sobrando":     "{sha1}}",
	} {
		pr := Profile{Name: "x", Format: FormatCOCO, Split: Split{70, 15, 15}, FileName: tmpl}
		pr.Normalize()
		if err := pr.Validate(&vehicles); err == nil {
			t.Errorf("%s: deveria ser inválido", name)
		}
	}
	ok := Profile{Name: "x", Format: FormatCOCO, Split: Split{70, 15, 15}, FileName: " {name}-{date} "}
	ok.Normalize()
	if err := ok.Validate(&vehicles); err != nil || ok.FileName != "{name}-{date}" {
		t.Errorf("válido: %v %q", err, ok.FileName)
	}
}

func TestWriteImages(t *testing.T) {
	write := func(t *testing.T, p Profile, list []Item) map[string]string {
		t.Helper()
		p.Bind(&vehicles)
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		res, err := Write(zw, p, list, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		zw.Close()
		if len(res.Boxes) != 0 || len(res.Missing) != 1 {
			t.Errorf("result = %+v", res)
		}
		files := readZip(t, buf.Bytes())
		for name := range files {
			if strings.HasSuffix(name, ".xml") || strings.HasSuffix(name, ".txt") && name != "FALTANDO.txt" || strings.HasSuffix(name, ".json") && name != "export_info.json" {
				t.Errorf("sem caixas não tem arquivo de anotação: %s", name)
			}
		}
		return files
	}
	withMeta := func() []Item {
		list := items()
		for i := range list {
			list[i].Boxes = nil // o Collect não busca caixas no images
		}
		list[0].Meta = Meta{"sha1": "a", "package": "p1", "original": "foto", "day_time": "night", "vehicle_type": "car", "vehicle_plate": "ABC1D23"}
		list[1].Meta = Meta{"sha1": "b", "package": "p1"}
		return list
	}

	t.Run("um grupo só: na raiz, com o csv", func(t *testing.T) {
		p, ok := Builtin("imagens")
		if !ok || p.NeedsTasks() {
			t.Fatal("faltou o perfil padrão imagens")
		}
		files := write(t, p, withMeta())
		if files["a_v2_x.jpg"] != "A" || files["b.jpg"] != "B" || files["export_info.json"] == "" {
			t.Errorf("arquivos: %v", keys(files))
		}
		if _, ok := files["train/"]; ok {
			t.Error("com 100/0/0 não tem pasta train/")
		}
		csv := files[CSVName]
		if !strings.HasPrefix(csv, "\uFEFFarquivo;sha1;pacote;nome_original;") {
			t.Fatalf("cabeçalho do csv:\n%s", csv)
		}
		lines := strings.Split(strings.TrimSuffix(csv, "\r\n"), "\r\n")
		if len(lines) != 3 { // cabeçalho + 2 (a que não abriu fica de fora)
			t.Fatalf("linhas do csv:\n%s", csv)
		}
		head := strings.Split(strings.TrimPrefix(lines[0], "\uFEFF"), ";")
		row := strings.Split(lines[1], ";")
		col := func(name string) string {
			for i, h := range head {
				if h == name {
					return row[i]
				}
			}
			t.Errorf("o csv não tem a coluna %s: %v", name, head)
			return ""
		}
		if col("arquivo") != "a_v2_x.jpg" || col("pacote") != "p1" || col("nome_original") != "foto" ||
			col("day_time") != "night" || col("vehicle_type") != "car" || col("vehicle_plate") != "ABC1D23" {
			t.Errorf("linha = %v", row)
		}
		for _, h := range head {
			if h == "plate" || h == "condition" || h == "split" {
				t.Errorf("atalho ou campo de arquivo não vira coluna: %s", h)
			}
		}
	})

	t.Run("por divisão", func(t *testing.T) {
		p := Profile{Name: "x", Format: FormatImages, Split: Split{70, 15, 15}}
		files := write(t, p, withMeta())
		if files["train/a_v2_x.jpg"] != "A" || files["valid/b.jpg"] != "B" {
			t.Errorf("arquivos: %v", keys(files))
		}
		if _, ok := files["test/"]; !ok {
			t.Error("a pasta test/ tem que existir mesmo vazia")
		}
		if !strings.Contains(files[CSVName], "\r\ntrain/a_v2_x.jpg;a;") {
			t.Errorf("o csv leva o caminho no zip:\n%s", files[CSVName])
		}
	})

	t.Run("por pastas", func(t *testing.T) {
		p := Profile{Name: "x", Format: FormatImages, Split: Split{100, 0, 0}, Layout: LayoutFolders, Folders: []FolderLevel{{Field: "day_time"}}}
		list := withMeta()
		list[0].Dir, list[1].Dir = "night", DefaultUnknown
		files := write(t, p, list)
		if files["night/a_v2_x.jpg"] != "A" || files[DefaultUnknown+"/b.jpg"] != "B" {
			t.Errorf("arquivos: %v", keys(files))
		}
	})
}

func TestValidateSoImagens(t *testing.T) {
	p := Profile{
		Name: "imagens-noite", Format: FormatImages, Split: Split{100, 0, 0},
		Layout: LayoutFolders, Folders: []FolderLevel{{Field: "vehicle_type"}},
		Tasks: []string{"veiculos"}, AllTasks: true, Labels: LabelRules{Include: []string{"placa"}},
		Filters: Filters{DayTime: "night"},
	}
	p.Normalize()
	if err := p.Validate(&vehicles); err != nil {
		t.Fatalf("pastas no images deveriam valer: %v", err)
	}
	if p.Tasks != nil || p.AllTasks || len(p.Labels.Include) != 0 {
		t.Errorf("sem caixas, tarefa e labels não ficam no perfil: %+v", p)
	}
	p.SetTasks([]string{"veiculos"})
	if p.Tasks != nil {
		t.Errorf("SetTasks no images: %v", p.Tasks)
	}
}
