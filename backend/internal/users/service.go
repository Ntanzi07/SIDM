package users

import (
	"context"
	"errors"
	"fmt"
	"regexp"
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

func checkNewPassword(p string) error {
	if utf8.RuneCountInString(p) < MinPassword {
		return &InvalidError{fmt.Sprintf("a senha precisa ter pelo menos %d caracteres", MinPassword)}
	}
	if len(p) > 200 {
		return &InvalidError{"senha longa demais"}
	}
	return nil
}

// RegisterInput é o corpo do cadastro.
type RegisterInput struct {
	Username string `json:"username"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

// Register cria uma conta PENDENTE (um admin aprova). platform.ErrDuplicate
// se o nome já existe.
func (s *Service) Register(ctx context.Context, in RegisterInput) (*User, error) {
	u, err := s.newUser(in, RoleUser, StatusPending)
	if err != nil {
		return nil, err
	}
	if err := s.repo.CreateUser(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

func (s *Service) newUser(in RegisterInput, role, status string) (*User, error) {
	name := strings.TrimSpace(in.Name)
	username := cleanUsername(in.Username)
	if err := checkUsername(username); err != nil {
		return nil, err
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
	return &User{Username: username, Name: name, PasswordHash: hash, Role: role, Status: status, CreatedAt: s.now()}, nil
}

// EnsureAdmin: na subida, se não existe nenhum admin ativo, cria (ou
// promove e ativa) a conta dada. Com um admin já ativo, não faz nada (a
// senha do .env não sobrescreve a que ele trocou). Devolve se mexeu.
func (s *Service) EnsureAdmin(ctx context.Context, username, password string) (bool, error) {
	n, err := s.repo.CountActiveAdmins(ctx)
	if err != nil || n > 0 {
		return false, err
	}
	username = cleanUsername(username)
	existing, err := s.repo.FindUser(ctx, username)
	switch {
	case errors.Is(err, platform.ErrNotFound):
		u, err := s.newUser(RegisterInput{Username: username, Name: username, Password: password}, RoleAdmin, StatusActive)
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
	existing.ApprovedBy, existing.ApprovedAt = "sistema", &now
	return true, s.repo.ReplaceUser(ctx, existing)
}

// Login confere a senha e abre uma sessão. Devolve o token (vai no cookie).
func (s *Service) Login(ctx context.Context, username, password string) (string, *User, error) {
	username = cleanUsername(username)
	u, err := s.repo.FindUser(ctx, username)
	if errors.Is(err, platform.ErrNotFound) {
		checkPassword(dummyHash(), password) // o mesmo tempo de uma conta que existe
		return "", nil, &LoginError{Msg: "usuário ou senha incorretos"}
	}
	if err != nil {
		return "", nil, err
	}
	if !checkPassword(u.PasswordHash, password) {
		return "", nil, &LoginError{Msg: "usuário ou senha incorretos"}
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
