package store

import (
	"context"
	"testing"

	"SIDM/internal/export"
)

func TestTaskRenamed(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	for _, p := range []export.Profile{
		{Name: "a", Format: export.FormatYOLO, Task: "capacete"},
		{Name: "b", Format: export.FormatYOLO, Task: "outra"},
	} {
		if err := s.CreateExportProfile(ctx, &p); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.TaskRenamed(ctx, "capacete", "moto-capacete"); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.FindExportProfile(ctx, "a"); p.Task != "moto-capacete" {
		t.Errorf("perfil a aponta para %q", p.Task)
	}
	if p, _ := s.FindExportProfile(ctx, "b"); p.Task != "outra" {
		t.Errorf("o perfil de outra tarefa mudou: %q", p.Task)
	}
}
