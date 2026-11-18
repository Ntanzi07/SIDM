package imports

import (
	"context"
	"errors"
	"testing"
	"time"

	"SIDM/internal/importer"
	"SIDM/internal/platform"
	"SIDM/internal/platform/mongotest"
)

func TestPerfisDeImport(t *testing.T) {
	s := NewMongoRepo(mongotest.DB(t))
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
	if err := s.CreateProfile(ctx, p); !errors.Is(err, platform.ErrDuplicate) {
		t.Errorf("duplicado: err = %v, want platform.ErrDuplicate", err)
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
	if _, err := s.FindProfile(ctx, "nao-existe"); !errors.Is(err, platform.ErrNotFound) {
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
	if err := s.ReplaceProfile(ctx, &ghost); !errors.Is(err, platform.ErrNotFound) {
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
	s := NewMongoRepo(mongotest.DB(t))
	ctx := context.Background()

	for _, name := range []string{"perfil-a", "perfil-b"} {
		p := &importer.Profile{Name: name, CreatedBy: "nathan", UpdatedAt: ms(time.Now())}
		if err := s.CreateProfile(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.RenameProfile(ctx, "perfil-a", "perfil-b", time.Now()); !errors.Is(err, platform.ErrDuplicate) {
		t.Errorf("para nome existente: err = %v, want platform.ErrDuplicate", err)
	}
	if err := s.RenameProfile(ctx, "nao-existe", "x", time.Now()); !errors.Is(err, platform.ErrNotFound) {
		t.Errorf("inexistente: err = %v, want platform.ErrNotFound", err)
	}

	now := ms(time.Now().Add(time.Minute))
	if err := s.RenameProfile(ctx, "perfil-a", "perfil-novo", now); err != nil {
		t.Fatal(err)
	}
	p, err := s.FindProfile(ctx, "perfil-novo")
	if err != nil || p.Name != "perfil-novo" || p.CreatedBy != "nathan" || !p.UpdatedAt.Equal(now) {
		t.Errorf("FindProfile = %+v, %v", p, err)
	}
	if _, err := s.FindProfile(ctx, "perfil-a"); !errors.Is(err, platform.ErrNotFound) {
		t.Error("o nome antigo deveria ter sumido")
	}

	if err := s.DeleteProfile(ctx, "perfil-novo"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FindProfile(ctx, "perfil-novo"); !errors.Is(err, platform.ErrNotFound) {
		t.Error("deveria ter sido apagado")
	}
	if err := s.DeleteProfile(ctx, "perfil-novo"); !errors.Is(err, platform.ErrNotFound) {
		t.Errorf("apagar duas vezes: err = %v", err)
	}
	if _, err := s.FindProfile(ctx, "perfil-b"); err != nil {
		t.Error("o outro perfil não pode ser afetado")
	}
}

// ms: o Mongo guarda datas com precisão de MILISSEGUNDOS. Um time.Now() com
// nanossegundos volta diferente do banco; nos testes, truncamos antes.
func ms(t time.Time) time.Time { return t.UTC().Truncate(time.Millisecond) }
