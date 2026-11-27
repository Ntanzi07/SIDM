package users

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"SIDM/internal/platform"
)

type Service struct {
	repo    Repo
	now     func() time.Time
	limiter *limiter
}

func New(repo Repo) *Service {
	now := func() time.Time { return time.Now().UTC().Truncate(time.Millisecond) }
	return &Service{repo: repo, now: now, limiter: newLimiter(now)}
}

// InvalidError: um pedido que não dá para atender (vira 400).
type InvalidError struct{ Msg string }

func (e *InvalidError) Error() string { return e.Msg }
func (e *InvalidError) Unwrap() error { return platform.ErrInvalid }

// LoginError: usuário ou senha errados, ou conta que não pode entrar (401
// ou 403). A mensagem é a que a pessoa vê.
type LoginError struct {
	Msg       string
	Forbidden bool // a senha confere, mas a conta está pendente ou desativada
}

func (e *LoginError) Error() string { return e.Msg }

// ErrEmailTaken: já existe uma conta com o e-mail (409).
var ErrEmailTaken = errors.New("este e-mail já tem uma conta")

// ConflictError: o pedido deixaria o sistema sem admin (409).
type ConflictError struct{ Msg string }

func (e *ConflictError) Error() string { return e.Msg }

var usernameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,31}$`)

// cleanUsername: minúsculo e sem espaços nas pontas ("Nathan " = "nathan").
func cleanUsername(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func checkUsername(u string) error {
	if !usernameRe.MatchString(u) {
		return &InvalidError{"usuário: de 2 a 32 caracteres, letras minúsculas, números, ponto, hífen ou _ (ex.: nathan.souza)"}
	}
	return nil
}

// cleanEmail: minúsculo e sem espaços nas pontas.
func cleanEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

var emailRe = regexp.MustCompile(`^[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}$`)

func checkEmail(e string) error {
	if len(e) > 254 || !emailRe.MatchString(e) {
		return &InvalidError{"e-mail inválido (ex.: maria.silva@empresa.com.br)"}
	}
	return nil
}

// usernameFrom: o nome de usuário a partir do e-mail (a parte antes do @,
// só com os caracteres que valem). Vazio se não sobra o bastante.
func usernameFrom(email string) string {
	local, _, _ := strings.Cut(email, "@")
	var b strings.Builder
	for _, r := range local {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('.')
		}
	}
	u := strings.Trim(b.String(), "._-")
	if len(u) > 28 {
		u = strings.TrimRight(u[:28], "._-")
	}
	return u
}

func checkNewPassword(p string) error {
	if utf8.RuneCountInString(p) < MinPassword {
		return &InvalidError{fmt.Sprintf("a senha precisa ter pelo menos %d caracteres", MinPassword)}
	}
	if len(p) > 200 {
		return &InvalidError{"senha longa demais"}
	}
	return nil
}

// RegisterInput é o corpo do cadastro. Sem username, ele sai do e-mail.
type RegisterInput struct {
	Email    string `json:"email"`
	Username string `json:"username,omitempty"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

// Register cria uma conta PENDENTE (um admin aprova). ErrEmailTaken se o
// e-mail já tem conta. O nome de usuário vem do e-mail; se já existe, ganha
// um número (maria.silva-2).
func (s *Service) Register(ctx context.Context, in RegisterInput) (*User, error) {
	return s.create(ctx, in, RoleUser, StatusPending, "")
}

// CreateInput é o corpo da conta criada por um admin. Sem role, é "user".
type CreateInput struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
	Role     string `json:"role,omitempty"`
}

// CreateByAdmin: o admin cadastra um colaborador. A conta já nasce ATIVA
// (aprovada por quem criou). Mesmas regras do cadastro: e-mail único, nome
// de usuário tirado do e-mail.
func (s *Service) CreateByAdmin(ctx context.Context, admin string, in CreateInput) (*User, error) {
	role := in.Role
	if role == "" {
		role = RoleUser
	}
	if role != RoleAdmin && role != RoleUser {
		return nil, &InvalidError{`role: "admin" ou "user"`}
	}
	return s.create(ctx, RegisterInput{Email: in.Email, Name: in.Name, Password: in.Password}, role, StatusActive, admin)
}

// create: o que o cadastro e a conta criada pelo admin têm em comum.
// approvedBy vazio = ninguém aprovou ainda.
func (s *Service) create(ctx context.Context, in RegisterInput, role, status, approvedBy string) (*User, error) {
	in.Email = cleanEmail(in.Email)
	if err := checkEmail(in.Email); err != nil {
		return nil, err
	}
	if _, err := s.repo.FindUserByEmail(ctx, in.Email); err == nil {
		return nil, ErrEmailTaken
	} else if !errors.Is(err, platform.ErrNotFound) {
		return nil, err
	}
	base := cleanUsername(in.Username)
	if base == "" {
		base = usernameFrom(in.Email)
	}
	if len(base) < 2 {
		base = "usuario"
	}
	for i := 1; ; i++ {
		in.Username = base
		if i > 1 {
			in.Username = fmt.Sprintf("%s-%d", base, i)
		}
		u, err := s.newUser(in, role, status)
		if err != nil {
			return nil, err
		}
		if approvedBy != "" {
			now := s.now()
			u.ApprovedBy, u.ApprovedAt = approvedBy, &now
		}
		err = s.repo.CreateUser(ctx, u)
		if errors.Is(err, platform.ErrDuplicate) && i < 50 {
			continue // o nome já é de outra pessoa: tenta o próximo
		}
		if err != nil {
			return nil, err
		}
		return u, nil
	}
}

func (s *Service) newUser(in RegisterInput, role, status string) (*User, error) {
	name := strings.TrimSpace(in.Name)
	username := cleanUsername(in.Username)
	if err := checkUsername(username); err != nil {
		return nil, err
	}
	email := cleanEmail(in.Email)
	if email != "" {
		if err := checkEmail(email); err != nil {
			return nil, err
		}
	}
	if name == "" {
		return nil, &InvalidError{"informe o seu nome"}
	}
	if utf8.RuneCountInString(name) > 80 {
		return nil, &InvalidError{"nome longo demais"}
	}
	if err := checkNewPassword(in.Password); err != nil {
		return nil, err
	}
	hash, err := hashPassword(in.Password)
	if err != nil {
		return nil, err
	}
	return &User{Username: username, Email: email, Name: name, PasswordHash: hash, Role: role, Status: status, CreatedAt: s.now()}, nil
}

// EnsureAdmin: na subida, se não existe nenhum admin ativo, cria (ou
// promove e ativa) a conta dada. Com um admin já ativo, não faz nada (a
// senha do .env não sobrescreve a que ele trocou). email pode ser vazio
// (aí ele entra pelo nome de usuário). Devolve se mexeu.
func (s *Service) EnsureAdmin(ctx context.Context, username, email, password string) (bool, error) {
	n, err := s.repo.CountActiveAdmins(ctx)
	if err != nil {
		return false, err
	}
	if n > 0 {
		// o admin criado antes do login por e-mail: ganha o e-mail do .env
		// (só se não tem nenhum; a senha não muda)
		return s.fillEmail(ctx, cleanUsername(username), cleanEmail(email))
	}
	username = cleanUsername(username)
	existing, err := s.repo.FindUser(ctx, username)
	switch {
	case errors.Is(err, platform.ErrNotFound):
		u, err := s.newUser(RegisterInput{Username: username, Email: email, Name: username, Password: password}, RoleAdmin, StatusActive)
		if err != nil {
			return false, err
		}
		now := s.now()
		u.ApprovedBy, u.ApprovedAt = "sistema", &now
		return true, s.repo.CreateUser(ctx, u)
	case err != nil:
		return false, err
	}
	if err := checkNewPassword(password); err != nil {
		return false, err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return false, err
	}
	now := s.now()
	existing.Role, existing.Status, existing.PasswordHash = RoleAdmin, StatusActive, hash
	if email = cleanEmail(email); email != "" {
		if err := checkEmail(email); err != nil {
			return false, err
		}
		existing.Email = email
	}
	existing.ApprovedBy, existing.ApprovedAt = "sistema", &now
	return true, s.repo.ReplaceUser(ctx, existing)
}

func (s *Service) fillEmail(ctx context.Context, username, email string) (bool, error) {
	if email == "" {
		return false, nil
	}
	u, err := s.repo.FindUser(ctx, username)
	if errors.Is(err, platform.ErrNotFound) {
		return false, nil
	}
	if err != nil || u.Email != "" {
		return false, err
	}
	if err := checkEmail(email); err != nil {
		return false, err
	}
	if _, err := s.repo.FindUserByEmail(ctx, email); err == nil {
		return false, ErrEmailTaken
	}
	u.Email = email
	return true, s.repo.ReplaceUser(ctx, u)
}

// Login confere a senha e abre uma sessão. login é o e-mail (com @) ou o
// nome de usuário. Devolve o token (vai no cookie).
func (s *Service) Login(ctx context.Context, login, password string) (string, *User, error) {
	var u *User
	var err error
	if login = strings.TrimSpace(login); strings.Contains(login, "@") {
		u, err = s.repo.FindUserByEmail(ctx, cleanEmail(login))
	} else {
		u, err = s.repo.FindUser(ctx, cleanUsername(login))
	}
	if errors.Is(err, platform.ErrNotFound) {
		checkPassword(dummyHash(), password) // o mesmo tempo de uma conta que existe
		return "", nil, &LoginError{Msg: "e-mail ou senha incorretos"}
	}
	if err != nil {
		return "", nil, err
	}
	if !checkPassword(u.PasswordHash, password) {
		return "", nil, &LoginError{Msg: "e-mail ou senha incorretos"}
	}
	switch u.Status {
	case StatusPending:
		return "", nil, &LoginError{Msg: "a sua conta ainda não foi aprovada: peça para um admin aprovar", Forbidden: true}
	case StatusDisabled:
		return "", nil, &LoginError{Msg: "esta conta está desativada", Forbidden: true}
	}
	token, id, err := newToken()
	if err != nil {
		return "", nil, err
	}
	now := s.now()
	if err := s.repo.CreateSession(ctx, &Session{ID: id, Username: u.Username, CreatedAt: now, ExpiresAt: now.Add(SessionTTL)}); err != nil {
		return "", nil, err
	}
	u.LastLogin = &now
	if err := s.repo.ReplaceUser(ctx, u); err != nil {
		platform.LogError(fmt.Errorf("último login de %s: %w", u.Username, err))
	}
	return token, u, nil
}

// Authenticate: a conta da sessão do token (nil se não tem sessão válida,
// se expirou, ou se a conta não está mais ativa).
func (s *Service) Authenticate(ctx context.Context, token string) (*User, error) {
	if token == "" {
		return nil, nil
	}
	sess, err := s.repo.FindSession(ctx, tokenID(token))
	if errors.Is(err, platform.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !s.now().Before(sess.ExpiresAt) { // o TTL do Mongo apaga com atraso
		return nil, nil
	}
	u, err := s.repo.FindUser(ctx, sess.Username)
	if errors.Is(err, platform.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if u.Status != StatusActive {
		return nil, nil
	}
	return u, nil
}

// Logout fecha a sessão do token.
func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.repo.DeleteSession(ctx, tokenID(token))
}

// ChangePassword: a própria senha, conferindo a atual. Fecha as outras
// sessões (as de outros computadores).
func (s *Service) ChangePassword(ctx context.Context, username, current, next string) error {
	u, err := s.repo.FindUser(ctx, username)
	if err != nil {
		return err
	}
	if !checkPassword(u.PasswordHash, current) {
		return &InvalidError{"a senha atual não confere"}
	}
	return s.setPassword(ctx, u, next)
}

func (s *Service) setPassword(ctx context.Context, u *User, password string) error {
	if err := checkNewPassword(password); err != nil {
		return err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	u.PasswordHash = hash
	if err := s.repo.ReplaceUser(ctx, u); err != nil {
		return err
	}
	return s.repo.DeleteSessions(ctx, u.Username)
}

// ---------- admin ----------

func (s *Service) List(ctx context.Context) ([]User, error) { return s.repo.ListUsers(ctx) }

// UpdateInput: o que o admin muda numa conta (nil = não muda).
type UpdateInput struct {
	Status *string `json:"status"` // active (aprova/reativa) ou disabled
	Role   *string `json:"role"`   // admin ou user
	Name   *string `json:"name"`
}

// Update: aprovar, desativar, trocar o papel. O sistema nunca fica sem
// nenhum admin ativo.
func (s *Service) Update(ctx context.Context, admin, username string, in UpdateInput) (*User, error) {
	u, err := s.repo.FindUser(ctx, cleanUsername(username))
	if err != nil {
		return nil, err
	}
	wasAdmin := u.IsAdmin()
	if in.Status != nil {
		switch *in.Status {
		case StatusActive:
			if u.Status == StatusPending {
				now := s.now()
				u.ApprovedBy, u.ApprovedAt = admin, &now
			}
		case StatusDisabled:
		default:
			return nil, &InvalidError{`status: "active" (aprovar ou reativar) ou "disabled" (desativar)`}
		}
		u.Status = *in.Status
	}
	if in.Role != nil {
		if *in.Role != RoleAdmin && *in.Role != RoleUser {
			return nil, &InvalidError{`role: "admin" ou "user"`}
		}
		u.Role = *in.Role
	}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" || utf8.RuneCountInString(name) > 80 {
			return nil, &InvalidError{"nome: de 1 a 80 caracteres"}
		}
		u.Name = name
	}
	if wasAdmin && !u.IsAdmin() {
		n, err := s.repo.CountActiveAdmins(ctx)
		if err != nil {
			return nil, err
		}
		if n <= 1 {
			return nil, &ConflictError{"é o único admin ativo: promova outra pessoa antes"}
		}
	}
	if err := s.repo.ReplaceUser(ctx, u); err != nil {
		return nil, err
	}
	if u.Status != StatusActive {
		if err := s.repo.DeleteSessions(ctx, u.Username); err != nil { // desativada: sai na hora
			return nil, err
		}
	}
	return u, nil
}

// ResetPassword: o admin define uma senha nova (a pessoa esqueceu).
func (s *Service) ResetPassword(ctx context.Context, username, password string) error {
	u, err := s.repo.FindUser(ctx, cleanUsername(username))
	if err != nil {
		return err
	}
	return s.setPassword(ctx, u, password)
}

var (
	shortcutAction = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`)
	maxShortcuts   = 60
)

// SetShortcuts troca os atalhos de teclado da pessoa. A tela é quem sabe
// as ações; aqui só se confere o formato e que uma tecla não serve a duas
// ações.
func (s *Service) SetShortcuts(ctx context.Context, username string, in map[string]string) (*User, error) {
	if len(in) > maxShortcuts {
		return nil, &InvalidError{fmt.Sprintf("no máximo %d atalhos", maxShortcuts)}
	}
	clean := make(map[string]string, len(in))
	used := map[string]string{}
	for _, action := range slices.Sorted(maps.Keys(in)) {
		key := strings.ToLower(strings.TrimSpace(in[action]))
		switch {
		case len(action) > 60 || !shortcutAction.MatchString(action):
			return nil, &InvalidError{fmt.Sprintf("ação inválida: %q", action)}
		case len(key) > 40:
			return nil, &InvalidError{fmt.Sprintf("tecla longa demais em %q", action)}
		case key == "":
			continue // sem tecla: a ação fica sem atalho só se a tela mandar "none"
		case used[key] != "":
			return nil, &InvalidError{fmt.Sprintf("a tecla %q está em %q e em %q", key, used[key], action)}
		}
		if key != "none" {
			used[key] = action
		}
		clean[action] = key
	}
	u, err := s.repo.FindUser(ctx, username)
	if err != nil {
		return nil, err
	}
	u.Shortcuts = clean
	if len(clean) == 0 {
		u.Shortcuts = nil
	}
	if err := s.repo.ReplaceUser(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}
