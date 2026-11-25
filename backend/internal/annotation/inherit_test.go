package annotation

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

// moto: estende veiculos com quantas pessoas estão na moto (na imagem) e
// o capacete de cada uma (no veículo), e diz que a caixa motorcycle é um
// veículo do tipo moto.
func moto() Profile {
	return Profile{
		Name: "moto", Extends: DefaultProfile,
		Fields: []Field{{Key: "riders", Label: "Pessoas na moto", Type: TypeNumber, Integer: true, Min: ptr(1), Max: ptr(3)}},
		Objects: []ObjectType{{Key: "vehicle", Fields: []Field{{Key: "helmet", Type: TypeBool}},
			BoxLabels: []BoxLabel{{Label: "motorcycle", Fields: map[string]any{"type": "motorcycle"}}}}},
	}
}

func TestHeranca(t *testing.T) {
	ctx := context.Background()
	s := New(NewMemory())
	m := moto()
	if err := s.CreateProfile(ctx, &m, "nathan"); err != nil {
		t.Fatalf("%v", err)
	}
	got, err := s.FindProfile(ctx, "moto")
	if err != nil {
		t.Fatal(err)
	}
	// os campos do pai primeiro (marcados), depois os dele
	if len(got.Fields) != 11 || got.Fields[0].Key != "equipment" || got.Fields[0].From != DefaultProfile || got.Fields[10].Key != "riders" || got.Fields[10].From != "" {
		t.Errorf("campos: %d, primeiro %+v, último %+v", len(got.Fields), got.Fields[0], got.Fields[len(got.Fields)-1])
	}
	v, _ := got.Object("vehicle")
	if f, _ := v.Field("plate"); f == nil || f.From != DefaultProfile {
		t.Errorf("placa herdada: %+v", f)
	}
	if f, ok := v.Field("helmet"); !ok || f.From != "" {
		t.Errorf("capacete acrescentado: %+v", f)
	}
	if !got.Is("moto") || !got.Is(DefaultProfile) || got.Is("capacete-moto") || !slices.Equal(got.Lineage, []string{"moto", DefaultProfile}) {
		t.Errorf("linhagem: %v", got.Lineage)
	}
	// no banco, só o que é dele
	if raw := s.repo.(*Memory).Profiles["moto"]; len(raw.Fields) != 1 || len(raw.Objects[0].Fields) != 1 {
		t.Errorf("gravado: %+v", raw)
	}
	// a tela devolve com os herdados: gravar de novo não duplica
	again := *got
	again.Description = "motos"
	if err := s.ReplaceProfile(ctx, &again); err != nil {
		t.Fatalf("regravar com os herdados: %v", err)
	}
	if raw := s.repo.(*Memory).Profiles["moto"]; len(raw.Fields) != 1 || raw.Description != "motos" {
		t.Errorf("regravado: %+v", raw)
	}

	// neto: estende moto
	n := Profile{Name: "moto-noite", Extends: "moto", Fields: []Field{{Key: "lamp", Type: TypeBool}}}
	if err := s.CreateProfile(ctx, &n, "nathan"); err != nil {
		t.Fatal(err)
	}
	if fam, _ := s.Family(ctx, DefaultProfile); !slices.Equal(fam, []string{DefaultProfile, "moto", "moto-noite"}) {
		t.Errorf("família: %v", fam)
	}
	neto, _ := s.FindProfile(ctx, "moto-noite")
	if f, _ := neto.Field("riders"); f == nil || f.From != "moto" {
		t.Errorf("riders do pai: %+v", f)
	}
	if f, _ := neto.Field("day_time"); f == nil || f.From != DefaultProfile {
		t.Errorf("day_time do avô: %+v", f)
	}

	// erros
	var invalid *InvalidError
	bad := Profile{Name: "x", Extends: "nao-existe"}
	if err := s.CreateProfile(ctx, &bad, "n"); !errors.As(err, &invalid) {
		t.Errorf("pai que não existe: %v", err)
	}
	dup := Profile{Name: "y", Extends: "moto", Fields: []Field{{Key: "riders", Type: TypeBool}}}
	if err := s.CreateProfile(ctx, &dup, "n"); !errors.As(err, &invalid) || !strings.Contains(err.Error(), "riders") {
		t.Errorf("campo repetido: %v", err)
	}
	loop := moto()
	loop.Extends = "moto-noite" // moto → moto-noite → moto
	if err := s.ReplaceProfile(ctx, &loop); !errors.As(err, &invalid) || !strings.Contains(err.Error(), "círculo") {
		t.Errorf("círculo: %v", err)
	}
	// o pai ganha um campo que o filho já tem: recusa
	clash := moto()
	clash.Fields = append(clash.Fields, Field{Key: "lamp", Type: TypeText})
	if err := s.ReplaceProfile(ctx, &clash); !errors.As(err, &invalid) || !strings.Contains(err.Error(), "moto-noite") {
		t.Errorf("colisão com o filho: %v", err)
	}
	// pai com filhos não se apaga
	if err := s.DeleteProfile(ctx, "moto"); !errors.As(err, &invalid) || !strings.Contains(err.Error(), "moto-noite") {
		t.Errorf("apagar o pai: %v", err)
	}
	// a lista vem resolvida
	list, _ := s.ListProfiles(ctx)
	for _, p := range list {
		if p.Name == "moto" && len(p.Fields) != 11 {
			t.Errorf("lista: moto com %d campos", len(p.Fields))
		}
	}
}

func TestLigacaoCaixa(t *testing.T) {
	ctx := context.Background()
	s := New(NewMemory())
	m := moto()
	if err := s.CreateProfile(ctx, &m, "n"); err != nil {
		t.Fatal(err)
	}
	p, _ := s.FindProfile(ctx, "moto")
	box := &BoxRef{Task: "helmet_detection", Label: "motorcycle", X: 10, Y: 20, W: 30, H: 40}

	// a caixa preenche o tipo de veículo
	out, err := p.Check(Values{Objects: []Object{{Type: "vehicle", Fields: map[string]any{"plate": "abc1d23"}, Box: box}}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if out.Objects[0].Fields["type"] != "motorcycle" || out.Objects[0].Box == nil || out.Objects[0].Box.Label != "motorcycle" {
		t.Errorf("preenchido: %+v", out.Objects[0])
	}
	// diferente do que a caixa diz: erro claro
	_, err = p.Check(Values{Objects: []Object{{Type: "vehicle", Fields: map[string]any{"type": "car"}, Box: box}}}, false)
	if err == nil || !strings.Contains(err.Error(), "motorcycle") {
		t.Errorf("tipo errado: %v", err)
	}
	// label que não representa o tipo
	other := *box
	other.Label = "helmet"
	if _, err = p.Check(Values{Objects: []Object{{Type: "vehicle", Box: &other}}}, false); err == nil || !strings.Contains(err.Error(), "não representa") {
		t.Errorf("label errada: %v", err)
	}
	// a mesma caixa em dois veículos
	_, err = p.Check(Values{Objects: []Object{{Type: "vehicle", Box: box}, {Type: "vehicle", Box: box}}}, false)
	if err == nil || !strings.Contains(err.Error(), "outro objeto") {
		t.Errorf("caixa repetida: %v", err)
	}
	// sem caixa, nada muda
	if out, err := p.Check(Values{Objects: []Object{{Type: "vehicle", Fields: map[string]any{"type": "car"}}}}, false); err != nil || out.Objects[0].Box != nil {
		t.Errorf("sem caixa: %v", err)
	}

	// o perfil: campo que o tipo não tem, valor inválido, tipo com caixa
	var invalid *InvalidError
	bad := moto()
	bad.Name = "bad"
	bad.Objects[0].BoxLabels = []BoxLabel{{Label: "motorcycle", Fields: map[string]any{"nada": 1}}, {Label: "car", Fields: map[string]any{"type": "aviao"}}}
	if err := s.CreateProfile(ctx, &bad, "n"); !errors.As(err, &invalid) || len(invalid.Problems) != 2 {
		t.Errorf("labels inválidas: %v", err)
	}
	boxType := Profile{Name: "cx", Objects: []ObjectType{{Key: "helmet", Box: true, Fields: []Field{}, BoxLabels: []BoxLabel{{Label: "x"}}}}}
	if err := s.CreateProfile(ctx, &boxType, "n"); !errors.As(err, &invalid) {
		t.Errorf("tipo com caixa e labels: %v", err)
	}
}

func TestAtributosDaCaixa(t *testing.T) {
	p := moto()
	p.Objects[0].BoxLabels[0].Attributes = map[string]string{" color ": "color"}
	p.Normalize()
	r := withLabels(Vehicles(), p.Objects)
	if err := r.validateLinks(); err != nil {
		t.Fatalf("mapa válido recusado: %v", err)
	}
	if r.Objects[0].BoxLabels[0].Attributes["color"] != "color" {
		t.Errorf("nome do atributo não foi limpo: %+v", r.Objects[0].BoxLabels[0].Attributes)
	}
	for name, attrs := range map[string]map[string]string{
		"campo que não existe":  {"cor": "nao_tem"},
		"campo repetido":        {"cor": "color", "colour": "color"},
		"campo que a label diz": {"kind": "type"},
	} {
		r.Objects[0].BoxLabels[0].Attributes = attrs
		if err := r.validateLinks(); err == nil {
			t.Errorf("%s: devia recusar", name)
		}
	}
}
