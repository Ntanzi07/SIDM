package annotation

import (
	"context"
	"fmt"
	"time"
)

type Service struct {
	repo Repo
	now  func() time.Time
}

func New(repo Repo) *Service {
	return &Service{repo: repo, now: func() time.Time { return time.Now().UTC() }}
}

// ListProfiles: os padrões e depois os salvos, por nome.
func (s *Service) ListProfiles(ctx context.Context) ([]Profile, error) {
	saved, err := s.repo.ListProfiles(ctx)
	if err != nil {
		return nil, err
	}
	return append(Builtins(), saved...), nil
}

// FindProfile busca um padrão ou um salvo (platform.ErrNotFound se nenhum).
func (s *Service) FindProfile(ctx context.Context, name string) (*Profile, error) {
	if p, ok := Builtin(name); ok {
		return &p, nil
	}
	return s.repo.FindProfile(ctx, name)
}

// CreateProfile valida e cadastra (platform.ErrDuplicate se o nome existe).
func (s *Service) CreateProfile(ctx context.Context, p *Profile, user string) error {
	if err := s.prepare(p); err != nil {
		return err
	}
	if _, builtin := Builtin(p.Name); builtin {
		return &InvalidError{Msg: fmt.Sprintf("%q é o nome de um perfil padrão: escolha outro", p.Name)}
	}
	p.CreatedBy, p.UpdatedAt = user, s.now()
	return s.repo.CreateProfile(ctx, p)
}

// ReplaceProfile grava a nova versão de um perfil salvo, mantendo quem criou.
func (s *Service) ReplaceProfile(ctx context.Context, p *Profile) error {
	if err := editable(p.Name); err != nil {
		return err
	}
	if err := s.prepare(p); err != nil {
		return err
	}
	old, err := s.repo.FindProfile(ctx, p.Name)
	if err != nil {
		return err
	}
	p.CreatedBy, p.UpdatedAt = old.CreatedBy, s.now()
	return s.repo.ReplaceProfile(ctx, p)
}

// DeleteProfile apaga um perfil salvo.
func (s *Service) DeleteProfile(ctx context.Context, name string) error {
	if err := editable(name); err != nil {
		return err
	}
	return s.repo.DeleteProfile(ctx, name)
}

func (s *Service) prepare(p *Profile) error {
	p.Builtin = false // só o código marca perfil padrão
	p.Normalize()
	return p.Validate()
}

func editable(name string) error {
	if _, builtin := Builtin(name); builtin {
		return &InvalidError{Msg: fmt.Sprintf("%q é um perfil padrão e não pode ser alterado nem apagado: salve uma cópia com outro nome", name)}
	}
	return nil
}
