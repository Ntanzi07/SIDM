package store

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"SIDM/internal/boxes"
)

func boxSet(task, id, pkg string, labels ...string) BoxSet {
	set := BoxSet{
		Task: task, ImageID: id, Package: pkg, Width: 640, Height: 480,
		Source:     BoxSource{Tool: "cvat", Format: "coco", File: "t.json", FileName: id + ".jpg"},
		ImportedBy: "nathan", ImportedAt: ms(time.Now()),
	}
	for i, l := range labels {
		set.Boxes = append(set.Boxes, boxes.Box{
			Label: l, X: float64(i), Y: 1.5, W: 10, H: 20, Occluded: i == 1,
			Attributes: map[string]any{"caractere": "A"},
		})
	}
	return set
}

func TestBoxSetsPorTarefa(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.ensureBoxIndexes(ctx); err != nil {
		t.Fatal(err)
	}

	if err := s.ReplaceBoxSets(ctx, []BoxSet{
		boxSet("veiculos", "a", "p1", "placa", "carro"),
		boxSet("capacete", "a", "p1", "com_capacete"), // a mesma imagem em outra tarefa
		boxSet("veiculos", "b", "p1", "placa"),
		boxSet("veiculos", "c", "p2"), // negativa
	}); err != nil {
		t.Fatal(err)
	}

	sets, err := s.FindImageBoxSets(ctx, "a")
	if err != nil || len(sets) != 2 || sets[0].Task != "capacete" || sets[1].Task != "veiculos" {
		t.Fatalf("FindImageBoxSets = %+v, %v", sets, err)
	}
	if b := sets[1].Boxes[1]; b.Label != "carro" || !b.Occluded || b.Y != 1.5 || b.Attributes["caractere"] != "A" {
		t.Errorf("caixa = %+v", b)
	}

	// negativa gravada como [] (e não null)
	var raw bson.M
	if err := s.boxes().FindOne(ctx, bson.D{{Key: "task", Value: "veiculos"}, {Key: "image", Value: "c"}}).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	if arr, ok := raw["boxes"].(bson.A); !ok || len(arr) != 0 {
		t.Errorf("boxes da negativa = %#v", raw["boxes"])
	}

	// substituir numa tarefa não mexe na outra
	if err := s.ReplaceBoxSets(ctx, []BoxSet{boxSet("veiculos", "a", "p1", "placa")}); err != nil {
		t.Fatal(err)
	}
	got, err := s.FindBoxSets(ctx, "veiculos", []string{"a", "c", "x"})
	if err != nil || len(got) != 2 || len(got["a"].Boxes) != 1 || len(got["c"].Boxes) != 0 {
		t.Errorf("FindBoxSets = %+v, %v", got, err)
	}
	if got, _ := s.FindBoxSets(ctx, "capacete", []string{"a"}); len(got["a"].Boxes) != 1 {
		t.Error("a outra tarefa não podia mudar")
	}

	counts, err := s.PackageBoxCounts(ctx, "p1")
	if err != nil || len(counts) != 2 || counts[0].Task != "capacete" || counts[1].Images != 2 || counts[1].Boxes != 2 {
		t.Errorf("PackageBoxCounts = %+v, %v", counts, err)
	}

	if err := s.DeleteImageBoxSets(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if sets, _ := s.FindImageBoxSets(ctx, "a"); len(sets) != 0 {
		t.Error("apagar a imagem deveria apagar as caixas dela em todas as tarefas")
	}
}

func TestMigracaoDasCaixasAntigas(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	// Como era antes: sem os índices novos (o testStore já cria), e as caixas
	// com _id = sha1, sem task/image.
	if err := s.boxes().Indexes().DropAll(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.boxes().InsertMany(ctx, []any{
		bson.D{{Key: "_id", Value: "a"}, {Key: "package", Value: "p1"}, {Key: "boxes", Value: bson.A{bson.D{{Key: "label", Value: "placa"}}}}},
		bson.D{{Key: "_id", Value: "b"}, {Key: "package", Value: "p1"}, {Key: "boxes", Value: bson.A{bson.D{{Key: "label", Value: "carro"}}}}},
	}); err != nil {
		t.Fatal(err)
	}
	for range 2 { // idempotente
		if err := s.ensureTaskIndexes(ctx); err != nil {
			t.Fatal(err)
		}
		if err := s.ensureBoxIndexes(ctx); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.FindBoxSets(ctx, DefaultTask, []string{"a", "b"})
	if err != nil || len(got) != 2 || got["a"].Boxes[0].Label != "placa" {
		t.Errorf("migradas = %+v, %v", got, err)
	}
	task, err := s.FindTask(ctx, DefaultTask)
	if err != nil || len(task.Labels) != 2 {
		t.Errorf("tarefa padrão = %+v, %v", task, err)
	}
}

func TestCaixasNoRenomearEApagarPacote(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	s.ReplaceBoxSets(ctx, []BoxSet{boxSet("t", "a", "velho", "placa"), boxSet("t", "z", "outro", "placa")})

	if err := s.PackageRenamed(ctx, "velho", "novo"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.FindImageBoxSets(ctx, "a"); len(got) != 1 || got[0].Package != "novo" {
		t.Errorf("as caixas deveriam ir junto com o pacote: %+v", got)
	}

	if err := s.PackageDeleted(ctx, "novo"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.FindImageBoxSets(ctx, "a"); len(got) != 0 {
		t.Error("apagar o pacote deveria apagar as caixas")
	}
	if got, _ := s.FindImageBoxSets(ctx, "z"); len(got) != 1 {
		t.Error("as caixas de outro pacote não podem ser apagadas")
	}
}
