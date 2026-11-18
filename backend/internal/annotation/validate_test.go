package annotation

import (
	"strings"
	"testing"
)

func ptr(f float64) *float64 { return &f }

// capacete é um perfil de exemplo, diferente do de veículos.
func capacete() Profile {
	return Profile{
		Name: "capacete-moto",
		Fields: []Field{
			{Key: "day_time", Type: TypeChoice, Required: true, Options: []Option{{Value: "day", Code: "D"}, {Value: "night", Code: "N"}}},
			{Key: "riders", Label: "Pessoas na moto", Type: TypeNumber, Integer: true, Min: ptr(1), Max: ptr(3)},
			{Key: "notes", Type: TypeText, MaxLen: 20},
			{Key: "blurry", Type: TypeBool},
			{Key: "tags", Type: TypeMulti, Options: []Option{{Value: "rain"}, {Value: "tunnel"}}},
			{Key: "seen_at", Type: TypeDateTime},
			{Key: "equipment", Type: TypeRef, Ref: RefEquipment},
		},
		Objects: []ObjectType{
			{Key: "helmet", Box: true, Fields: []Field{{Key: "color", Type: TypeChoice, Options: []Option{{Value: "black"}, {Value: "white"}}}}},
			{Key: "plate", Min: 1, Max: 2, Fields: []Field{{Key: "text", Type: TypeText, Pattern: "[A-Z0-9]{7}", Upper: true, Required: true}}},
		},
	}
}

func TestPerfilValido(t *testing.T) {
	p := capacete()
	p.Normalize()
	if err := p.Validate(); err != nil {
		t.Fatalf("%v: %+v", err, err.(*InvalidError).Problems)
	}
}

// problemPaths valida e devolve os caminhos dos problemas.
func problemPaths(t *testing.T, p Profile) string {
	t.Helper()
	p.Normalize()
	err := p.Validate()
	if err == nil {
		return ""
	}
	var paths []string
	for _, pr := range err.(*InvalidError).Problems {
		paths = append(paths, pr.Path)
	}
	return strings.Join(paths, " ")
}

func TestPerfilInvalido(t *testing.T) {
	cases := []struct {
		name   string
		change func(p *Profile)
		want   string // caminho esperado entre os problemas
	}{
		{"nome", func(p *Profile) { p.Name = "Com Espaço" }, "name"},
		{"chave", func(p *Profile) { p.Fields[0].Key = "Dia" }, "fields[0].key"},
		{"reservada", func(p *Profile) { p.Fields[0].Key = "sha1" }, "fields[0].key"},
		{"repetida", func(p *Profile) { p.Fields[1].Key = "day_time" }, "fields[1].key"},
		{"tipo", func(p *Profile) { p.Fields[0].Type = "lista" }, "fields[0].type"},
		{"sem opções", func(p *Profile) { p.Fields[0].Options = nil }, "fields[0].options"},
		{"valor repetido", func(p *Profile) { p.Fields[0].Options[1].Value = "day" }, "fields[0].options[1].value"},
		{"código repetido", func(p *Profile) { p.Fields[0].Options[1].Code = "d" }, "fields[0].options[1].code"},
		{"código faltando", func(p *Profile) { p.Fields[0].Options[1].Code = "" }, "fields[0].options"},
		{"min > max", func(p *Profile) { p.Fields[1].Min = ptr(5) }, "fields[1]"},
		{"regex", func(p *Profile) { p.Objects[1].Fields[0].Pattern = "([" }, "objects[1].fields[0].pattern"},
		{"ref", func(p *Profile) { p.Fields[6].Ref = "pessoa" }, "fields[6].ref"},
		{"opção fora de lugar", func(p *Profile) { p.Fields[2].Options = []Option{{Value: "x"}} }, "fields[2].options"},
		{"upper fora de lugar", func(p *Profile) { p.Fields[0].Upper = true }, "fields[0]"},
		{"objeto repetido", func(p *Profile) { p.Objects[1].Key = "helmet" }, "objects[1].key"},
		{"objeto min > max", func(p *Profile) { p.Objects[1].Min = 3 }, "objects[1]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := capacete()
			tc.change(&p)
			got := problemPaths(t, p)
			if !strings.Contains(" "+got+" ", " "+tc.want+" ") {
				t.Errorf("problemas = %q, esperava %q", got, tc.want)
			}
		})
	}
}

// Todos os erros vêm juntos, não só o primeiro.
func TestPerfilTodosOsErros(t *testing.T) {
	p := capacete()
	p.Name = ""
	p.Fields[3].Type = "x" // blurry: um bool sem opções nem limites
	p.Objects[0].Key = ""
	err := p.Validate().(*InvalidError)
	if len(err.Problems) != 3 || !strings.Contains(err.Msg, "(e mais 2)") {
		t.Errorf("%s: %+v", err.Msg, err.Problems)
	}
}

func TestNormalize(t *testing.T) {
	p := Profile{Name: " x ", Fields: []Field{{Key: " a ", Type: " choice ", Options: []Option{{Value: " v ", Code: " f "}}}}}
	p.Normalize()
	f := p.Fields[0]
	if p.Name != "x" || f.Key != "a" || f.Type != "choice" || f.Options[0].Value != "v" || f.Options[0].Code != "F" || p.Objects == nil {
		t.Errorf("%+v", p)
	}
}
