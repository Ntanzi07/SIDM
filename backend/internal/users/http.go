package users

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"SIDM/internal/platform"
)

// Routes são as rotas do módulo.
func (s *Service) Routes() []platform.Route {
	return []platform.Route{
		{Method: "POST", Path: "/auth/register", Handler: s.handleRegister},
		{Method: "POST", Path: "/auth/login", Handler: s.handleLogin},
		{Method: "POST", Path: "/auth/logout", Handler: s.handleLogout},
		{Method: "GET", Path: "/auth/me", Handler: s.handleMe},
		{Method: "PUT", Path: "/auth/password", Handler: s.handleChangePassword},
		{Method: "PUT", Path: "/auth/shortcuts", Handler: s.handleShortcuts},
		{Method: "GET", Path: "/users", Handler: s.handleList},
		{Method: "PATCH", Path: "/users/{username}", Handler: s.handleUpdate},
		{Method: "PUT", Path: "/users/{username}/password", Handler: s.handleResetPassword},
	}
}

// ---------- quem pode o quê ----------

// public: o que responde sem login (entrar, cadastrar, a saúde e a
// documentação da API).
func public(method, path string) bool {
	switch method + " " + path {
	case "GET /health", "GET /docs", "GET /openapi.yaml",
		"POST /auth/login", "POST /auth/register", "POST /auth/logout", "GET /auth/me":
		return true
	}
	return false
}

// userWrites: o que o usuário normal pode GRAVAR (no formato das rotas do
// Go). Ler (GET) ele pode tudo, menos as contas e a página Processar
// (adminAreas). Ele anota, importa (cria
// pacote, sobe imagens, importa CSV e caixas), apaga imagens, baixa o export
// e cria datasets e versões. O resto (tarefas e labels, perfis, renomear e
// apagar pacotes, apagar datasets e versões, contas) é só de admin.
var userWrites = []string{
	// anotar
	"POST /packages/{name}/next",
	"PUT /images/{sha1}",
	"PUT /images/{sha1}/boxes/{task}",
	"POST /images/{sha1}/release",
	"POST /images/{sha1}/reopen",
	"POST /equipments",         // cadastrar na anotação
	"PATCH /equipments/{name}", // reativar na anotação
	"POST /cameras",
	"PATCH /cameras/{name}",
	"POST /annotation-profiles/{name}/check", // só confere, não grava
	// importar
	"POST /packages",
	"POST /images",
	"POST /images/exists",
	"POST /packages/{name}/import",
	"POST /imports/inspect",
	"POST /tasks/{name}/import",
	"POST /packages/{name}/boxes/link", // o que o import das caixas já faz
	"DELETE /images/{sha1}",
	"POST /packages/{name}/zip-uploads", // o zip em partes
	"PUT /zip-uploads/{id}/chunk",
	"POST /zip-uploads/{id}/finish",
	"DELETE /zip-uploads/{id}", // cancelar o próprio envio
	// export e versões
	"POST /exports/preview",
	"POST /exports/download",
	"POST /datasets",
	"POST /datasets/{name}/versions",
	"POST /datasets/{name}/versions/{number}/items/{sha1}/restore", // a volta
	// a própria conta
	"PUT /auth/password",
	"PUT /auth/shortcuts", // os próprios atalhos de teclado
}

// userMux casa um pedido com userWrites do mesmo jeito que o servidor casa
// as rotas (um pacote chamado "images" não confunde nada).
var userMux = func() *http.ServeMux {
	mux := http.NewServeMux()
	for _, pattern := range userWrites {
		mux.HandleFunc(pattern, func(http.ResponseWriter, *http.Request) {})
	}
	return mux
}()

// adminOnly: o que o usuário normal não pode. Toda gravação que não está em
// userWrites, e tudo (até ler) das contas e da página Processar.
func adminOnly(r *http.Request) bool {
	for _, prefix := range adminAreas {
		if r.URL.Path == prefix || strings.HasPrefix(r.URL.Path, prefix+"/") {
			return true
		}
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return false
	}
	_, pattern := userMux.Handler(r)
	return pattern == ""
}

// adminAreas: as rotas que o usuário normal nem lê (o caminho e o que vem
// abaixo dele).
var adminAreas = []string{
	"/users",      // as contas
	"/processing", // a página Processar (modelos de IA)
	"/system",     // o Sistema do Configurar (disco, lixeira)
}

type ctxKey struct{}

// FromContext: a conta logada (nil fora do middleware, ex.: nos testes).
func FromContext(ctx context.Context) *User {
	u, _ := ctx.Value(ctxKey{}).(*User)
	return u
}

// Protect exige login em tudo que não é público e admin no que é só de
// admin. O X-User que veio do navegador é trocado pelo da sessão: é dele
// que o resto do sistema lê quem fez a ação (platform.UserFrom).
func (s *Service) Protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Del("X-User") // nunca vale o que o navegador mandou
		var token string
		if c, err := r.Cookie(CookieName); err == nil {
			token = c.Value
		}
		u, err := s.Authenticate(r.Context(), token)
		if err != nil {
			platform.ServerError(w, err)
			return
		}
		if u != nil {
			r.Header.Set("X-User", u.Username)
			r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, u))
		}
		switch {
		case public(r.Method, r.URL.Path):
		case u == nil:
			platform.WriteError(w, http.StatusUnauthorized, "faça login para continuar")
			return
		case adminOnly(r) && !u.IsAdmin():
			platform.WriteError(w, http.StatusForbidden, "só um admin pode fazer isso")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---------- handlers ----------

func writeErr(w http.ResponseWriter, err error) {
	var invalid *InvalidError
	var login *LoginError
	var conflict *ConflictError
	switch {
	case errors.As(err, &invalid):
		platform.WriteError(w, http.StatusBadRequest, invalid.Msg)
	case errors.As(err, &login):
		status := http.StatusUnauthorized
		if login.Forbidden {
			status = http.StatusForbidden
		}
		platform.WriteError(w, status, login.Msg)
	case errors.As(err, &conflict):
		platform.WriteError(w, http.StatusConflict, conflict.Msg)
	case errors.Is(err, platform.ErrNotFound):
		platform.WriteError(w, http.StatusNotFound, "usuário não encontrado")
	case errors.Is(err, ErrEmailTaken):
		platform.WriteError(w, http.StatusConflict, "este e-mail já tem uma conta: entre com ele (ou peça uma senha nova a um admin)")
	case errors.Is(err, platform.ErrDuplicate):
		platform.WriteError(w, http.StatusConflict, "este usuário já existe: escolha outro")
	default:
		platform.ServerError(w, err)
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		platform.WriteError(w, http.StatusBadRequest, "JSON inválido: "+err.Error())
		return false
	}
	return true
}

func (s *Service) handleRegister(w http.ResponseWriter, r *http.Request) {
	var in RegisterInput
	if !decode(w, r, &in) {
		return
	}
	u, err := s.Register(r.Context(), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusCreated, u)
}

func (s *Service) handleLogin(w http.ResponseWriter, r *http.Request) {
	// login: o e-mail ou o nome de usuário ("username", o nome antigo do campo, ainda vale)
	var in struct {
		Login    string `json:"login"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Login == "" {
		in.Login = in.Username
	}
	key := strings.ToLower(strings.TrimSpace(in.Login))
	if !s.limiter.allow(key) {
		platform.WriteError(w, http.StatusTooManyRequests, "muitas tentativas erradas: espere alguns minutos")
		return
	}
	token, u, err := s.Login(r.Context(), in.Login, in.Password)
	var login *LoginError
	if errors.As(err, &login) && !login.Forbidden {
		s.limiter.fail(key)
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	s.limiter.reset(key)
	s.limiter.reset(u.Username) // entrou: zera também o outro jeito de escrever a conta
	s.limiter.reset(u.Email)
	http.SetCookie(w, sessionCookie(r, token, int(SessionTTL.Seconds())))
	platform.WriteJSON(w, http.StatusOK, u)
}

// sessionCookie: HttpOnly (o JavaScript não lê), SameSite=Lax (outro site
// não manda o cookie num POST) e Secure quando a conexão é HTTPS.
func sessionCookie(r *http.Request, value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name: CookieName, Value: value, Path: "/", MaxAge: maxAge,
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
	}
}

func (s *Service) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(CookieName); err == nil {
		if err := s.Logout(r.Context(), c.Value); err != nil {
			platform.ServerError(w, err)
			return
		}
	}
	http.SetCookie(w, sessionCookie(r, "", -1))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) handleMe(w http.ResponseWriter, r *http.Request) {
	u := FromContext(r.Context())
	if u == nil {
		platform.WriteError(w, http.StatusUnauthorized, "faça login para continuar")
		return
	}
	platform.WriteJSON(w, http.StatusOK, u)
}

func (s *Service) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if !decode(w, r, &in) {
		return
	}
	user := platform.UserFrom(r)
	if err := s.ChangePassword(r.Context(), user, in.Current, in.New); err != nil {
		writeErr(w, err)
		return
	}
	// a troca fecha todas as sessões: abre uma nova para este computador
	token, _, err := s.Login(r.Context(), user, in.New)
	if err != nil {
		writeErr(w, err)
		return
	}
	http.SetCookie(w, sessionCookie(r, token, int(SessionTTL.Seconds())))
	w.WriteHeader(http.StatusNoContent)
}

// PUT /auth/shortcuts  {"annotation.next": "f", ...}: troca todos os atalhos
// da pessoa logada ({} volta aos padrões). Devolve a conta.
func (s *Service) handleShortcuts(w http.ResponseWriter, r *http.Request) {
	var in map[string]string
	if !decode(w, r, &in) {
		return
	}
	u, err := s.SetShortcuts(r.Context(), platform.UserFrom(r), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, u)
}

func (s *Service) handleList(w http.ResponseWriter, r *http.Request) {
	list, err := s.List(r.Context())
	if err != nil {
		platform.ServerError(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, platform.Page[User]{Total: int64(len(list)), Items: list})
}

func (s *Service) handleUpdate(w http.ResponseWriter, r *http.Request) {
	var in UpdateInput
	if !decode(w, r, &in) {
		return
	}
	u, err := s.Update(r.Context(), platform.UserFrom(r), r.PathValue("username"), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, u)
}

func (s *Service) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := s.ResetPassword(r.Context(), r.PathValue("username"), in.Password); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
