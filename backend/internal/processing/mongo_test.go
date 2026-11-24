package processing

// Testes de INTEGRAÇÃO do MongoRepo (ver platform/mongotest): sem Mongo, são pulados.

import (
	"context"
	"errors"
	"testing"
	"time"

	"SIDM/internal/platform"
	"SIDM/internal/platform/mongotest"
)

func TestMongoRuns(t *testing.T) {
	ctx := context.Background()
	r := NewMongoRepo(mongotest.DB(t))
	if err := r.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	t0 := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	old := Run{ID: "r1", Model: "lpr", Packages: []string{"p1"}, Scope: ScopePending, Status: StatusDone, StartedAt: t0}
	cur := Run{ID: "r2", Model: "lpr", Packages: []string{"p1", "p2"}, Scope: ScopeAll, Status: StatusRunning, StartedAt: t0.Add(time.Hour),
		Errors: []RunError{{SHA1: "a", Msg: "x"}}}
	for _, run := range []Run{old, cur} {
		if err := r.CreateRun(ctx, &run); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.CreateRun(ctx, &old); !errors.Is(err, platform.ErrDuplicate) {
		t.Errorf("id repetido: %v", err)
	}
	cur.Done, cur.Boxes = 3, 9
	if err := r.SaveRun(ctx, &cur); err != nil {
		t.Fatal(err)
	}
	if err := r.SaveRun(ctx, &Run{ID: "nao"}); !errors.Is(err, platform.ErrNotFound) {
		t.Errorf("gravar o que não existe: %v", err)
	}
	got, err := r.FindRun(ctx, "r2")
	if err != nil || got.Done != 3 || got.Boxes != 9 || len(got.Packages) != 2 || len(got.Errors) != 1 {
		t.Fatalf("lido: %+v %v", got, err)
	}
	runs, err := r.ListRuns(ctx, 10)
	if err != nil || len(runs) != 2 || runs[0].ID != "r2" {
		t.Fatalf("lista (mais novo primeiro): %+v %v", runs, err)
	}
	if n, err := r.InterruptRunning(ctx, t0.Add(2*time.Hour)); err != nil || n != 1 {
		t.Fatalf("interromper: %d %v", n, err)
	}
	got, _ = r.FindRun(ctx, "r2")
	if got.Status != StatusInterrupted || got.FinishedAt == nil {
		t.Errorf("interrompido: %+v", got)
	}
	if _, err := r.FindRun(ctx, "nao"); !errors.Is(err, platform.ErrNotFound) {
		t.Errorf("não existe: %v", err)
	}
}
