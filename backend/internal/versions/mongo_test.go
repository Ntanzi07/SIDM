package versions

// Testes de INTEGRAÇÃO do MongoRepo (ver platform/mongotest): sem Mongo, são pulados.

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"SIDM/internal/platform"
	"SIDM/internal/platform/mongotest"
)

func testRepo(t *testing.T) *MongoRepo {
	t.Helper()
	r := NewMongoRepo(mongotest.DB(t))
	if err := r.EnsureIndexes(context.Background()); err != nil {
		t.Fatal(err)
	}
	return r
}

// O caminho todo pelo serviço, gravando no Mongo: o resumo e as caixas
// voltam iguais do banco, o número é único por dataset e os itens paginam.
func TestMongoVersoes(t *testing.T) {
	ctx := context.Background()
	r := testRepo(t)
	s, _, img, _ := newTestService()
	s.repo = r

	if _, err := s.CreateDataset(ctx, DatasetInput{Name: "motos", Profile: "veiculos", Query: Query{Packages: []string{"p1"}}}, "nathan"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateDataset(ctx, DatasetInput{Name: "motos", Profile: "veiculos"}, "nathan"); !errors.Is(err, platform.ErrDuplicate) {
		t.Errorf("nome repetido: %v", err)
	}
	v1, err := s.CreateVersion(ctx, "motos", "primeira", "nathan")
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.FindVersion(ctx, "motos", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Summary, v1.Summary) || !got.CreatedAt.Equal(v1.CreatedAt) {
		t.Errorf("resumo do banco =\n%+v\nquer\n%+v", got.Summary, v1.Summary)
	}

	// o mesmo número de novo: recusado pelo índice
	if err := r.InsertVersion(ctx, &Version{ID: "x", Dataset: "motos", Number: 1}); !errors.Is(err, platform.ErrDuplicate) {
		t.Errorf("número repetido: %v", err)
	}
	if _, err := s.CreateVersion(ctx, "motos", "segunda", "nathan"); err != nil {
		t.Fatal(err)
	}
	if last, _ := r.LastVersion(ctx, "motos"); last == nil || last.Number != 2 {
		t.Errorf("última: %+v", last)
	}
	if list, _ := r.ListVersions(ctx, "motos"); len(list) != 2 || list[0].Number != 2 {
		t.Errorf("versões: %+v", list)
	}

	// itens: em ordem de sha1, paginados, com as caixas
	items, total, err := s.Items(ctx, "motos", 1, 1, 1)
	if err != nil || total != 2 || len(items) != 1 || items[0].SHA1 != "b" {
		t.Fatalf("página 2: %+v %d %v", items, total, err)
	}
	items, _, _ = s.Items(ctx, "motos", 1, 10, 0)
	if b := items[0].Boxes["placa"]; b.Width != 100 || len(b.Boxes) != 1 || b.Boxes[0].W != 3 {
		t.Errorf("caixas do banco: %+v", items[0].Boxes)
	}
	if items[0].Hash == "" || items[0].Fields["weather"] != "rain" {
		t.Errorf("anotação do banco: %+v", items[0])
	}

	// diff pelo banco: v1 e v2 iguais
	d, err := s.Diff(ctx, "motos", 1, 2)
	if err != nil || d.Unchanged != 2 || len(d.Added)+len(d.Removed)+len(d.Changed) != 0 {
		t.Errorf("diff igual: %+v %v", d, err)
	}
	if _, err := r.FindItem(ctx, got.ID, "zzz"); !errors.Is(err, platform.ErrNotFound) {
		t.Errorf("item que não existe: %v", err)
	}

	// uma lista volta do banco como bson.A: o detalhe mostra como lista
	a := img.imgs["a"]
	a.Fields = map[string]any{"weather": "rain", "tags": []any{"x", "y"}}
	img.imgs["a"] = a
	if _, err := s.CreateVersion(ctx, "motos", "tags", "nathan"); err != nil {
		t.Fatal(err)
	}
	a.Fields = map[string]any{"weather": "rain", "tags": []any{"x"}}
	img.imgs["a"] = a
	if _, err := s.CreateVersion(ctx, "motos", "menos tags", "nathan"); err != nil {
		t.Fatal(err)
	}
	ia, err := s.DiffItem(ctx, "motos", 3, 4, "a")
	if err != nil {
		t.Fatal(err)
	}
	if ia.Status != StatusChanged || !reflect.DeepEqual(ia.Fields, []Change{{Key: "fields.tags", From: []any{"x", "y"}, To: []any{"x"}}}) {
		t.Errorf("tags: %+v", ia)
	}
	// a pesquisa na versão lendo do banco: a lista volta como bson.A e conta igual
	st, err := s.VersionStats(ctx, "motos", 4, map[string][]string{"fields.weather": {"rain"}})
	if err != nil {
		t.Fatal(err)
	}
	if st.Images.Total == 0 {
		t.Errorf("pesquisa na versão pelo banco: %+v", st.Images)
	}
	if d, _ := s.Diff(ctx, "motos", 3, 4); len(d.Changed) != 1 || d.Changed[0].SHA1 != "a" || d.Changed[0].FileName != "a_v2.jpg" {
		t.Errorf("diff tags: %+v", d)
	}

	// com versões, não apaga; sem, apaga
	var conflict *ConflictError
	if err := s.DeleteDataset(ctx, "motos"); !errors.As(err, &conflict) {
		t.Errorf("apagar com versões: %v", err)
	}
	if _, err := s.CreateDataset(ctx, DatasetInput{Name: "vazio", Profile: "veiculos"}, "n"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteDataset(ctx, "vazio"); err != nil {
		t.Errorf("apagar sem versões: %v", err)
	}
	if _, err := r.FindDataset(ctx, "vazio"); !errors.Is(err, platform.ErrNotFound) {
		t.Errorf("apagado: %v", err)
	}
}
