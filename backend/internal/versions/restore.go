package versions

// A volta: uma imagem volta para como estava numa versão (os campos, os
// objetos e as caixas de cada tarefa da versão). Se ela foi apagada depois,
// sai da lixeira. A versão não muda: a volta mexe só na imagem de hoje (e a
// próxima versão registra a mudança).

import (
	"context"
	"fmt"

	"SIDM/internal/boxes"
	"SIDM/internal/images"
)

// RestoreResult: o que a volta fez.
type RestoreResult struct {
	SHA1      string `json:"sha1"`
	Version   int    `json:"version"`
	FromTrash bool   `json:"from_trash"` // estava apagada e saiu da lixeira
	Package   string `json:"package"`
	FileName  string `json:"file_name"`
}

// RestoreItem volta a imagem sha1 para a versão number do dataset.
func (s *Service) RestoreItem(ctx context.Context, name string, number int, sha1, user string) (*RestoreResult, error) {
	v, err := s.restoreVersion(ctx, name, number)
	if err != nil {
		return nil, err
	}
	return s.restoreOne(ctx, v, sha1, changedBy(user, v))
}

func (s *Service) restoreVersion(ctx context.Context, name string, number int) (*Version, error) {
	if number == Now {
		return nil, &InvalidError{"a volta é para uma versão salva (o estado atual já é o de hoje)"}
	}
	return s.repo.FindVersion(ctx, name, number)
}

func changedBy(user string, v *Version) string {
	return fmt.Sprintf("%s via volta para a v%d de %s", user, v.Number, v.Dataset)
}

func (s *Service) restoreOne(ctx context.Context, v *Version, sha1, changedBy string) (*RestoreResult, error) {
	it, err := s.repo.FindItem(ctx, v.ID, sha1)
	if err != nil {
		return nil, err
	}
	img, fromTrash, err := s.images.Restore(ctx, sha1, images.RestoreInput{Fields: normalized(it.Fields), Objects: normalizedObjects(it.Objects)}, changedBy)
	if err != nil {
		return nil, err
	}
	frozen := make(map[string]boxes.Set, len(it.Boxes))
	for task, set := range it.Boxes {
		frozen[task] = boxes.Set{Width: set.Width, Height: set.Height, Boxes: set.Boxes}
	}
	if err := s.boxes.RestoreSets(ctx, sha1, img.Package, v.Tasks, frozen, changedBy); err != nil {
		return nil, fmt.Errorf("a anotação voltou, mas as caixas não: %w", err)
	}
	return &RestoreResult{SHA1: sha1, Version: v.Number, FromTrash: fromTrash, Package: img.Package, FileName: img.FileName}, nil
}

// RestoreReport: o que a volta de tudo fez.
type RestoreReport struct {
	Version   int              `json:"version"`
	Restored  int              `json:"restored"`   // voltaram
	FromTrash int              `json:"from_trash"` // dessas, as que saíram da lixeira
	Failed    []RestoreFailure `json:"failed"`     // as que não deu (as outras voltaram mesmo assim)
	Added     int              `json:"added"`      // entraram depois da versão: não mudam
}

type RestoreFailure struct {
	SHA1  string `json:"sha1"`
	Name  string `json:"name"`
	Error string `json:"error"`
}

// RestoreAll volta para a versão number todas as imagens que mudaram desde
// ela: as alteradas e as que saíram (apagadas, reabertas ou que deixaram de
// passar na pesquisa). As que entraram depois não mudam. Uma que falha não
// para as outras: vai para Failed.
func (s *Service) RestoreAll(ctx context.Context, name string, number int, user string) (*RestoreReport, error) {
	v, err := s.restoreVersion(ctx, name, number)
	if err != nil {
		return nil, err
	}
	d, err := s.Diff(ctx, name, number, Now)
	if err != nil {
		return nil, err
	}
	rep := &RestoreReport{Version: v.Number, Failed: []RestoreFailure{}, Added: len(d.Added)}
	by := changedBy(user, v)
	for _, ref := range append(d.Changed, d.Removed...) {
		res, err := s.restoreOne(ctx, v, ref.SHA1, by)
		if err != nil {
			if ctx.Err() != nil {
				return rep, ctx.Err()
			}
			name := ref.OriginalName
			if name == "" {
				name = ref.FileName
			}
			rep.Failed = append(rep.Failed, RestoreFailure{SHA1: ref.SHA1, Name: name, Error: err.Error()})
			continue
		}
		rep.Restored++
		if res.FromTrash {
			rep.FromTrash++
		}
	}
	return rep, nil
}
