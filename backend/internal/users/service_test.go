package users

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func newTestService(t *testing.T) (*Service, *Memory, *time.Time) {
	t.Helper()
	HashIterations = 1000
	mem := NewMemory()
	s := New(mem)
	now := time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	s.limiter.now = s.now
	return s, mem, &now
}

func TestSenha(t *testing.T) {
	HashIterations = 1000
	h, err := hashPassword("uma senha boa")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "pbkdf2-sha256$1000$") || strings.Contains(h, "senha") {
		t.Errorf("hash: %s", h)
	}
	h2, _ := hashPassword("uma senha boa")
	if h == h2 {
		t.Error("o sal tem que mudar a cada hash")
	}
	if !checkPassword(h, "uma senha boa") || checkPassword(h, "uma senha boA") || checkPassword("lixo", "x") {
		t.Error("checkPassword")
	}
	// mudar as iterações não invalida as senhas antigas
	HashIterations = 2000
	if !checkPassword(h, "uma senha boa") {
		t.Error("a senha antiga tem que continuar valendo")
	}
}

func TestSessaoExpira(t *testing.T) {
	s, mem, now := newTestService(t)
	ctx := context.Background()
	if _, err := s.EnsureAdmin(ctx, "admin", "senha-admin"); err != nil {
		t.Fatal(err)
	}
	token, _, err := s.Login(ctx, "admin", "senha-admin")
	if err != nil {
		t.Fatal(err)
	}
	for id := range mem.Sessions {
		if id == token || !strings.EqualFold(id, tokenID(token)) {
			t.Error("no banco fica só o hash do token")
		}
	}
	if u, _ := s.Authenticate(ctx, token); u == nil || u.Username != "admin" {
		t.Fatalf("logado: %+v", u)
	}
	*now = now.Add(SessionTTL - time.Minute)
	if u, _ := s.Authenticate(ctx, token); u == nil {
		t.Error("antes das 8 horas ainda vale")
	}
	*now = now.Add(2 * time.Minute)
	if u, _ := s.Authenticate(ctx, token); u != nil {
		t.Error("depois das 8 horas tem que entrar de novo")
	}
	if u, _ := s.Authenticate(ctx, "token-inventado"); u != nil {
		t.Error("token inventado")
	}
}

func TestAdminDoEnv(t *testing.T) {
	s, mem, _ := newTestService(t)
	ctx := context.Background()
	// já cadastrado (pendente): vira admin ativo com a senha do .env
	if _, err := s.Register(ctx, RegisterInput{Username: "nathan", Name: "Nathan", Password: "senha-antiga"}); err != nil {
		t.Fatal(err)
	}
	if created, err := s.EnsureAdmin(ctx, "Nathan", "senha-do-env"); err != nil || !created {
		t.Fatalf("%v %v", created, err)
	}
	if u := mem.Users["nathan"]; !u.IsAdmin() || !checkPassword(u.PasswordHash, "senha-do-env") {
		t.Errorf("promovido: %+v", u)
	}
	// com admin ativo, o .env não mexe (nem na senha trocada)
	if created, _ := s.EnsureAdmin(ctx, "nathan", "outra-do-env"); created {
		t.Error("não podia mexer")
	}
	// senha curta no .env: erro claro
	s2, _, _ := newTestService(t)
	if _, err := s2.EnsureAdmin(ctx, "admin", "curta"); err == nil {
		t.Error("senha curta no .env")
	}
}

func TestTentativas(t *testing.T) {
	s, _, now := newTestService(t)
	for i := 0; i < maxFailures; i++ {
		if !s.limiter.allow("maria") {
			t.Fatalf("bloqueou cedo (%d)", i)
		}
		s.limiter.fail("maria")
	}
	if s.limiter.allow("maria") || !s.limiter.allow("joao") {
		t.Error("bloqueia só quem errou")
	}
	*now = now.Add(lockout)
	if !s.limiter.allow("maria") {
		t.Error("depois do tempo, libera")
	}
}

func TestRegras(t *testing.T) {
	cases := []struct {
		method, path string
		admin        bool
	}{
		{"DELETE", "/packages/x", true},
		{"DELETE", "/images/abc", true},
		{"GET", "/users", true},
		{"PATCH", "/users/maria", true},
		{"POST", "/annotation-profiles", true},
		{"PUT", "/annotation-profiles/veiculos", true},
		{"POST", "/annotation-profiles/veiculos/check", false},
		{"PATCH", "/import-profiles/x", true},
		{"GET", "/import-profiles", false},
		{"POST", "/equipments", false}, // cadastrar na anotação
		{"PATCH", "/cameras/x", false}, // reativar na anotação
		{"POST", "/datasets", false},
		{"POST", "/packages/x/next", false},
		{"POST", "/images/abc/release", false},
	}
	for _, c := range cases {
		if got := adminOnly(c.method, c.path); got != c.admin {
			t.Errorf("%s %s: só admin = %v, esperava %v", c.method, c.path, got, c.admin)
		}
	}
	if !public("POST", "/auth/login") || public("GET", "/packages") {
		t.Error("public")
	}
	var invalid *InvalidError
	if err := checkUsername("Maria"); !errors.As(err, &invalid) {
		t.Error("maiúscula chega aqui já limpa; crua é inválida")
	}
}
