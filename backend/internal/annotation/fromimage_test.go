package annotation

import (
	"strings"
	"testing"
)

func TestFillFromImage(t *testing.T) {
	p := Vehicles()
	// a resolução do padrão vem do arquivo
	var res Field
	for _, f := range p.Fields {
		if f.Key == "resolution" {
			res = f
		}
	}
	if res.FromImage != FromImageSize {
		t.Fatalf("resolution do veiculos: from_image = %q", res.FromImage)
	}

	// o que veio é trocado pelo medido
	out, w := p.FillFromImage(map[string]any{"resolution": "1920x1080", "weather": "rain"}, ImageSize{640, 480})
	if out["resolution"] != "640x480" || out["weather"] != "rain" || len(w) != 0 {
		t.Errorf("medido: %v %v", out, w)
	}
	// tamanho fora das opções: fica a medida (com aviso), e o Check aceita
	out, w = p.FillFromImage(map[string]any{"resolution": "640x480"}, ImageSize{33, 33})
	if out["resolution"] != "33x33" || len(w) != 1 || !strings.Contains(w[0], "33x33") {
		t.Errorf("fora das opções: %v %v", out, w)
	}
	if _, err := res.Clean("33x33"); err != nil {
		t.Errorf("Clean da medida fora das opções: %v", err)
	}
	// quem anota não inventa valor: só o formato de medida passa
	if _, err := res.Clean("grande"); err == nil {
		t.Error("Clean aceitou um valor que não é opção nem medida")
	}
	// sem medir: fica como veio
	out, _ = p.FillFromImage(map[string]any{"resolution": "640x480"}, ImageSize{})
	if out["resolution"] != "640x480" {
		t.Errorf("sem medida: %v", out)
	}
	// num campo de texto, qualquer tamanho vale
	q := Profile{Name: "x", Fields: []Field{{Key: "tamanho", Type: TypeText, FromImage: FromImageSize}}}
	out, _ = q.FillFromImage(nil, ImageSize{3000, 2000})
	if out["tamanho"] != "3000x2000" {
		t.Errorf("texto: %v", out)
	}
}

// O import (Fit) não traz valor para o que vem da imagem.
func TestFitIgnoraOQueVemDaImagem(t *testing.T) {
	p := Vehicles()
	out, _ := p.Fit(Values{Fields: map[string]any{"resolution": "1920x1080", "weather": "rain"}})
	if _, ok := out.Fields["resolution"]; ok || out.Fields["weather"] != "rain" {
		t.Errorf("fit: %v", out.Fields)
	}
}

func TestFromImageValida(t *testing.T) {
	cases := []struct {
		name  string
		p     Profile
		where string
	}{
		{"valor desconhecido", Profile{Name: "a", Fields: []Field{{Key: "r", Type: TypeText, FromImage: "cor"}}}, "fields[0].from_image"},
		{"tipo número", Profile{Name: "a", Fields: []Field{{Key: "r", Type: TypeNumber, FromImage: FromImageSize}}}, "fields[0].from_image"},
		{"com repeat", Profile{Name: "a", Fields: []Field{{Key: "r", Type: TypeText, FromImage: FromImageSize, Repeat: true}}}, "fields[0].from_image"},
		{"num objeto", Profile{Name: "a", Objects: []ObjectType{{Key: "v", Fields: []Field{{Key: "r", Type: TypeText, FromImage: FromImageSize}}}}}, "objects[0].fields[0].from_image"},
	}
	for _, c := range cases {
		err := c.p.Validate()
		inv, ok := err.(*InvalidError)
		if !ok {
			t.Errorf("%s: esperava erro, veio %v", c.name, err)
			continue
		}
		found := false
		for _, pr := range inv.Problems {
			found = found || pr.Path == c.where
		}
		if !found {
			t.Errorf("%s: sem problema em %s: %+v", c.name, c.where, inv.Problems)
		}
	}
	if err := (&Profile{Name: "a", Fields: []Field{{Key: "r", Type: TypeText, FromImage: FromImageSize}}}).Validate(); err != nil {
		t.Errorf("válido: %v", err)
	}
}
