package images

import (
	"context"
	"errors"
	"time"

	"SIDM/internal/annotation"
)

// LinkBoxes liga as caixas de uma tarefa aos objetos da imagem
// (annotation.Profile.LinkBoxes): chamado pelo import das caixas, para não
// precisar ligar imagem por imagem. Numa verificada só se liga caixa (nada
// muda de valor, o nome v2 continua o mesmo); nas outras também cria os
// objetos. Imagem que alguém está anotando fica de fora (Busy): a pessoa
// salvaria por cima.
func (s *Service) LinkBoxes(ctx context.Context, sha1, task string, boxes []annotation.SeenBox, user string) (annotation.LinkResult, error) {
	img, err := s.repo.FindImage(ctx, sha1)
	if err != nil {
		return annotation.LinkResult{}, err
	}
	p, err := s.ProfileOf(ctx, img.Package)
	if err != nil {
		return annotation.LinkResult{}, err
	}
	objects, res := p.LinkBoxes(img.Objects, task, boxes, img.Status != StatusVerified)
	if !res.Changed() {
		return res, nil
	}
	if img.ClaimedByOther("", time.Now().UTC()) {
		return annotation.LinkResult{Busy: 1, Loose: res.Loose + res.Linked + res.Created}, nil
	}
	before := *img
	img.Objects = objects
	img.UpdatedAt = s.now()
	if err := s.repo.ReplaceImage(ctx, img, before.UpdatedAt); errors.Is(err, ErrConflict) {
		return annotation.LinkResult{Busy: 1, Loose: res.Loose + res.Linked + res.Created}, nil
	} else if err != nil {
		return annotation.LinkResult{}, err
	}
	if err := s.repo.InsertHistory(ctx, DiffHistory(&before, img, user)); err != nil {
		return res, err
	}
	res.Images = 1
	return res, nil
}
