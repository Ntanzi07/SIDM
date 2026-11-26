// Package users é o login do SIDM: as contas (admin ou usuário normal), o
// cadastro com aprovação, as sessões e quem pode o quê.
//
// Uma pessoa se cadastra e fica PENDENTE até um admin aprovar. O login dura
// 8 horas: um token aleatório num cookie HttpOnly; no banco fica só o hash
// dele (quem lê o banco não consegue entrar com o que está lá). A senha é
// guardada com PBKDF2-SHA256 (biblioteca padrão do Go), nunca em texto.
//
// O resto do sistema continua lendo o usuário do header X-User
// (platform.UserFrom): o middleware apaga o que veio do navegador e escreve
// o da sessão. Assim o histórico das imagens e a fila não mudam.
//
// Coleções: users e sessions (no mesmo banco sidm).
package users

import (
	"context"
	"time"
)

// Papéis.
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// Situação da conta.
const (
	StatusPending  = "pending"  // cadastrou, esperando um admin aprovar
	StatusActive   = "active"   // pode entrar
	StatusDisabled = "disabled" // desativada (o histórico continua com o nome)
)

// SessionTTL: quanto o login dura.
const SessionTTL = 8 * time.Hour

// CookieName: o cookie da sessão.
const CookieName = "sidm_session"

// User é um documento da coleção users. Entra-se pelo e-mail; o _id é o
// nome de usuário (o que aparece no histórico das imagens), tirado do
// e-mail no cadastro (maria.silva@empresa.com → maria.silva).
type User struct {
	Username     string     `bson:"_id" json:"username"`
	Email        string     `bson:"email,omitempty" json:"email,omitempty"` // único; o admin do .env pode não ter
	Name         string     `bson:"name" json:"name"`
	PasswordHash string     `bson:"password_hash" json:"-"`
	Role         string     `bson:"role" json:"role"`
	Status       string     `bson:"status" json:"status"`
	CreatedAt    time.Time  `bson:"created_at" json:"created_at"`
	ApprovedBy   string     `bson:"approved_by,omitempty" json:"approved_by,omitempty"`
	ApprovedAt   *time.Time `bson:"approved_at,omitempty" json:"approved_at,omitempty"`
	LastLogin    *time.Time `bson:"last_login,omitempty" json:"last_login,omitempty"`
	// Shortcuts: os atalhos de teclado da pessoa (ação → tecla, ex.:
	// "annotation.next" → "f"); vazio = os padrões da tela.
	Shortcuts map[string]string `bson:"shortcuts,omitempty" json:"shortcuts,omitempty"`
}

// IsAdmin: admin ativo.
func (u *User) IsAdmin() bool { return u.Role == RoleAdmin && u.Status == StatusActive }

// Session é um documento da coleção sessions. O _id é o sha256 do token
// (o token mesmo só existe no cookie).
type Session struct {
	ID        string    `bson:"_id"`
	Username  string    `bson:"username"`
	CreatedAt time.Time `bson:"created_at"`
	ExpiresAt time.Time `bson:"expires_at"` // índice TTL: o Mongo apaga sozinho depois
}

// Repo são as coleções do módulo.
type Repo interface {
	CreateUser(ctx context.Context, u *User) error // platform.ErrDuplicate
	FindUser(ctx context.Context, username string) (*User, error)
	FindUserByEmail(ctx context.Context, email string) (*User, error)
	ListUsers(ctx context.Context) ([]User, error)
	ReplaceUser(ctx context.Context, u *User) error
	CountActiveAdmins(ctx context.Context) (int64, error)

	CreateSession(ctx context.Context, s *Session) error
	FindSession(ctx context.Context, id string) (*Session, error)
	DeleteSession(ctx context.Context, id string) error
	// DeleteSessions: todas da pessoa (troca de senha, desativação).
	DeleteSessions(ctx context.Context, username string) error
}
