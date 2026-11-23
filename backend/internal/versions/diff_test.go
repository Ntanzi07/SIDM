package versions

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"SIDM/internal/annotation"
	"SIDM/internal/boxes"
	"SIDM/internal/images"
	"SIDM/internal/platform"
)

func refs(list []ItemRef) []string {
	out := []string{}
	for _, r := range list {
		out = append(out, r.SHA1)
	}
	return out
}

// Da v1 para a v2: a "a" muda (campo, objeto e caixas), a "b" sai (voltou a
// pendente), a "d" entra (foi verificada) e a "c" fica igual.
func TestDiffEntreVersoes(t *testing.T) {
	ctx := context.Background()
	s, _, img, bx := newTestService()
	if _, err := s.CreateDataset(ctx, DatasetInput{Name: "tudo", Profile: "veiculos"}, "n"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateVersion(ctx, "tudo", "v1", "n"); err != nil {
		t.Fatal(err)
	}

	// a v1 sozinha: tudo adicionado, sem from
	d, err := s.Diff(ctx, "tudo", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if d.From != nil || d.To.Number != 1 || !reflect.DeepEqual(refs(d.Added), []string{"a", "b", "c"}) {
		t.Errorf("v1 sozinha: %+v", d)
	}

	a := img.imgs["a"]
	a.Fields = map[string]any{"weather": "clear"}
	a.Objects = []annotation.Object{{Type: "veiculo", Fields: map[string]any{"cor": "preto"}}}
	img.imgs["a"] = a
	b := img.imgs["b"]
	b.Status = images.StatusPending
	img.imgs["b"] = b
	dd := img.imgs["d"]
	dd.Status = images.StatusVerified
	img.imgs["d"] = dd
	bx.sets[0].Boxes = []boxes.Box{{Label: "placa", X: 9, Y: 2, W: 3, H: 4}} // placa: movida
	bx.sets[1].Boxes = []boxes.Box{{Label: "helmet"}, {Label: "no_helmet"}}  // capacete: 1 helmet virou no_helmet
	if _, err := s.CreateVersion(ctx, "tudo", "v2", "n"); err != nil {
		t.Fatal(err)
	}

	d, err = s.Diff(ctx, "tudo", 0, 0) // padrão: a mais nova contra a anterior
	if err != nil {
		t.Fatal(err)
	}
	if d.From.Number != 1 || d.To.Number != 2 {
		t.Errorf("versões: %+v %+v", d.From, d.To)
	}
	if !reflect.DeepEqual(refs(d.Added), []string{"d"}) || !reflect.DeepEqual(refs(d.Removed), []string{"b"}) ||
		!reflect.DeepEqual(refs(d.Changed), []string{"a"}) || d.Unchanged != 1 {
		t.Errorf("diff: +%v -%v ~%v =%d", refs(d.Added), refs(d.Removed), refs(d.Changed), d.Unchanged)
	}
	if d.Removed[0].FileName != "b.jpg" || d.Removed[0].OriginalName != "b.jpg" {
		t.Errorf("removida com o nome da versão velha: %+v", d.Removed[0])
	}
	// ao contrário: o que entrou vira o que saiu
	if r, _ := s.Diff(ctx, "tudo", 2, 1); !reflect.DeepEqual(refs(r.Added), []string{"b"}) || !reflect.DeepEqual(refs(r.Removed), []string{"d"}) {
		t.Errorf("de 2 para 1: %+v", r)
	}

	ia, err := s.DiffItem(ctx, "tudo", 1, 2, "a")
	if err != nil {
		t.Fatal(err)
	}
	if ia.Status != StatusChanged || ia.FileName != "a_v2.jpg" {
		t.Errorf("a: %+v", ia)
	}
	if !reflect.DeepEqual(ia.Fields, []Change{{Key: "fields.weather", From: "rain", To: "clear"}}) {
		t.Errorf("campos: %+v", ia.Fields)
	}
	if !reflect.DeepEqual(ia.Objects, []Change{{Key: "objects.veiculo.cor", From: nil, To: "preto"}}) {
		t.Errorf("objetos: %+v", ia.Objects)
	}
	want := []TaskChange{
		{Task: "capacete", From: 2, To: 2, Labels: []LabelChange{{"helmet", 2, 1}, {"no_helmet", 0, 1}}},
		{Task: "placa", From: 1, To: 1, Labels: []LabelChange{}, Edited: true},
	}
	if !reflect.DeepEqual(ia.Tasks, want) {
		t.Errorf("tarefas:\n%+v\nquer\n%+v", ia.Tasks, want)
	}

	if ib, _ := s.DiffItem(ctx, "tudo", 1, 2, "b"); ib.Status != StatusRemoved || ib.FileName != "b.jpg" ||
		!reflect.DeepEqual(ib.Fields, []Change{{Key: "fields.weather", From: "clear", To: nil}}) {
		t.Errorf("b: %+v", ib)
	}
	if id, _ := s.DiffItem(ctx, "tudo", 1, 2, "d"); id.Status != StatusAdded ||
		!reflect.DeepEqual(id.Tasks, []TaskChange{{Task: "capacete", From: 0, To: 1, Labels: []LabelChange{{"helmet", 0, 1}}}}) {
		t.Errorf("d: %+v", id)
	}
	if ic, _ := s.DiffItem(ctx, "tudo", 1, 2, "c"); ic.Status != StatusUnchanged || len(ic.Fields)+len(ic.Objects)+len(ic.Tasks) != 0 {
		t.Errorf("c: %+v", ic)
	}

	// erros
	if _, err := s.Diff(ctx, "tudo", 2, 2); !errors.Is(err, platform.ErrInvalid) {
		t.Errorf("mesma versão: %v", err)
	}
	if _, err := s.Diff(ctx, "tudo", 1, 9); !errors.Is(err, platform.ErrNotFound) {
		t.Errorf("versão que não existe: %v", err)
	}
	if _, err := s.DiffItem(ctx, "tudo", 1, 2, "zzz"); !errors.Is(err, platform.ErrNotFound) {
		t.Errorf("imagem fora das duas: %v", err)
	}
	if _, err := s.CreateDataset(ctx, DatasetInput{Name: "novo", Profile: "veiculos"}, "n"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Diff(ctx, "novo", 0, 0); !errors.Is(err, platform.ErrInvalid) {
		t.Errorf("sem versões: %v", err)
	}
}

// Com mais de um objeto do mesmo tipo, a chave leva a posição.
func TestDiffObjetosRepetidos(t *testing.T) {
	a := []annotation.Object{{Type: "veiculo", Fields: map[string]any{"cor": "preto"}}}
	b := []annotation.Object{{Type: "veiculo", Fields: map[string]any{"cor": "preto"}}, {Type: "veiculo", Fields: map[string]any{"cor": "azul"}}}
	got := diffObjects(a, b)
	if !reflect.DeepEqual(got, []Change{{Key: "objects.veiculo[2].cor", From: nil, To: "azul"}}) {
		t.Errorf("%+v", got)
	}
}

// O Mongo devolve listas como bson.A e documentos como bson.D: o diff
// compara e mostra como JSON comum.
func TestPlain(t *testing.T) {
	v := plain(bson.A{"x", bson.D{{Key: "k", Value: bson.A{int32(1)}}}})
	want := []any{"x", map[string]any{"k": []any{int32(1)}}}
	if !reflect.DeepEqual(v, want) {
		t.Errorf("%#v", v)
	}
	if !same(bson.A{"x", "y"}, []any{"x", "y"}) || same([]any{"x", "y"}, []any{"y", "x"}) || !same(int32(3), 3.0) {
		t.Error("same")
	}
}
