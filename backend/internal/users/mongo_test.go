package users

// Testes de INTEGRAÇÃO do MongoRepo (ver platform/mongotest): sem Mongo, são pulados.

import (
	"context"
	"errors"
	"testing"
	"time"

	"SIDM/internal/platform"
	"SIDM/internal/platform/mongotest"
)

// O caminho pelo serviço gravando no Mongo: nome único, aprovação, sessão
// que volta do banco (com o fuso certo) e cai na troca de senha.
func TestMongoUsuarios(t *testing.T) {
	ctx := context.Background()
	r := NewMongoRepo(mongotest.DB(t))
	if err := r.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	if err := r.EnsureIndexes(ctx); err != nil { // idempotente
		t.Fatal(err)
	}
	s, _, _ := newTestService(t)
	s.repo = r
	s.now = func() time.Time { return time.Now().UTC() } // o TTL do Mongo usa o relógio de verdade

	if _, err := s.EnsureAdmin(ctx, "admin", "senha-admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Register(ctx, RegisterInput{Username: "maria", Name: "Maria", Password: "senha-da-maria"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Register(ctx, RegisterInput{Username: "maria", Name: "Outra", Password: "senha-da-maria"}); !errors.Is(err, platform.ErrDuplicate) {
		t.Errorf("duplicado: %v", err)
	}
	if n, _ := r.CountActiveAdmins(ctx); n != 1 {
		t.Errorf("admins: %d", n)
	}
	active := StatusActive
	if _, err := s.Update(ctx, "admin", "maria", UpdateInput{Status: &active}); err != nil {
		t.Fatal(err)
	}
	token, _, err := s.Login(ctx, "maria", "senha-da-maria")
	if err != nil {
		t.Fatal(err)
	}
	if u, err := s.Authenticate(ctx, token); err != nil || u == nil || u.Username != "maria" || u.LastLogin == nil {
		t.Fatalf("sessão pelo banco: %+v %v", u, err)
	}
	if list, _ := s.List(ctx); len(list) != 2 || list[0].Username != "admin" {
		t.Errorf("lista: %+v", list)
	}
	if err := s.ChangePassword(ctx, "maria", "senha-da-maria", "nova-senha-1"); err != nil {
		t.Fatal(err)
	}
	if u, _ := s.Authenticate(ctx, token); u != nil {
		t.Error("trocar a senha fecha as sessões")
	}
	if _, err := r.FindUser(ctx, "ninguem"); !errors.Is(err, platform.ErrNotFound) {
		t.Errorf("não existe: %v", err)
	}
}
