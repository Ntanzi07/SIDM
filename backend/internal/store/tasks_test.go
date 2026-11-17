package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"SIDM/internal/export"
)

func TestTarefasNoMongo(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	for _, ensure := range []func(context.Context) error{s.ensureTaskIndexes, s.ensureBoxIndexes, s.ensureExportIndexes} {
		if err := ensure(ctx); err != nil {
			t.Fatal(err)
		}
	}
	now := ms(time.Now())
	task := &Task{Name: "capacete", Labels: []string{"com_capacete"}, CreatedBy: "nathan", CreatedAt: now, UpdatedAt: now}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateTask(ctx, task); !errors.Is(err, ErrDuplicate) {
		t.Errorf("duplicada: %v", err)
	}
	if err := s.CreateTask(ctx, &Task{Name: "letras", Labels: []string{}}); err != nil {
		t.Fatal(err)
	}

	// labels novas no fim, sem repetir
	if err := s.AddTaskLabels(ctx, "capacete", []string{"sem_capacete", "com_capacete"}, now); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.FindTask(ctx, "capacete"); len(got.Labels) != 2 || got.Labels[1] != "sem_capacete" {
		t.Errorf("labels = %v", got.Labels)
	}

	// imagens: a verificada, b pendente; c sem imagem no banco
	for i, st := range []string{StatusVerified, StatusPending} {
		img := newImage(string(rune('a'+i)), time.Now())
		img.Package, img.Status = "p1", st
		if err := s.InsertImage(ctx, img); err != nil {
			t.Fatal(err)
		}
	}
	s.ReplaceBoxSets(ctx, []BoxSet{
		boxSet("capacete", "a", "p1", "com_capacete", "com_capacete"),
		boxSet("capacete", "b", "p1"),
		boxSet("capacete", "c", "p2", "sem_capacete"),
		boxSet("letras", "a", "p1", "A"),
	})
	st, err := s.TaskStats(ctx, "capacete")
	if err != nil {
		t.Fatal(err)
	}
	if st.Images != 3 || st.Verified != 1 || st.Negatives != 1 || st.Boxes != 3 || st.Labels["com_capacete"] != 2 ||
		len(st.Packages) != 2 || st.Packages[0].Name != "p1" || st.Packages[0].Images != 2 || st.Packages[0].Verified != 1 {
		t.Errorf("stats = %+v", st)
	}

	// renomear leva caixas e perfis junto
	if err := s.CreateExportProfile(ctx, &export.Profile{Name: "p", Format: export.FormatYOLO, Task: "capacete"}); err != nil {
		t.Fatal(err)
	}
	task.Name = "letras"
	if err := s.UpdateTask(ctx, "capacete", task); !errors.Is(err, ErrDuplicate) {
		t.Errorf("rename para nome existente: %v", err)
	}
	task.Name = "moto-capacete"
	if err := s.UpdateTask(ctx, "capacete", task); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.FindBoxSets(ctx, "moto-capacete", []string{"a", "b", "c"}); len(got) != 3 {
		t.Errorf("caixas renomeadas = %d", len(got))
	}
	if p, _ := s.FindExportProfile(ctx, "p"); p.Task != "moto-capacete" {
		t.Errorf("perfil aponta para %q", p.Task)
	}
	if _, err := s.FindTask(ctx, "capacete"); !errors.Is(err, ErrNotFound) {
		t.Error("o nome antigo deveria sumir")
	}

	list, err := s.ListTasks(ctx)
	if err != nil || len(list) != 2 || list[0].Name != "letras" {
		t.Errorf("list = %+v, %v", list, err)
	}

	n, err := s.DeleteTask(ctx, "moto-capacete")
	if err != nil || n != 3 {
		t.Errorf("delete = %d, %v", n, err)
	}
	if got, _ := s.FindImageBoxSets(ctx, "a"); len(got) != 1 || got[0].Task != "letras" {
		t.Errorf("as caixas da outra tarefa têm que ficar: %+v", got)
	}
	if _, err := s.DeleteTask(ctx, "moto-capacete"); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete de novo: %v", err)
	}
}
