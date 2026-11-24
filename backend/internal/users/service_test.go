package users

import (
	"context"
	"errors"
	"net/http/httptest"
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
	if _, err := s.EnsureAdmin(ctx, "admin", "", "senha-admin"); err != nil {
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
	if _, err := s.Register(ctx, RegisterInput{Email: "nathan@empresa.com", Name: "Nathan", Password: "senha-antiga"}); err != nil {
		t.Fatal(err)
	}
	if created, err := s.EnsureAdmin(ctx, "Nathan", "", "senha-do-env"); err != nil || !created {
		t.Fatalf("%v %v", created, err)
	}
	if u := mem.Users["nathan"]; !u.IsAdmin() || !checkPassword(u.PasswordHash, "senha-do-env") || u.Email != "nathan@empresa.com" {
		t.Errorf("promovido (sem perder o e-mail): %+v", u)
	}
	// com admin ativo, o .env não mexe (nem na senha trocada)
	if created, _ := s.EnsureAdmin(ctx, "nathan", "", "outra-do-env"); created {
		t.Error("não podia mexer")
	}
	// o admin de antes do e-mail: o .env completa o e-mail, sem mexer na senha
	s3, mem3, _ := newTestService(t)
	if _, err := s3.EnsureAdmin(ctx, "chefe", "", "senha-do-chefe"); err != nil {
		t.Fatal(err)
	}
	if filled, err := s3.EnsureAdmin(ctx, "chefe", "Chefe@Empresa.com", "outra-senha"); err != nil || !filled {
		t.Fatalf("completar o e-mail: %v %v", filled, err)
	}
	if u := mem3.Users["chefe"]; u.Email != "chefe@empresa.com" || !checkPassword(u.PasswordHash, "senha-do-chefe") {
		t.Errorf("chefe: %+v", u)
	}
	if filled, _ := s3.EnsureAdmin(ctx, "chefe", "novo@empresa.com", "x"); filled {
		t.Error("com e-mail, o .env não troca")
	}
	// senha curta no .env: erro claro
	s2, _, _ := newTestService(t)
	if _, err := s2.EnsureAdmin(ctx, "admin", "", "curta"); err == nil {
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
		// usuário normal: anota, importa, apaga imagem, exporta, cria versão
		{"POST", "/packages/x/next", false},
		{"PUT", "/images/abc", false},
		{"PUT", "/images/abc/boxes/capacete", false},
		{"POST", "/images/abc/release", false},
		{"POST", "/images/abc/reopen", false},
		{"POST", "/equipments", false},
		{"PATCH", "/cameras/x", false},
		{"POST", "/annotation-profiles/veiculos/check", false},
		{"POST", "/packages", false},
		{"POST", "/images", false},
		{"POST", "/packages/x/import", false},
		{"POST", "/tasks/capacete/import", false},
		{"DELETE", "/images/abc", false},
		{"POST", "/exports/download", false},
		{"POST", "/datasets", false},
		{"POST", "/datasets/motos/versions", false},
		{"POST", "/datasets/motos/versions/3/items/abc/restore", false},
		{"POST", "/packages/images/next", false}, // pacote com nome de rota
		{"GET", "/tasks", false},
		// só admin
		{"POST", "/datasets/motos/versions/3/restore", true}, // voltar tudo
		{"PUT", "/tasks/capacete", true},                     // nome, descrição, labels, cores
		{"POST", "/tasks", true},
		{"DELETE", "/tasks/capacete", true},
		{"PATCH", "/packages/x", true},
		{"DELETE", "/packages/x", true},
		{"DELETE", "/datasets/x", true},
		{"PATCH", "/datasets/x", true},
		{"POST", "/annotation-profiles", true},
		{"PUT", "/annotation-profiles/veiculos", true},
		{"PATCH", "/import-profiles/x", true},
		{"POST", "/export-profiles", true},
		{"DELETE", "/equipments/x", true},
		{"GET", "/users", true},
		{"PATCH", "/users/maria", true},
	}
	for _, c := range cases {
		r := httptest.NewRequest(c.method, c.path, nil)
		if got := adminOnly(r); got != c.admin {
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

func TestEmail(t *testing.T) {
	s, _, _ := newTestService(t)
	ctx := context.Background()
	cases := map[string]string{
		"Maria.Silva@Empresa.com.br": "maria.silva",
		"joao+sidm@x.io":             "joao.sidm",
		"a@x.io":                     "usuario", // curto demais: vira "usuario"
	}
	for email, want := range cases {
		u, err := s.Register(ctx, RegisterInput{Email: email, Name: "X", Password: "senha-boa-1"})
		if err != nil || u.Username != want || u.Email != strings.ToLower(email) {
			t.Errorf("%s: %+v %v", email, u, err)
		}
	}
	for _, bad := range []string{"", "sem-arroba", "a@b", "a b@x.com", "@x.com"} {
		if _, err := s.Register(ctx, RegisterInput{Email: bad, Name: "X", Password: "senha-boa-1"}); err == nil {
			t.Errorf("%q devia ser inválido", bad)
		}
	}
	// entra pelo e-mail (qualquer maiúscula) ou pelo nome de usuário
	active := StatusActive
	if _, err := s.Update(ctx, "admin", "maria.silva", UpdateInput{Status: &active}); err != nil {
		t.Fatal(err)
	}
	for _, login := range []string{"maria.silva@empresa.com.br", " MARIA.SILVA@empresa.com.br", "maria.silva"} {
		if _, u, err := s.Login(ctx, login, "senha-boa-1"); err != nil || u.Username != "maria.silva" {
			t.Errorf("login %q: %v", login, err)
		}
	}
	var le *LoginError
	if _, _, err := s.Login(ctx, "ninguem@empresa.com.br", "senha-boa-1"); !errors.As(err, &le) {
		t.Errorf("e-mail que não existe: %v", err)
	}
}
