package annotation

import "testing"

func motoComCor() *Profile {
	p := Vehicles()
	p.Objects[0].BoxLabels = []BoxLabel{{Label: "motorcycle", Fields: map[string]any{"type": "motorcycle"}, Attributes: map[string]string{"color": "color"}}}
	return &p
}

func TestLigarCaixasNoImport(t *testing.T) {
	p := motoComCor()
	m1 := SeenBox{Label: "motorcycle", X: 10, Y: 10, W: 100, H: 100, Attributes: map[string]any{"color": "blue"}}
	m2 := SeenBox{Label: "motorcycle", X: 200, Y: 10, W: 100, H: 100}
	helmet := SeenBox{Label: "helmet", X: 20, Y: 5, W: 20, H: 20}

	// imagem nova (não verificada), sem objetos: cada moto vira um veículo
	out, res := p.LinkBoxes(nil, "t", []SeenBox{m1, helmet, m2}, true)
	if res.Created != 2 || len(out) != 2 || res.Loose != 0 {
		t.Fatalf("criar: %+v %+v", res, out)
	}
	if out[0].Fields["type"] != "motorcycle" || out[0].Fields["color"] != "blue" || out[0].Fields["plate_type"] != "mercosul" || out[0].Box.Label != "motorcycle" {
		t.Errorf("objeto criado: %+v", out[0])
	}
	if _, ok := out[1].Fields["color"]; ok {
		t.Errorf("sem atributo, sem cor: %+v", out[1].Fields)
	}

	// de novo, com as caixas um pouco mexidas (reimport): só atualiza a posição
	m1b := m1
	m1b.X = 12
	again, res := p.LinkBoxes(out, "t", []SeenBox{m1b, helmet, m2}, true)
	if res.Moved != 1 || res.Created != 0 || len(again) != 2 || again[0].Box.X != 12 {
		t.Errorf("reimport: %+v %+v", res, again[0].Box)
	}
	if out[0].Box.X != 10 {
		t.Error("mudou os objetos de entrada")
	}

	// um veículo sem caixa e uma moto: liga, preenche o vazio
	car := []Object{{Type: "vehicle", Fields: map[string]any{"plate": "ABC1234"}}}
	got, res := p.LinkBoxes(car, "t", []SeenBox{m1}, true)
	if res.Linked != 1 || len(got) != 1 || got[0].Fields["type"] != "motorcycle" || got[0].Fields["color"] != "blue" || got[0].Fields["plate"] != "ABC1234" {
		t.Errorf("ligar: %+v %+v", res, got)
	}
	// o veículo é carro: a label contradiz, fica solta
	car[0].Fields["type"] = "car"
	_, res = p.LinkBoxes(car, "t", []SeenBox{m1}, true)
	if res.Linked != 0 || res.Loose != 1 {
		t.Errorf("contradiz: %+v", res)
	}
	// dois veículos sem caixa e duas motos: ambíguo
	two := []Object{{Type: "vehicle", Fields: map[string]any{}}, {Type: "vehicle", Fields: map[string]any{}}}
	_, res = p.LinkBoxes(two, "t", []SeenBox{m1, m2}, true)
	if res.Changed() || res.Loose != 2 {
		t.Errorf("ambíguo: %+v", res)
	}

	// verificada: não cria; liga só se não muda nenhum valor
	_, res = p.LinkBoxes(nil, "t", []SeenBox{m1}, false)
	if res.Changed() || res.Loose != 1 {
		t.Errorf("verificada sem objeto: %+v", res)
	}
	moto := []Object{{Type: "vehicle", Fields: map[string]any{"type": "motorcycle"}}}
	got, res = p.LinkBoxes(moto, "t", []SeenBox{m1}, false)
	if res.Linked != 1 || got[0].Fields["color"] != nil {
		t.Errorf("verificada: liga sem preencher: %+v %+v", res, got[0].Fields)
	}
	empty := []Object{{Type: "vehicle", Fields: map[string]any{}}}
	if _, res = p.LinkBoxes(empty, "t", []SeenBox{m1}, false); res.Linked != 0 {
		t.Errorf("verificada: ligar mudaria o tipo: %+v", res)
	}

	// já ligado em outra tarefa: não cria de novo (seria a mesma moto)
	other := []Object{{Type: "vehicle", Fields: map[string]any{}, Box: &BoxRef{Task: "placas", Label: "motorcycle", W: 1, H: 1}}}
	if got, res = p.LinkBoxes(other, "t", []SeenBox{m1}, true); res.Created != 0 || res.Loose != 1 || len(got) != 1 {
		t.Errorf("outra tarefa: %+v", res)
	}

	// máximo do tipo
	p.Objects[0].Max = 1
	if _, res = p.LinkBoxes(nil, "t", []SeenBox{m1, m2}, true); res.Created != 1 || res.Loose != 1 {
		t.Errorf("máximo: %+v", res)
	}
}
