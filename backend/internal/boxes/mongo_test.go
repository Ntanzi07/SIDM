package boxes

// Testes de INTEGRAÇÃO do MongoRepo (ver platform/mongotest): sem Mongo, são pulados.

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"SIDM/internal/platform"
	"SIDM/internal/platform/mongotest"
)

func boxSet(task, id, pkg string, labels ...string) Set {
	set := Set{
		Task: task, ImageID: id, Package: pkg, Width: 640, Height: 480,
		Source:     Source{Tool: "cvat", Format: "coco", File: "t.json", FileName: id + ".jpg"},
		ImportedBy: "nathan", ImportedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
	for i, l := range labels {
		set.Boxes = append(set.Boxes, Box{
			Label: l, X: float64(i), Y: 1.5, W: 10, H: 20, Occluded: i == 1,
			Attributes: map[string]any{"caractere": "A"},
		})
	}
	return set
}

func TestMongoCaixasPorTarefa(t *testing.T) {
	r := testRepo(t)
	ctx := context.Background()
	if err := r.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}

	if err := r.ReplaceSets(ctx, []Set{
		boxSet("veiculos", "a", "p1", "placa", "carro"),
		boxSet("capacete", "a", "p1", "com_capacete"), // a mesma imagem em outra tarefa
		boxSet("veiculos", "b", "p1", "placa"),
		boxSet("veiculos", "c", "p2"), // negativa
	}); err != nil {
		t.Fatal(err)
	}

	sets, err := r.FindImageSets(ctx, "a")
	if err != nil || len(sets) != 2 || sets[0].Task != "capacete" || sets[1].Task != "veiculos" {
		t.Fatalf("FindImageBoxSets = %+v, %v", sets, err)
	}
	if b := sets[1].Boxes[1]; b.Label != "carro" || !b.Occluded || b.Y != 1.5 || b.Attributes["caractere"] != "A" {
		t.Errorf("caixa = %+v", b)
	}

	// negativa gravada como [] (e não null)
	var raw bson.M
	if err := r.sets().FindOne(ctx, bson.D{{Key: "task", Value: "veiculos"}, {Key: "image", Value: "c"}}).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	if arr, ok := raw["boxes"].(bson.A); !ok || len(arr) != 0 {
		t.Errorf("boxes da negativa = %#v", raw["boxes"])
	}

	// substituir numa tarefa não mexe na outra
	if err := r.ReplaceSets(ctx, []Set{boxSet("veiculos", "a", "p1", "placa")}); err != nil {
		t.Fatal(err)
	}
	got, err := r.FindSets(ctx, "veiculos", []string{"a", "c", "x"})
	if err != nil || len(got) != 2 || len(got["a"].Boxes) != 1 || len(got["c"].Boxes) != 0 {
		t.Errorf("FindBoxSets = %+v, %v", got, err)
	}
	if got, _ := r.FindSets(ctx, "capacete", []string{"a"}); len(got["a"].Boxes) != 1 {
		t.Error("a outra tarefa não podia mudar")
	}

	counts, err := r.PackageCounts(ctx, "p1")
	if err != nil || len(counts) != 2 || counts[0].Task != "capacete" || counts[1].Images != 2 || counts[1].Boxes != 2 {
		t.Errorf("PackageBoxCounts = %+v, %v", counts, err)
	}

	if err := r.DeleteImageSets(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if sets, _ := r.FindImageSets(ctx, "a"); len(sets) != 0 {
		t.Error("apagar a imagem deveria apagar as caixas dela em todas as tarefas")
	}
}

func TestMongoMigracaoDasCaixasAntigas(t *testing.T) {
	r := testRepo(t)
	ctx := context.Background()
	// Como era antes: sem os índices novos, e as caixas com _id = sha1, sem
	// task/image.
	if _, err := r.sets().InsertMany(ctx, []any{
		bson.D{{Key: "_id", Value: "a"}, {Key: "package", Value: "p1"}, {Key: "boxes", Value: bson.A{bson.D{{Key: "label", Value: "placa"}}}}},
		bson.D{{Key: "_id", Value: "b"}, {Key: "package", Value: "p1"}, {Key: "boxes", Value: bson.A{bson.D{{Key: "label", Value: "carro"}}}}},
	}); err != nil {
		t.Fatal(err)
	}
	for range 2 { // idempotente
		if err := r.EnsureIndexes(ctx); err != nil {
			t.Fatal(err)
		}
		if err := r.EnsureIndexes(ctx); err != nil {
			t.Fatal(err)
		}
	}
	got, err := r.FindSets(ctx, DefaultTask, []string{"a", "b"})
	if err != nil || len(got) != 2 || got["a"].Boxes[0].Label != "placa" {
		t.Errorf("migradas = %+v, %v", got, err)
	}
	task, err := r.FindTask(ctx, DefaultTask)
	if err != nil || len(task.Labels) != 2 {
		t.Errorf("tarefa padrão = %+v, %v", task, err)
	}
}

func TestMongoPacoteRenomeadoEApagado(t *testing.T) {
	r := testRepo(t)
	ctx := context.Background()
	r.ReplaceSets(ctx, []Set{boxSet("t", "a", "velho", "placa"), boxSet("t", "z", "outro", "placa")})

	if err := r.RenamePackageSets(ctx, "velho", "novo"); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.FindImageSets(ctx, "a"); len(got) != 1 || got[0].Package != "novo" {
		t.Errorf("as caixas deveriam ir junto com o pacote: %+v", got)
	}

	if err := r.DeletePackageSets(ctx, "novo"); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.FindImageSets(ctx, "a"); len(got) != 0 {
		t.Error("apagar o pacote deveria apagar as caixas")
	}
	if got, _ := r.FindImageSets(ctx, "z"); len(got) != 1 {
		t.Error("as caixas de outro pacote não podem ser apagadas")
	}
}

func testRepo(t *testing.T) *MongoRepo {
	t.Helper()
	return NewMongoRepo(mongotest.DB(t))
}

func TestMongoTarefas(t *testing.T) {
	r := testRepo(t)
	ctx := context.Background()
	if err := r.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	task := &Task{Name: "capacete", Labels: []string{"com_capacete"}, CreatedBy: "nathan", CreatedAt: now, UpdatedAt: now}
	if err := r.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := r.CreateTask(ctx, task); !errors.Is(err, platform.ErrDuplicate) {
		t.Errorf("duplicada: %v", err)
	}
	if err := r.CreateTask(ctx, &Task{Name: "letras", Labels: []string{}}); err != nil {
		t.Fatal(err)
	}

	// labels novas no fim, sem repetir
	if err := r.AddTaskLabels(ctx, "capacete", []string{"sem_capacete", "com_capacete"}, now); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.FindTask(ctx, "capacete"); len(got.Labels) != 2 || got.Labels[1] != "sem_capacete" {
		t.Errorf("labels = %v", got.Labels)
	}

	r.ReplaceSets(ctx, []Set{
		boxSet("capacete", "a", "p1", "com_capacete", "com_capacete"),
		boxSet("capacete", "b", "p1"),
		boxSet("capacete", "c", "p2", "sem_capacete"),
		boxSet("letras", "a", "p1", "A"),
	})
	ti, err := r.TaskImages(ctx, "capacete")
	if err != nil {
		t.Fatal(err)
	}
	if len(ti.IDs["p1"]) != 2 || len(ti.IDs["p2"]) != 1 || ti.Negatives != 1 || ti.Labels["com_capacete"] != 2 || ti.Labels["sem_capacete"] != 1 {
		t.Errorf("TaskImages = %+v", ti)
	}

	// renomear: as caixas e a tarefa (o serviço decide a ordem)
	if err := r.RenameTaskSets(ctx, "capacete", "moto-capacete"); err != nil {
		t.Fatal(err)
	}
	task.Name = "letras"
	if err := r.ReplaceTask(ctx, "capacete", task); !errors.Is(err, platform.ErrDuplicate) {
		t.Errorf("para nome existente: %v", err)
	}
	task.Name = "moto-capacete"
	if err := r.ReplaceTask(ctx, "capacete", task); err != nil {
		t.Fatal(err)
	}
	if err := r.ReplaceTask(ctx, "sumiu", task); !errors.Is(err, platform.ErrNotFound) {
		t.Errorf("inexistente: %v", err)
	}
	if got, _ := r.FindSets(ctx, "moto-capacete", []string{"a", "b", "c"}); len(got) != 3 {
		t.Errorf("caixas renomeadas = %d", len(got))
	}
	if _, err := r.FindTask(ctx, "capacete"); !errors.Is(err, platform.ErrNotFound) {
		t.Error("o nome antigo deveria sumir")
	}
	if list, err := r.ListTasks(ctx); err != nil || len(list) != 2 || list[0].Name != "letras" {
		t.Errorf("list = %+v, %v", list, err)
	}

	n, err := r.DeleteTaskSets(ctx, "moto-capacete")
	if err != nil || n != 3 {
		t.Errorf("DeleteTaskSets = %d, %v", n, err)
	}
	if err := r.DeleteTask(ctx, "moto-capacete"); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.FindImageSets(ctx, "a"); len(got) != 1 || got[0].Task != "letras" {
		t.Errorf("as caixas da outra tarefa têm que ficar: %+v", got)
	}
	if err := r.DeleteTask(ctx, "moto-capacete"); !errors.Is(err, platform.ErrNotFound) {
		t.Errorf("delete de novo: %v", err)
	}
}
