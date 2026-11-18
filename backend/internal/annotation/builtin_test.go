package annotation

import (
	"testing"

	"SIDM/internal/filename"
)

// O perfil "veiculos" tem que ser exatamente o padrão v2: as mesmas opções,
// com os mesmos códigos, e um nome em português para cada uma.
func TestVeiculosIgualAoNomeV2(t *testing.T) {
	p := Vehicles()
	if err := p.Validate(); err != nil {
		t.Fatalf("o perfil padrão não passa na própria validação: %v (%+v)", err, err.(*InvalidError).Problems)
	}

	// campo do perfil -> tabela do filename
	tables := map[string]string{
		"direction": "direction", "resolution": "resolution", "capture_angle": "capture_angle",
		"day_time": "day_time", "image_range": "image_range", "weather": "weather", "conditions": "conditions",
		"plate_type": "plate_type", "plate_category": "plate_category", "type": "vehicle_type", "color": "vehicle_color",
	}
	fields := append([]Field{}, p.Fields...)
	fields = append(fields, p.Objects[0].Fields...)
	seen := 0
	for _, f := range fields {
		table, ok := tables[f.Key]
		if !ok {
			continue
		}
		seen++
		codes := filename.Codes()[table]
		if len(f.Options) != len(codes) {
			t.Errorf("%s: %d opções, o v2 tem %d", f.Key, len(f.Options), len(codes))
		}
		for _, o := range f.Options {
			if codes[o.Code] != o.Value {
				t.Errorf("%s: código %q -> %q, no v2 é %q", f.Key, o.Code, o.Value, codes[o.Code])
			}
			if o.Label == "" || (o.Label == o.Value && f.Key != "resolution") {
				t.Errorf("%s: %q sem nome em português", f.Key, o.Value)
			}
		}
	}
	if seen != len(tables) {
		t.Errorf("achei %d dos %d campos com tabela", seen, len(tables))
	}
	if p.Objects[0].Key != "vehicle" || p.Objects[0].Box {
		t.Errorf("veículo: %+v", p.Objects[0])
	}
}

func TestBuiltin(t *testing.T) {
	if _, ok := Builtin(DefaultProfile); !ok {
		t.Error("faltou o padrão")
	}
	if _, ok := Builtin("nao-existe"); ok {
		t.Error("achou o que não existe")
	}
	// Cópia: mexer no retorno não muda o padrão.
	a, _ := Builtin(DefaultProfile)
	a.Fields[0].Label = "mudou"
	if b, _ := Builtin(DefaultProfile); b.Fields[0].Label == "mudou" {
		t.Error("o padrão foi alterado por quem recebeu")
	}
}
