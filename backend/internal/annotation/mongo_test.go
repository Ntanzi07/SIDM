package annotation

import (
	"context"
	"errors"
	"testing"

	"SIDM/internal/platform"
	"SIDM/internal/platform/mongotest"
)

func TestPerfisNoMongo(t *testing.T) {
	m := NewMongoRepo(mongotest.DB(t))
	ctx := context.Background()
	if err := m.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}

	p := capacete()
	p.Normalize()
	if err := m.CreateProfile(ctx, &p); err != nil {
		t.Fatal(err)
	}
	if err := m.CreateProfile(ctx, &p); !errors.Is(err, platform.ErrDuplicate) {
		t.Errorf("duplicado: %v", err)
	}

	got, err := m.FindProfile(ctx, p.Name)
	if err != nil {
		t.Fatal(err)
	}
	// O que volta do banco tem que passar na mesma validação e ser igual.
	if err := got.Validate(); err != nil {
		t.Fatalf("o perfil voltou do banco inválido: %v", err)
	}
	if len(got.Fields) != len(p.Fields) || *got.Fields[1].Max != 3 || got.Objects[1].Fields[0].Pattern != "[A-Z0-9]{7}" || !got.Objects[0].Box {
		t.Errorf("voltou diferente: %+v", got)
	}

	got.Description = "nova"
	if err := m.ReplaceProfile(ctx, got); err != nil {
		t.Fatal(err)
	}
	if list, err := m.ListProfiles(ctx); err != nil || len(list) != 1 || list[0].Description != "nova" {
		t.Errorf("lista = %+v, %v", list, err)
	}
	if err := m.ReplaceProfile(ctx, &Profile{Name: "nao-existe"}); !errors.Is(err, platform.ErrNotFound) {
		t.Errorf("replace inexistente: %v", err)
	}
	if err := m.DeleteProfile(ctx, p.Name); err != nil {
		t.Fatal(err)
	}
	if err := m.DeleteProfile(ctx, p.Name); !errors.Is(err, platform.ErrNotFound) {
		t.Errorf("apagar duas vezes: %v", err)
	}
}
