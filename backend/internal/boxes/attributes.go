package boxes

import (
	"context"
	"errors"
	"fmt"
	"time"

	"SIDM/internal/annotation"
	"SIDM/internal/platform"
)

// A edição dos atributos das caixas no SIDM. A posição e a label das caixas
// vêm do CVAT e não mudam aqui; um reimport da tarefa substitui tudo (o
// CVAT é a fonte).

// AttributesInput é o pedido de edição dos atributos de uma imagem numa
// tarefa: uma entrada por caixa, na ordem do GET, e a versão lida.
type AttributesInput struct {
	Version time.Time `json:"version"`
	Boxes   []struct {
		Attributes map[string]any `json:"attributes"`
	} `json:"boxes"`
}

// UpdateAttributes confere os atributos pelo perfil do pacote e grava.
// Caixa de label com tipo no perfil: os valores passam pelo CheckFields (o
// erro volta com o caminho "boxes[i].attributes.<campo>"). Caixa sem tipo:
// os atributos ficam como estavam (não dá para conferir).
func (s *Service) UpdateAttributes(ctx context.Context, sha1, task string, in AttributesInput, user string) (*Set, error) {
	sets, err := s.repo.FindSets(ctx, task, []string{sha1})
	if err != nil {
		return nil, err
	}
	set, ok := sets[sha1]
	if !ok {
		return nil, platform.ErrNotFound
	}
	if !set.Version().Equal(in.Version) {
		return nil, ErrConflict
	}
	if len(in.Boxes) != len(set.Boxes) {
		return nil, &InvalidError{fmt.Sprintf("a imagem tem %d caixa(s) na tarefa %q, vieram %d", len(set.Boxes), task, len(in.Boxes))}
	}
	p, err := s.profiles.ProfileOf(ctx, set.Package)
	if err != nil {
		return nil, err
	}

	var all []annotation.Problem
	boxes := make([]Box, len(set.Boxes))
	copy(boxes, set.Boxes)
	for i := range boxes {
		t, ok := p.BoxType(boxes[i].Label)
		if !ok {
			continue
		}
		clean, err := t.CheckFields(fmt.Sprintf("boxes[%d].attributes", i), in.Boxes[i].Attributes, false)
		var invalid *annotation.InvalidError
		switch {
		case errors.As(err, &invalid):
			all = append(all, invalid.Problems...)
			continue
		case err != nil:
			return nil, err
		}
		boxes[i].Attributes = clean
		if len(clean) == 0 {
			boxes[i].Attributes = nil
		}
	}
	if len(all) > 0 {
		msg := all[0].Path + ": " + all[0].Msg
		if len(all) > 1 {
			msg = fmt.Sprintf("%s (e mais %d)", msg, len(all)-1)
		}
		return nil, &annotation.InvalidError{Msg: msg, Problems: all}
	}

	// A versão é uma data em milissegundos: duas gravações no mesmo
	// milissegundo teriam a mesma versão, e a versão velha passaria. Por isso
	// a nova é sempre maior que a anterior.
	prev := set.Version()
	now := s.now()
	if !now.After(prev) {
		now = prev.Add(time.Millisecond)
	}
	set.Boxes, set.UpdatedBy, set.UpdatedAt = boxes, user, &now
	if err := s.repo.UpdateSet(ctx, &set, prev); err != nil {
		return nil, err
	}
	return &set, nil
}
