package annotation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"SIDM/internal/platform"
)

type Service struct {
	repo     Repo
	packages Packages
	now      func() time.Time
}

func New(repo Repo) *Service {
	return &Service{repo: repo, now: func() time.Time { return time.Now().UTC() }}
}

// SetPackages liga o catálogo depois de criado (o catálogo também depende
// deste módulo: o main cria os dois e depois liga).
func (s *Service) SetPackages(p Packages) { s.packages = p }

// ListProfiles: os padrões e depois os salvos, por nome, já com a herança.
// Um perfil cujo pai sumiu (não devia acontecer: o pai não se apaga com
// filhos) vem sem os campos do pai e é logado.
func (s *Service) ListProfiles(ctx context.Context) ([]Profile, error) {
	saved, err := s.repo.ListProfiles(ctx)
	if err != nil {
		return nil, err
	}
	var out []Profile
	for _, b := range Builtins() {
		p, err := s.builtinWithLabels(ctx, b)
		if err != nil {
			return nil, err
		}
		r, _ := s.resolve(ctx, p) // um padrão não estende ninguém: não falha
		out = append(out, *r)
	}
	for i := range saved {
		if _, builtin := Builtin(saved[i].Name); builtin {
			continue // as caixas de um padrão (já entraram acima)
		}
		r, err := s.resolve(ctx, &saved[i])
		if err != nil {
			platform.LogError(fmt.Errorf("perfil de anotação %s: %w", saved[i].Name, err))
			out = append(out, saved[i])
			continue
		}
		out = append(out, *r)
	}
	return out, nil
}

// FindProfile busca um padrão ou um salvo (platform.ErrNotFound se nenhum),
// já com os campos herdados dos pais.
func (s *Service) FindProfile(ctx context.Context, name string) (*Profile, error) {
	p, err := s.raw(ctx, name)
	if err != nil {
		return nil, err
	}
	return s.resolve(ctx, p)
}

// CreateProfile valida e cadastra (platform.ErrDuplicate se o nome existe).
func (s *Service) CreateProfile(ctx context.Context, p *Profile, user string) error {
	if err := s.prepare(ctx, p); err != nil {
		return err
	}
	if _, builtin := Builtin(p.Name); builtin {
		return &InvalidError{Msg: fmt.Sprintf("%q é o nome de um perfil padrão: escolha outro", p.Name)}
	}
	p.CreatedBy, p.UpdatedAt = user, s.now()
	return s.repo.CreateProfile(ctx, p)
}

// ReplaceProfile grava a nova versão de um perfil salvo, mantendo quem criou.
// De um perfil padrão, só as caixas que representam os tipos mudam.
func (s *Service) ReplaceProfile(ctx context.Context, p *Profile) error {
	if b, builtin := Builtin(p.Name); builtin {
		return s.replaceBuiltinLabels(ctx, b, p)
	}
	if err := s.prepare(ctx, p); err != nil {
		return err
	}
	old, err := s.repo.FindProfile(ctx, p.Name)
	if err != nil {
		return err
	}
	if err := s.checkFamily(ctx, p); err != nil {
		return err
	}
	p.CreatedBy, p.UpdatedAt = old.CreatedBy, s.now()
	return s.repo.ReplaceProfile(ctx, p)
}

// DeleteProfile apaga um perfil salvo que nenhum pacote usa.
func (s *Service) DeleteProfile(ctx context.Context, name string) error {
	if err := editable(name); err != nil {
		return err
	}
	if family, err := s.Family(ctx, name); err != nil {
		return err
	} else if len(family) > 1 {
		return &InvalidError{Msg: fmt.Sprintf("o perfil %q é estendido por %s: apague ou mude esses antes", name, strings.Join(family[1:], ", "))}
	}
	if s.packages != nil {
		using, err := s.packages.PackagesWithProfile(ctx, name)
		if err != nil {
			return err
		}
		if len(using) > 0 {
			shown := using
			if len(shown) > 5 {
				shown = append(shown[:5:5], "...")
			}
			return &InvalidError{Msg: fmt.Sprintf("o perfil %q está em uso por %d pacote(s) (%s): troque o perfil deles antes de apagar",
				name, len(using), strings.Join(shown, ", "))}
		}
	}
	return s.repo.DeleteProfile(ctx, name)
}

// prepare: só o que é do perfil (os herdados que a tela devolveu saem),
// limpo e conferido sozinho e junto com os pais.
func (s *Service) prepare(ctx context.Context, p *Profile) error {
	p.Builtin = false // só o código marca perfil padrão
	own(p)
	p.Normalize()
	if err := p.Validate(); err != nil {
		return err
	}
	if p.Extends == p.Name && p.Name != "" {
		return &InvalidError{Msg: "um perfil não estende ele mesmo", Problems: []Problem{{Path: "extends", Msg: "escolha outro perfil"}}}
	}
	r, err := s.resolveWith(ctx, p, p)
	if err != nil {
		return err
	}
	return r.validateLinks()
}

func editable(name string) error {
	if _, builtin := Builtin(name); builtin {
		return &InvalidError{Msg: fmt.Sprintf("%q é um perfil padrão e não pode ser apagado", name)}
	}
	return nil
}
