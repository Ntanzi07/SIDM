package images

// A volta de uma imagem para como estava numa versão (o módulo versions
// chama): a anotação congelada volta para a imagem; se ela foi apagada, sai
// antes da lixeira.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"SIDM/internal/annotation"
	"SIDM/internal/files"
	"SIDM/internal/platform"
)

// RestoreInput: a anotação para onde a imagem volta.
type RestoreInput struct {
	Fields  map[string]any
	Objects []annotation.Object
}

// RestoreError: a volta não dá (vira 400 em quem chamou). Embrulha o
// platform.ErrInvalid.
type RestoreError struct{ Msg string }

func (e *RestoreError) Error() string { return e.Msg }
func (e *RestoreError) Unwrap() error { return platform.ErrInvalid }

// Restore volta a imagem: se ela não existe mais, tira da lixeira; depois
// grava a anotação dada como verificada (passando pelo perfil do pacote,
// como uma anotação normal). changedBy vai para o histórico. Devolve a
// imagem e se ela veio da lixeira. ErrConflict: outra pessoa está anotando.
func (s *Service) Restore(ctx context.Context, sha1 string, in RestoreInput, changedBy string) (*Image, bool, error) {
	img, err := s.repo.FindImage(ctx, sha1)
	fromTrash := false
	if errors.Is(err, platform.ErrNotFound) {
		if img, err = s.untrash(ctx, sha1); err != nil {
			return nil, false, err
		}
		fromTrash = true
	} else if err != nil {
		return nil, false, err
	}
	if img.ClaimedByOther(changedBy, time.Now().UTC()) {
		return nil, fromTrash, fmt.Errorf("imagem está sendo anotada por %s: %w", img.ClaimedBy, ErrConflict)
	}

	profile, err := s.ProfileOf(ctx, img.Package)
	if err != nil {
		return nil, fromTrash, err
	}
	clean, err := profile.Check(annotation.Values{Fields: in.Fields, Objects: in.Objects}, true)
	var invalid *annotation.InvalidError
	if errors.As(err, &invalid) {
		return nil, fromTrash, &RestoreError{fmt.Sprintf("a anotação da versão não passa mais no perfil %q (ele mudou depois): %s", profile.Name, invalid.Msg)}
	}
	if err != nil {
		return nil, fromTrash, err
	}

	before := *img
	now := s.now()
	img.Fields, img.Objects = clean.Fields, clean.Objects
	name, version, msg := s.finalName(ctx, img, profile)
	if msg != "" {
		return nil, fromTrash, &RestoreError{"a anotação da versão não fecha mais: " + msg}
	}
	img.FileName, img.PatternVersion, img.Status = name, version, StatusVerified
	img.ClaimedBy, img.ClaimedUntil = "", nil
	img.UpdatedAt = now
	if err := s.repo.ReplaceImage(ctx, img, before.UpdatedAt); err != nil {
		return nil, fromTrash, err
	}
	if err := s.repo.InsertHistory(ctx, DiffHistory(&before, img, changedBy)); err != nil {
		platform.LogError(err)
	}
	img.ensureValues()
	return img, fromTrash, nil
}

// untrash tira a imagem da lixeira: o arquivo volta para a pasta do pacote
// e o registro volta para images.
func (s *Service) untrash(ctx context.Context, sha1 string) (*Image, error) {
	t, err := s.repo.FindTrash(ctx, sha1)
	if errors.Is(err, platform.ErrNotFound) {
		return nil, &RestoreError{"a imagem foi apagada antes de existir a lixeira: o arquivo não existe mais e ela não dá para voltar"}
	}
	if err != nil {
		return nil, err
	}
	if _, err := s.catalog.FindPackage(ctx, t.Package); errors.Is(err, platform.ErrNotFound) {
		return nil, &RestoreError{fmt.Sprintf("o pacote %q da imagem não existe mais (foi renomeado ou apagado)", t.Package)}
	} else if err != nil {
		return nil, err
	}

	img := t.Image
	dest := files.PendingPath(img.Package, img.SHA1, img.Ext)
	switch trash := files.TrashPath(img.SHA1, img.Ext); {
	case s.files.Exists(trash):
		if err := s.files.MoveTo(trash, dest); err != nil {
			return nil, err
		}
	case s.files.Exists(img.Path): // o arquivo não chegou a ir para a lixeira
		dest = img.Path
	default:
		return nil, &RestoreError{"o arquivo da imagem não está na lixeira (" + trash + ")"}
	}

	now := s.now()
	img.Path = dest
	img.ClaimedBy, img.ClaimedUntil = "", nil
	img.QueuedAt, img.UpdatedAt = now, now
	if err := s.repo.InsertImage(ctx, &img); err != nil {
		return nil, err
	}
	if err := s.repo.DeleteTrash(ctx, sha1); err != nil {
		platform.LogError(err) // a imagem já voltou; sobra só o registro na lixeira
	}
	return &img, nil
}
