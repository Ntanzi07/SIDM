package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"SIDM/internal/importer"
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

func TestPerfisDeImport(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	p := &importer.Profile{
		Name:   "csv-8003",
		Reader: importer.ReaderConfig{Type: "csv", Delimiter: ";"},
		Match:  importer.MatchConfig{Column: "Arquivo", By: importer.MatchByOriginalName},
		Fields: map[string]importer.FieldRule{
			// Chaves com "." e "$": o motivo de guardar o perfil como texto.
			"weather": {Column: "Clima", Map: map[string]string{"Chuva.forte": "rain", "$limpo": "clear"}},
		},
		CreatedBy: "nathan",
		UpdatedAt: ms(time.Now()),
	}
	if err := s.CreateProfile(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateProfile(ctx, p); !errors.Is(err, ErrDuplicate) {
		t.Errorf("duplicado: err = %v, want ErrDuplicate", err)
	}

	got, err := s.FindProfile(ctx, "csv-8003")
	if err != nil {
		t.Fatal(err)
	}
	if got.Fields["weather"].Map["Chuva.forte"] != "rain" || got.Fields["weather"].Map["$limpo"] != "clear" {
		t.Errorf("chaves com . e $ não sobreviveram: %+v", got.Fields["weather"].Map)
	}
	if got.CreatedBy != "nathan" || !got.UpdatedAt.Equal(p.UpdatedAt) {
		t.Errorf("metadados: %+v", got)
	}
	if _, err := s.FindProfile(ctx, "nao-existe"); !errors.Is(err, ErrNotFound) {
		t.Errorf("inexistente: err = %v", err)
	}

	// Atualizar mantém o created_by.
	got.Fields["weather"].Map["Garoa"] = "rain"
	got.CreatedBy = "outra-pessoa"
	if err := s.ReplaceProfile(ctx, got); err != nil {
		t.Fatal(err)
	}
	again, _ := s.FindProfile(ctx, "csv-8003")
	if again.Fields["weather"].Map["Garoa"] != "rain" || again.CreatedBy != "nathan" {
		t.Errorf("replace: map=%v created_by=%q", again.Fields["weather"].Map, again.CreatedBy)
	}
	ghost := *p
	ghost.Name = "nao-existe"
	if err := s.ReplaceProfile(ctx, &ghost); !errors.Is(err, ErrNotFound) {
		t.Errorf("replace inexistente: err = %v", err)
	}

	s.CreateProfile(ctx, &importer.Profile{Name: "outro-csv", Reader: p.Reader, Match: p.Match})
	list, total, err := s.ListProfiles(ctx, "8003", 10, 0)
	if err != nil || total != 1 || list[0].Name != "csv-8003" {
		t.Errorf("busca: total=%d %v %v", total, list, err)
	}
	list, total, _ = s.ListProfiles(ctx, "", 1, 1)
	if total != 2 || len(list) != 1 || list[0].Name != "outro-csv" {
		t.Errorf("paginação: total=%d %v", total, list)
	}
}

func TestRenomearEApagarPerfil(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	for _, name := range []string{"perfil-a", "perfil-b"} {
		p := &importer.Profile{Name: name, CreatedBy: "nathan", UpdatedAt: ms(time.Now())}
		if err := s.CreateProfile(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.RenameProfile(ctx, "perfil-a", "perfil-b", time.Now()); !errors.Is(err, ErrDuplicate) {
		t.Errorf("para nome existente: err = %v, want ErrDuplicate", err)
	}
	if err := s.RenameProfile(ctx, "nao-existe", "x", time.Now()); !errors.Is(err, ErrNotFound) {
		t.Errorf("inexistente: err = %v, want ErrNotFound", err)
	}

	now := ms(time.Now().Add(time.Minute))
	if err := s.RenameProfile(ctx, "perfil-a", "perfil-novo", now); err != nil {
		t.Fatal(err)
	}
	p, err := s.FindProfile(ctx, "perfil-novo")
	if err != nil || p.Name != "perfil-novo" || p.CreatedBy != "nathan" || !p.UpdatedAt.Equal(now) {
		t.Errorf("FindProfile = %+v, %v", p, err)
	}
	if _, err := s.FindProfile(ctx, "perfil-a"); !errors.Is(err, ErrNotFound) {
		t.Error("o nome antigo deveria ter sumido")
	}

	if err := s.DeleteProfile(ctx, "perfil-novo"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FindProfile(ctx, "perfil-novo"); !errors.Is(err, ErrNotFound) {
		t.Error("deveria ter sido apagado")
	}
	if err := s.DeleteProfile(ctx, "perfil-novo"); !errors.Is(err, ErrNotFound) {
		t.Errorf("apagar duas vezes: err = %v", err)
	}
	if _, err := s.FindProfile(ctx, "perfil-b"); err != nil {
		t.Error("o outro perfil não pode ser afetado")
	}
}

// ms: o Mongo guarda datas com precisão de MILISSEGUNDOS. Um time.Now() com
// nanossegundos volta diferente do banco; nos testes, truncamos antes.
func ms(t time.Time) time.Time { return t.UTC().Truncate(time.Millisecond) }
