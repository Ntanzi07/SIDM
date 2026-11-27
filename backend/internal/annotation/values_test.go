package annotation

import (
	"strings"
	"testing"
	"time"
)

func check(t *testing.T, v Values, complete bool) (Values, string) {
	t.Helper()
	p := capacete()
	out, err := p.Check(v, complete)
	if err == nil {
		return out, ""
	}
	var paths []string
	for _, pr := range err.(*InvalidError).Problems {
		paths = append(paths, pr.Path)
	}
	return out, strings.Join(paths, " ")
}

func TestCheckLimpa(t *testing.T) {
	out, probs := check(t, Values{
		Fields: map[string]any{
			"day_time": " night ", "riders": float64(2), "notes": "  ok  ", "blurry": true,
			"tags":      []any{"rain", "rain", " tunnel "}, // repetido entra uma vez
			"seen_at":   "2026-09-03T08:25:23-03:00",
			"equipment": "", // vazio = ausente
		},
		Objects: []Object{{Type: "plate", Fields: map[string]any{"text": "abc1d23"}}},
	}, true)
	if probs != "" {
		t.Fatal(probs)
	}
	f := out.Fields
	if f["day_time"] != "night" || f["notes"] != "ok" || f["riders"] != 2.0 || f["blurry"] != true {
		t.Errorf("campos = %#v", f)
	}
	if tags := f["tags"].([]string); len(tags) != 2 || tags[1] != "tunnel" {
		t.Errorf("tags = %v", tags)
	}
	if at := f["seen_at"].(time.Time); at.Location() != time.UTC || at.Hour() != 11 {
		t.Errorf("data = %v (esperado em UTC)", at)
	}
	if _, ok := f["equipment"]; ok {
		t.Error("vazio deveria sumir")
	}
	if out.Objects[0].Fields["text"] != "ABC1D23" {
		t.Errorf("placa = %v (esperado em maiúsculas)", out.Objects[0].Fields["text"])
	}
}

func TestCheckRascunhoNaoCobraObrigatorio(t *testing.T) {
	if _, probs := check(t, Values{}, false); probs != "" {
		t.Errorf("rascunho vazio: %s", probs)
	}
	_, probs := check(t, Values{}, true)
	if !strings.Contains(probs, "fields.day_time") || !strings.Contains(probs, "objects") {
		t.Errorf("verificar vazio: %s", probs)
	}
}

func TestCheckErros(t *testing.T) {
	cases := []struct {
		name string
		v    Values
		want string
	}{
		{"opção", Values{Fields: map[string]any{"day_time": "tarde"}}, "fields.day_time"},
		{"tipo errado", Values{Fields: map[string]any{"day_time": 1}}, "fields.day_time"},
		{"inteiro", Values{Fields: map[string]any{"riders": 1.5}}, "fields.riders"},
		{"mínimo", Values{Fields: map[string]any{"riders": 0}}, "fields.riders"},
		{"máximo", Values{Fields: map[string]any{"riders": int64(4)}}, "fields.riders"},
		{"tamanho", Values{Fields: map[string]any{"notes": strings.Repeat("x", 21)}}, "fields.notes"},
		{"bool", Values{Fields: map[string]any{"blurry": "sim"}}, "fields.blurry"},
		{"multi", Values{Fields: map[string]any{"tags": []any{"neve"}}}, "fields.tags"},
		{"multi tipo", Values{Fields: map[string]any{"tags": "rain"}}, "fields.tags"},
		{"data", Values{Fields: map[string]any{"seen_at": "03/09/2026"}}, "fields.seen_at"},
		{"campo desconhecido", Values{Fields: map[string]any{"cor": "azul"}}, "fields.cor"},
		{"padrão", Values{Objects: []Object{{Type: "plate", Fields: map[string]any{"text": "ABC12"}}}}, "objects[0].text"},
		// o padrão vale para o valor INTEIRO, não para um pedaço
		{"padrão inteiro", Values{Objects: []Object{{Type: "plate", Fields: map[string]any{"text": "ABC1D23X"}}}}, "objects[0].text"},
		{"objeto desconhecido", Values{Objects: []Object{{Type: "pessoa"}}}, "objects[0].type"},
		{"objeto com caixa", Values{Objects: []Object{{Type: "helmet"}}}, "objects[0].type"},
		{"objetos demais", Values{Objects: []Object{{Type: "plate"}, {Type: "plate"}, {Type: "plate"}}}, "objects"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, probs := check(t, tc.v, false)
			if !strings.Contains(" "+probs+" ", " "+tc.want+" ") {
				t.Errorf("problemas = %q, esperava %q", probs, tc.want)
			}
		})
	}
}

// O Mongo devolve listas como bson.A (um tipo com nome próprio).
type namedList []any

func TestCheckListaComNome(t *testing.T) {
	if _, probs := check(t, Values{Fields: map[string]any{"tags": namedList{"rain"}}}, false); probs != "" {
		t.Error(probs)
	}
}

// Uma anotação de hoje cabe no perfil "veiculos".
func TestCheckVeiculos(t *testing.T) {
	p := Vehicles()
	_, err := p.Check(Values{
		Fields: map[string]any{
			"equipment": "8003-MF4739", "direction": "rear", "resolution": "1920x1080", "day_time": "night",
			"conditions": []string{"glare"}, "captured_at": "2026-09-03T11:25:23Z",
		},
		Objects: []Object{{Type: "vehicle", Fields: map[string]any{
			"plate": "abc1d23", "plate_type": "mercosul", "plate_category": "private", "type": "car", "color": "white",
		}}},
	}, true)
	if err != nil {
		t.Fatalf("%v: %+v", err, err.(*InvalidError).Problems)
	}
}

func TestFit(t *testing.T) {
	p := capacete()
	out, warnings := p.Fit(Values{
		Fields: map[string]any{"day_time": "night", "riders": 9.0, "weather": "rain", "notes": ""},
		Objects: []Object{
			{Type: "plate", Fields: map[string]any{"text": "abc1d23", "color": "white"}},
			{Type: "helmet"},  // com caixa: não entra pela lista
			{Type: "vehicle"}, // não existe no perfil
		},
	})
	if out.Fields["day_time"] != "night" || len(out.Fields) != 1 {
		t.Errorf("campos = %v", out.Fields)
	}
	if len(out.Objects) != 1 || out.Objects[0].Fields["text"] != "ABC1D23" || len(out.Objects[0].Fields) != 1 {
		t.Errorf("objetos = %+v", out.Objects)
	}
	// riders fora do limite, weather e color que o perfil não tem, helmet e vehicle
	if len(warnings) != 5 {
		t.Errorf("avisos = %q", warnings)
	}
}

func TestFromText(t *testing.T) {
	cor := Field{Key: "color", Type: TypeChoice, Options: []Option{{Value: "white", Label: "Branco", Code: "WHT"}}}
	for _, in := range []string{"white", "WHT", "branco", "BRANCO"} {
		if v, err := cor.FromText(in); err != nil || v != "white" {
			t.Errorf("%q -> %v %v", in, v, err)
		}
	}
	if _, err := cor.FromText("azul"); err == nil {
		t.Error("azul não é opção")
	}
	num := Field{Type: TypeNumber}
	if v, _ := num.FromText("2,5"); v != 2.5 {
		t.Errorf("número com vírgula: %v", v)
	}
	sim := Field{Type: TypeBool}
	if v, _ := sim.FromText("Sim"); v != true {
		t.Errorf("sim: %v", v)
	}
	if _, err := sim.FromText("talvez"); err == nil {
		t.Error("talvez não é sim/não")
	}
}

func TestFitFieldsDeUmaCaixa(t *testing.T) {
	helmet := ObjectType{Key: "helmet", Box: true, Fields: []Field{
		{Key: "color", Label: "Cor", Type: TypeChoice, Required: true, Options: []Option{{Value: "black", Label: "Preto"}}},
		{Key: "count", Type: TypeNumber, Integer: true},
	}}
	clean, warns := helmet.FitFields(map[string]any{"Cor": "preto", "count": "2", "x": "y"})
	if clean["color"] != "black" || clean["count"] != 2.0 || len(warns) != 1 || !strings.Contains(warns[0], "x") {
		t.Errorf("clean=%v warns=%v", clean, warns)
	}
	if m := helmet.Missing(map[string]any{"count": 1.0}); len(m) != 1 || m[0] != "color" {
		t.Errorf("missing = %v", m)
	}
	if _, err := helmet.CheckFields("boxes[0].attributes", map[string]any{"color": "azul"}, false); err == nil ||
		err.(*InvalidError).Problems[0].Path != "boxes[0].attributes.color" {
		t.Errorf("CheckFields: %v", err)
	}
	p := Profile{Objects: []ObjectType{helmet, {Key: "plate"}}}
	if _, ok := p.BoxType("helmet"); !ok {
		t.Error("helmet é tipo com caixa")
	}
	if _, ok := p.BoxType("plate"); ok {
		t.Error("plate não tem caixa")
	}
}

// A opção exclusiva ("Sem avarias") não convive com as outras: junto de um
// problema, sai; sozinha, fica.
func TestMultiExclusiva(t *testing.T) {
	f := Field{Key: "conditions", Type: TypeMulti, Options: []Option{{Value: "no_damage", Exclusive: true}, {Value: "glare"}, {Value: "low_contrast"}}}
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"no_damage"}, "no_damage"},
		{[]string{"no_damage", "glare"}, "glare"},
		{[]string{"glare", "no_damage", "low_contrast"}, "glare low_contrast"},
		{[]string{"glare"}, "glare"},
	}
	for _, c := range cases {
		got, err := f.Clean(c.in)
		if err != nil {
			t.Fatalf("%v: %v", c.in, err)
		}
		if s := strings.Join(got.([]string), " "); s != c.want {
			t.Errorf("%v = %q, esperava %q", c.in, s, c.want)
		}
	}
	// no perfil veiculos, "Sem avarias" é a exclusiva das condições
	v := Vehicles()
	for _, field := range v.Fields {
		if field.Key != "conditions" {
			continue
		}
		got, _ := field.Clean([]string{"no_damage", "glare"})
		if s := strings.Join(got.([]string), " "); s != "glare" {
			t.Errorf("veiculos: %q", s)
		}
	}
}
