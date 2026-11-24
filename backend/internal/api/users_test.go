package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"SIDM/internal/users"
	"SIDM/internal/versions"
)

// client: um navegador com o seu cookie de sessão, falando com o servidor
// COM o login (users.Protect), como no main.go.
type client struct {
	t       *testing.T
	handler http.Handler
	cookie  *http.Cookie
}

func (c *client) do(method, path, body string, headers ...string) *httptest.ResponseRecorder {
	c.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, req)
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == users.CookieName {
			if ck.MaxAge < 0 {
				c.cookie = nil
			} else {
				c.cookie = ck
			}
		}
	}
	return rec
}

func (c *client) login(user, pass string) *httptest.ResponseRecorder {
	c.t.Helper()
	return c.do("POST", "/auth/login", `{"username": "`+user+`", "password": "`+pass+`"}`)
}

func TestLoginEPermissoes(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	if created, err := env.usrSvc.EnsureAdmin(ctx, "admin", "senha-do-admin"); err != nil || !created {
		t.Fatalf("admin: %v %v", created, err)
	}
	if created, _ := env.usrSvc.EnsureAdmin(ctx, "outro", "outra-senha-1"); created {
		t.Error("com um admin ativo, o .env não cria outro")
	}
	handler := env.usrSvc.Protect(env.srv.Routes())
	anon := &client{t: t, handler: handler}
	admin := &client{t: t, handler: handler}
	maria := &client{t: t, handler: handler}

	// sem login: só o público
	assertStatus(t, anon.do("GET", "/health", ""), http.StatusOK)
	assertStatus(t, anon.do("GET", "/packages", ""), http.StatusUnauthorized)
	assertStatus(t, anon.do("GET", "/packages", "", "X-User", "admin"), http.StatusUnauthorized) // o header não vale
	assertStatus(t, anon.do("GET", "/auth/me", ""), http.StatusUnauthorized)

	// cadastro: fica pendente
	assertStatus(t, anon.do("POST", "/auth/register", `{"username": "Maria ", "name": "Maria Silva", "password": "senha-da-maria"}`), http.StatusCreated)
	assertStatus(t, anon.do("POST", "/auth/register", `{"username": "maria", "name": "Outra", "password": "senha-da-maria"}`), http.StatusConflict)
	assertStatus(t, anon.do("POST", "/auth/register", `{"username": "x", "name": "X", "password": "senha-longa"}`), http.StatusBadRequest)
	assertStatus(t, anon.do("POST", "/auth/register", `{"username": "joao", "name": "João", "password": "curta"}`), http.StatusBadRequest)
	if rec := maria.login("maria", "senha-da-maria"); rec.Code != http.StatusForbidden || !strings.Contains(errorMsg(t, rec), "aprovada") {
		t.Errorf("pendente entrando: %d %s", rec.Code, rec.Body.String())
	}
	if env.usr.Users["maria"].PasswordHash == "" || strings.Contains(env.usr.Users["maria"].PasswordHash, "senha-da-maria") {
		t.Error("a senha tem que ser guardada só como hash")
	}

	// admin entra e aprova
	assertStatus(t, admin.login("admin", "errada-errada"), http.StatusUnauthorized)
	assertStatus(t, admin.login("ADMIN", "senha-do-admin"), http.StatusOK)
	if admin.cookie == nil || !admin.cookie.HttpOnly || admin.cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie: %+v", admin.cookie)
	}
	var list struct{ Items []users.User }
	rec := admin.do("GET", "/users", "")
	assertStatus(t, rec, http.StatusOK)
	decode(t, rec, &list)
	if len(list.Items) != 2 || strings.Contains(rec.Body.String(), "password") {
		t.Errorf("lista: %s", rec.Body.String())
	}
	rec = admin.do("PATCH", "/users/maria", `{"status": "active"}`)
	assertStatus(t, rec, http.StatusOK)
	var u users.User
	decode(t, rec, &u)
	if u.ApprovedBy != "admin" || u.Status != users.StatusActive {
		t.Errorf("aprovada: %+v", u)
	}

	// maria entra: é ela que fica no histórico, mesmo mandando outro X-User
	assertStatus(t, maria.login("maria", "senha-da-maria"), http.StatusOK)
	rec = maria.do("GET", "/auth/me", "")
	decode(t, rec, &u)
	if u.Username != "maria" || u.Role != users.RoleUser {
		t.Errorf("me: %+v", u)
	}
	assertStatus(t, maria.do("POST", "/datasets", `{"name": "teste", "profile": "veiculos"}`, "X-User", "admin"), http.StatusCreated)
	var d versions.Dataset
	decode(t, maria.do("GET", "/datasets/teste", ""), &d)
	if d.CreatedBy != "maria" {
		t.Errorf("created_by = %q, devia ser maria", d.CreatedBy)
	}

	// o que é só de admin: apagar, o Configurar e as contas
	assertStatus(t, maria.do("DELETE", "/datasets/teste", ""), http.StatusForbidden)
	assertStatus(t, maria.do("GET", "/users", ""), http.StatusForbidden)
	assertStatus(t, maria.do("PATCH", "/users/maria", `{"role": "admin"}`), http.StatusForbidden)
	assertStatus(t, maria.do("POST", "/annotation-profiles", `{}`), http.StatusForbidden)
	assertStatus(t, maria.do("PUT", "/import-profiles/x", `{}`), http.StatusForbidden)
	assertStatus(t, maria.do("GET", "/annotation-profiles", ""), http.StatusOK) // ler pode
	assertStatus(t, admin.do("DELETE", "/datasets/teste", ""), http.StatusNoContent)

	// trocar a senha: a atual tem que conferir; as outras sessões caem
	other := &client{t: t, handler: handler}
	assertStatus(t, other.login("maria", "senha-da-maria"), http.StatusOK)
	assertStatus(t, maria.do("PUT", "/auth/password", `{"current": "errada", "new": "nova-senha-123"}`), http.StatusBadRequest)
	assertStatus(t, maria.do("PUT", "/auth/password", `{"current": "senha-da-maria", "new": "nova-senha-123"}`), http.StatusNoContent)
	assertStatus(t, maria.do("GET", "/auth/me", ""), http.StatusOK) // este computador continua
	assertStatus(t, other.do("GET", "/auth/me", ""), http.StatusUnauthorized)
	assertStatus(t, other.login("maria", "senha-da-maria"), http.StatusUnauthorized)

	// desativada: sai na hora
	assertStatus(t, admin.do("PATCH", "/users/maria", `{"status": "disabled"}`), http.StatusOK)
	assertStatus(t, maria.do("GET", "/packages", ""), http.StatusUnauthorized)
	assertStatus(t, maria.login("maria", "nova-senha-123"), http.StatusForbidden)

	// o único admin não se desativa nem perde o papel
	assertStatus(t, admin.do("PATCH", "/users/admin", `{"role": "user"}`), http.StatusConflict)
	assertStatus(t, admin.do("PATCH", "/users/admin", `{"status": "disabled"}`), http.StatusConflict)

	// o admin redefine a senha de quem esqueceu
	assertStatus(t, admin.do("PATCH", "/users/maria", `{"status": "active"}`), http.StatusOK)
	assertStatus(t, admin.do("PUT", "/users/maria/password", `{"password": "senha-nova-dela"}`), http.StatusNoContent)
	assertStatus(t, maria.login("maria", "senha-nova-dela"), http.StatusOK)

	// sair
	assertStatus(t, admin.do("POST", "/auth/logout", ""), http.StatusNoContent)
	if admin.cookie != nil {
		t.Error("o logout tem que apagar o cookie")
	}
	assertStatus(t, admin.do("GET", "/users", ""), http.StatusUnauthorized)

	// adivinhar a senha: depois de 5 erros, espera
	for i := 0; i < 5; i++ {
		assertStatus(t, anon.login("admin", "chute-errado"), http.StatusUnauthorized)
	}
	assertStatus(t, anon.login("admin", "senha-do-admin"), http.StatusTooManyRequests)
}
