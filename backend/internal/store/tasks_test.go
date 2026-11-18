package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"SIDM/internal/export"
)

// O lado do store do módulo boxes: quais imagens estão verificadas, e os
// perfis de export quando uma tarefa muda de nome.

func TestVerifiedIDs(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	for i, st := range []string{StatusVerified, StatusPending, StatusVerified} {
		img := newImage(fmt.Sprintf("img-%d", i), time.Now())
		img.Status = st
		if err := s.InsertImage(ctx, img); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.VerifiedIDs(ctx, []string{"img-0", "img-1", "img-2", "nao-existe"})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(got) != "map[img-0:true img-2:true]" {
		t.Errorf("VerifiedIDs = %v", got)
	}
}

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
