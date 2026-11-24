package boxes

import (
	"context"
	"errors"
	"slices"

	"SIDM/internal/annotation"
	"SIDM/internal/platform"
)

// Caixas que vêm de um modelo de IA (o módulo processing): a pré-anotação.
// Elas nunca passam por cima do que uma pessoa fez. O que veio do CVAT, ou
// foi editado no SIDM, fica como está.

// EnsureTask cria a tarefa se ela não existe; se existe, junta as labels
// que faltam (as outras, a descrição e as cores ficam como estão).
func (s *Service) EnsureTask(ctx context.Context, in TaskInput, user string) error {
	if err := in.normalize(); err != nil {
		return err
	}
	t, err := s.repo.FindTask(ctx, in.Name)
	if errors.Is(err, platform.ErrNotFound) {
		_, err = s.CreateTask(ctx, in, user)
		if !errors.Is(err, platform.ErrDuplicate) { // criada ao mesmo tempo por outro: junta as labels
			return err
		}
		if t, err = s.repo.FindTask(ctx, in.Name); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	var missing []string
	for _, l := range in.Labels {
		if !slices.Contains(t.Labels, l) {
			missing = append(missing, l)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return s.repo.AddTaskLabels(ctx, in.Name, missing, s.now())
}

// PutModelSets grava as caixas que o modelo tool achou, cada Set na tarefa
// dele. Uma imagem cujo Set já existe e veio de outra fonte (CVAT, outro
// modelo) ou foi editado no SIDM é pulada: volta em skipped. Os atributos
// passam pelo perfil do pacote, como no import do COCO.
func (s *Service) PutModelSets(ctx context.Context, sets []Set, tool, user string) (written int, skipped []string, err error) {
	byTask := map[string][]string{}
	for _, set := range sets {
		byTask[set.Task] = append(byTask[set.Task], set.ImageID)
	}
	current := map[string]map[string]Set{}
	for task, ids := range byTask {
		if current[task], err = s.repo.FindSets(ctx, task, ids); err != nil {
			return 0, nil, err
		}
	}

	profiles := map[string]*annotation.Profile{}
	now := s.now()
	keep := make([]Set, 0, len(sets))
	for _, set := range sets {
		if cur, ok := current[set.Task][set.ImageID]; ok && (cur.Source.Tool != tool || cur.UpdatedAt != nil) {
			skipped = append(skipped, set.ImageID)
			continue
		}
		p, ok := profiles[set.Package]
		if !ok {
			if p, err = s.profiles.ProfileOf(ctx, set.Package); err != nil {
				return 0, nil, err
			}
			profiles[set.Package] = p
		}
		set.ID = nil
		set.Boxes = fitBoxes(p, set.Boxes)
		set.Source = Source{Tool: tool, Format: "rpc", File: set.Source.File}
		set.ImportedBy, set.ImportedAt = user, now
		set.UpdatedBy, set.UpdatedAt = "", nil
		keep = append(keep, set)
	}
	if len(keep) == 0 {
		return 0, skipped, nil
	}
	if err := s.repo.ReplaceSets(ctx, keep); err != nil {
		return 0, nil, err
	}
	return len(keep), skipped, nil
}

// fitBoxes encaixa os atributos de cada caixa no tipo de objeto com caixa
// do perfil (o que o tipo não tem, ou não é válido, sai). Label sem tipo no
// perfil: os atributos ficam como vieram.
func fitBoxes(p *annotation.Profile, in []Box) []Box {
	out := make([]Box, len(in))
	copy(out, in)
	for i := range out {
		t, ok := p.BoxType(out[i].Label)
		if !ok {
			continue
		}
		clean, _ := t.FitFields(out[i].Attributes)
		out[i].Attributes = clean
		if len(clean) == 0 {
			out[i].Attributes = nil
		}
	}
	if out == nil {
		out = []Box{}
	}
	return out
}
