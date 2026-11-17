package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"SIDM/internal/export"
)

func TestPerfisDeExportNoMongo(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.ensureExportIndexes(ctx); err != nil {
		t.Fatal(err)
	}

	p := &export.Profile{
		Name: "yolo-noite", Format: export.FormatYOLO, Split: export.Split{Train: 80, Valid: 20},
		Packages:  []string{"p1"},
		Filters:   export.Filters{DayTime: "night"},
		Labels:    export.LabelRules{Rename: map[string]string{"Placa": "placa"}, Include: []string{"placa", "carro"}},
		CreatedBy: "nathan", UpdatedAt: ms(time.Now()),
	}
	if err := s.CreateExportProfile(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateExportProfile(ctx, p); !errors.Is(err, ErrDuplicate) {
		t.Errorf("duplicado: %v", err)
	}

	got, err := s.FindExportProfile(ctx, "yolo-noite")
	if err != nil {
		t.Fatal(err)
	}
	if got.Split.Valid != 20 || got.Filters.DayTime != "night" || got.Labels.Rename["Placa"] != "placa" ||
		len(got.Labels.Include) != 2 || got.Labels.Include[1] != "carro" || got.Builtin {
		t.Errorf("perfil = %+v", got)
	}

	got.Split = export.Split{Train: 70, Valid: 30}
	if err := s.ReplaceExportProfile(ctx, got); err != nil {
		t.Fatal(err)
	}
	if again, _ := s.FindExportProfile(ctx, "yolo-noite"); again.Split.Train != 70 {
		t.Error("o replace não gravou")
	}
	if err := s.ReplaceExportProfile(ctx, &export.Profile{Name: "nao-existe"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("replace inexistente: %v", err)
	}

	list, err := s.ListExportProfiles(ctx)
	if err != nil || len(list) != 1 {
		t.Errorf("list = %v, %v", list, err)
	}
	if err := s.DeleteExportProfile(ctx, "yolo-noite"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteExportProfile(ctx, "yolo-noite"); !errors.Is(err, ErrNotFound) {
		t.Errorf("apagar duas vezes: %v", err)
	}
}
