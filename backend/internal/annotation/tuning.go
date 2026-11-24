package annotation

// Os perfis padrão (do código) não mudam, com uma exceção: as caixas que
// representam cada tipo de objeto (BoxLabels). Labels novas aparecem com o
// tempo (uma tarefa nova no CVAT), e ligar uma label a um tipo não muda o
// que já foi anotado. Essas caixas ficam no banco, num registro com o nome
// do padrão que só tem os tipos e as caixas.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"SIDM/internal/platform"
)

// builtinWithLabels: o padrão do código com as caixas gravadas no banco.
func (s *Service) builtinWithLabels(ctx context.Context, b Profile) (*Profile, error) {
	saved, err := s.repo.FindProfile(ctx, b.Name)
	if errors.Is(err, platform.ErrNotFound) {
		return &b, nil
	}
	if err != nil {
		return nil, err
	}
	out := withLabels(b, saved.Objects)
	return &out, nil
}

// withLabels põe as caixas de cada tipo (pela chave) no padrão. Um tipo que
// o padrão não tem mais é ignorado.
func withLabels(b Profile, labels []ObjectType) Profile {
	b.Objects = slices.Clone(b.Objects)
	for i := range b.Objects {
		j := slices.IndexFunc(labels, func(o ObjectType) bool { return o.Key == b.Objects[i].Key })
		if j >= 0 {
			b.Objects[i].BoxLabels = slices.Clone(labels[j].BoxLabels)
		} else {
			b.Objects[i].BoxLabels = nil
		}
	}
	return b
}

// replaceBuiltinLabels: de um padrão, só as caixas de cada tipo valem (o
// resto que vier é ignorado: o padrão é o do código).
func (s *Service) replaceBuiltinLabels(ctx context.Context, b Profile, p *Profile) error {
	var labels []ObjectType
	for _, o := range p.Objects {
		own := ownLabels(o.BoxLabels)
		for i := range own {
			own[i].Label = strings.TrimSpace(own[i].Label)
		}
		if len(own) > 0 {
			labels = append(labels, ObjectType{Key: o.Key, BoxLabels: own})
		}
	}
	merged := withLabels(b, labels)
	if err := merged.validateLinks(); err != nil {
		return err
	}
	if err := s.checkFamily(ctx, &merged); err != nil {
		return err
	}
	record := &Profile{Name: b.Name, Objects: labels, UpdatedAt: s.now()}
	_, err := s.repo.FindProfile(ctx, b.Name)
	switch {
	case errors.Is(err, platform.ErrNotFound):
		err = s.repo.CreateProfile(ctx, record)
	case err == nil:
		err = s.repo.ReplaceProfile(ctx, record)
	}
	if err != nil {
		return fmt.Errorf("caixas do perfil %s: %w", b.Name, err)
	}
	*p = merged
	return nil
}
