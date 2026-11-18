package store

import (
	"context"
	"testing"
	"time"

	"SIDM/internal/platform/mongotest"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	uri := mongotest.URI(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	s, err := New(ctx, uri, mongotest.DBName())
	if err != nil {
		t.Skipf("Mongo indisponível (%v): pulando integração", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_ = s.db.Drop(bg)
		_ = s.Close(bg)
	})
	if err := mongotest.UniqueNames(ctx, s.db); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestNewURIInvalida(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if _, err := New(ctx, "http://nao-e-mongo", "x"); err == nil {
		t.Error("esquema inválido deveria falhar no Connect")
	}
	if _, err := New(ctx, "mongodb://localhost:1", "x"); err == nil {
		t.Error("porta sem Mongo deveria falhar no Ping")
	}
}

func TestPing(t *testing.T) {
	s := testStore(t)
	if err := s.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func ms(t time.Time) time.Time { return t.UTC().Truncate(time.Millisecond) }
