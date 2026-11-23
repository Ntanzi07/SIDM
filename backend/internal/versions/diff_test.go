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
	if d.Counts != (DiffCounts{Added: 1, Removed: 1, Changed: 1, Unchanged: 1}) {
		t.Errorf("counts = %+v", d.Counts)
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

// A pesquisa dentro de uma versão: filtra os itens congelados (a imagem
// mudou depois, a versão não) e conta as caixas só das que passam.
func TestPesquisaNaVersao(t *testing.T) {
	ctx := context.Background()
	s, _, img, _ := newTestService()
	if _, err := s.CreateDataset(ctx, DatasetInput{Name: "tudo", Profile: "veiculos"}, "n"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateVersion(ctx, "tudo", "v1", "n"); err != nil {
		t.Fatal(err)
	}
	// depois da v1: a "b" vira chuva; na v1 ela continua "clear"
	b := img.imgs["b"]
	b.Fields = map[string]any{"weather": "rain"}
	img.imgs["b"] = b

	all, err := s.VersionStats(ctx, "tudo", 1, map[string][]string{})
	if err != nil {
		t.Fatal(err)
	}
	if all.Images.Total != 3 || all.Labels.Images != 3 {
		t.Errorf("sem filtro: %+v / %+v", all.Images, all.Labels)
	}

	rain, err := s.VersionStats(ctx, "tudo", 1, map[string][]string{"fields.weather": {"rain"}, "status": {"all"}})
	if err != nil {
		t.Fatal(err)
	}
	if rain.Images.Total != 1 || rain.Images.Packages["p1"] != 1 {
		t.Errorf("chuva na v1 (só a a): %+v", rain.Images)
	}
	// só a tarefa pedida, com as labels zeradas que não aparecem
	if len(rain.Labels.Tasks) != 2 {
		t.Fatalf("tarefas: %+v", rain.Labels.Tasks)
	}
	only, err := s.VersionStats(ctx, "tudo", 1, map[string][]string{"task": {"capacete"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(only.Labels.Tasks) != 1 || only.Labels.Tasks[0].Boxes != 2 || only.Labels.Tasks[0].Labels["no_helmet"].Boxes != 0 {
		t.Errorf("capacete: %+v", only.Labels.Tasks)
	}
	if _, ok := only.Labels.Tasks[0].Labels["no_helmet"]; !ok {
		t.Error("a label sem caixa tem que aparecer (zero)")
	}

	p2, _ := s.VersionStats(ctx, "tudo", 1, map[string][]string{"package": {"p2"}})
	if p2.Images.Total != 1 {
		t.Errorf("pacote p2: %d", p2.Images.Total)
	}
	if _, err := s.VersionStats(ctx, "tudo", 1, map[string][]string{"fields.nao": {"x"}}); !errors.Is(err, platform.ErrInvalid) {
		t.Errorf("campo que não existe: %v", err)
	}
	if _, err := s.VersionStats(ctx, "tudo", 9, nil); !errors.Is(err, platform.ErrNotFound) {
		t.Errorf("versão que não existe: %v", err)
	}
}

// O "agora": o estado atual montado na hora, sem gravar; o diff e a pesquisa
// funcionam com ele como se fosse uma versão.
func TestAgora(t *testing.T) {
	ctx := context.Background()
	s, mem, img, _ := newTestService()
	if _, err := s.CreateDataset(ctx, DatasetInput{Name: "tudo", Profile: "veiculos"}, "n"); err != nil {
		t.Fatal(err)
	}
	// sem versão: o agora vem com as 3 verificadas e o diff é tudo novo
	now, err := s.Current(ctx, "tudo")
	if err != nil || now.Number != 0 || now.Summary.Images.Total != 3 {
		t.Fatalf("agora sem versão: %+v %v", now, err)
	}
	if d, err := s.Diff(ctx, "tudo", 0, Now); err != nil || d.From != nil || len(d.Added) != 3 {
		t.Errorf("diff do agora sem versão: %+v %v", d, err)
	}
	if _, err := s.CreateVersion(ctx, "tudo", "v1", "n"); err != nil {
		t.Fatal(err)
	}
	versions, items := len(mem.Versions), len(mem.Items)

	// muda a "a" e verifica a "d": o agora mostra, nada é gravado
	a := img.imgs["a"]
	a.Fields = map[string]any{"weather": "clear"}
	img.imgs["a"] = a
	dd := img.imgs["d"]
	dd.Status = images.StatusVerified
	img.imgs["d"] = dd

	d, err := s.Diff(ctx, "tudo", 0, Now) // sem from: contra a mais nova
	if err != nil {
		t.Fatal(err)
	}
	if d.From.Number != 1 || d.To.Number != 0 || d.Counts != (DiffCounts{Added: 1, Changed: 1, Unchanged: 2}) {
		t.Errorf("diff v1 → agora: from %+v to %+v %+v", d.From, d.To, d.Counts)
	}
	it, err := s.DiffItem(ctx, "tudo", 1, Now, "a")
	if err != nil || it.Status != StatusChanged || len(it.Fields) != 1 {
		t.Errorf("detalhe da a no agora: %+v %v", it, err)
	}
	st, err := s.VersionStats(ctx, "tudo", Now, map[string][]string{"fields.weather": {"clear"}})
	if err != nil || st.Images.Total != 2 {
		t.Errorf("pesquisa no agora (a e b claras): %+v %v", st, err)
	}
	if len(mem.Versions) != versions || len(mem.Items) != items {
		t.Error("o agora gravou alguma coisa")
	}
	if _, err := s.Diff(ctx, "tudo", Now, 1); !errors.Is(err, platform.ErrInvalid) {
		t.Errorf("agora como lado velho: %v", err)
	}
}
