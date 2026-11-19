package images

import (
	"context"
	"reflect"
	"testing"
	"time"

	"SIDM/internal/annotation"
)

// statsImages: quatro imagens com campo de lista (conditions), sim/não
// (night) e objetos, em dois pacotes; uma pendente.
func statsImages() []*Image {
	at := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	mk := func(sha, pkg, status string, fields map[string]any, objs ...annotation.Object) *Image {
		img := newImage(sha, at)
		img.Package, img.Status, img.Fields, img.Objects = pkg, status, fields, objs
		if img.Objects == nil {
			img.Objects = []annotation.Object{}
		}
		return img
	}
	return []*Image{
		mk("a", "p1", StatusVerified, map[string]any{"weather": "rain", "conditions": []string{"dirty", "no_damage"}, "night": true},
			veh("AAA1A11", "car", "white"), veh("BBB2B22", "truck", "white")),
		mk("b", "p1", StatusVerified, map[string]any{"weather": "clear", "conditions": []string{"no_damage"}, "night": false},
			veh("CCC3C33", "car", "black")),
		mk("c", "p2", StatusVerified, map[string]any{"weather": "rain", "conditions": []string{}},
			annotation.Object{Type: "vehicle", Fields: map[string]any{"type": "car"}}), // sem cor
		mk("d", "p2", StatusPending, map[string]any{"weather": "rain", "night": true}),
	}
}

var statsSpec = StatsSpec{Fields: []string{"weather", "conditions", "night"}, Objects: map[string][]string{"vehicle": {"type", "color"}}}

// wantStats: as verificadas de p1 e p2.
func wantStats() *Stats {
	return &Stats{
		Total:    3,
		Packages: Counts{"p1": 2, "p2": 1},
		Status:   Counts{"verified": 3},
		Fields: map[string]Counts{
			"weather":    {"rain": 2, "clear": 1},
			"conditions": {"dirty": 1, "no_damage": 2}, // a soma (3) não é o total de imagens com o campo (2)
			"night":      {"true": 1, "false": 1},
		},
		Missing: Counts{"weather": 0, "conditions": 1, "night": 1}, // c: lista vazia e sem night
		Objects: map[string]*ObjectStats{"vehicle": {Images: 3, Count: 4, Fields: map[string]Counts{
			"type":  {"car": 3, "truck": 1},
			"color": {"white": 2, "black": 1},
		}, Missing: Counts{"type": 0, "color": 1}}},
	}
}

func memoryWith(imgs []*Image) *Memory {
	m := NewMemory()
	for _, img := range imgs {
		m.Images[img.SHA1] = *img
	}
	return m
}

func TestStatsNaMemoria(t *testing.T) {
	ctx := context.Background()
	m := memoryWith(statsImages())
	got, err := m.Stats(ctx, ImageFilter{Packages: []string{"p1", "p2"}, Status: StatusVerified}, statsSpec)
	if err != nil {
		t.Fatal(err)
	}
	if want := wantStats(); !reflect.DeepEqual(got, want) {
		t.Errorf("stats =\n%+v\nquer\n%+v", got, want)
	}

	// filtro de objeto: só a imagem com caminhão; as contagens de objeto
	// continuam contando TODOS os objetos dela
	got, _ = m.Stats(ctx, ImageFilter{Packages: []string{"p1", "p2"}, Status: StatusVerified,
		Objects: []ObjectFilter{{Type: "vehicle", Fields: map[string]any{"type": "truck"}}}}, statsSpec)
	if got.Total != 1 || got.Objects["vehicle"].Count != 2 || got.Fields["conditions"]["dirty"] != 1 {
		t.Errorf("com caminhão: %+v %+v", got, got.Objects["vehicle"])
	}
	// sem nada: as chaves do spec aparecem zeradas
	got, _ = m.Stats(ctx, ImageFilter{Packages: []string{"outro"}}, statsSpec)
	if got.Total != 0 || got.Fields["weather"] == nil || got.Objects["vehicle"].Fields["type"] == nil || got.Missing["weather"] != 0 {
		t.Errorf("vazio: %+v", got)
	}
}

func TestSpecDoPerfil(t *testing.T) {
	p := &annotation.Profile{Name: "x",
		Fields: []annotation.Field{{Key: "weather", Type: annotation.TypeChoice}, {Key: "note", Type: annotation.TypeText}, {Key: "night", Type: annotation.TypeBool}},
		Objects: []annotation.ObjectType{
			{Key: "vehicle", Fields: []annotation.Field{{Key: "type", Type: annotation.TypeChoice}, {Key: "plate", Type: annotation.TypeText}}},
			{Key: "helmet", Box: true, Fields: []annotation.Field{{Key: "color", Type: annotation.TypeChoice}}},
		}}
	got := SpecOf(p)
	want := StatsSpec{Fields: []string{"weather", "night"}, Objects: map[string][]string{"vehicle": {"type"}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("spec = %+v", got)
	}
}
