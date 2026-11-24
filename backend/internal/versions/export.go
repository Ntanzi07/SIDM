package versions

// O export de uma versão: as imagens com a anotação CONGELADA (os campos, os
// objetos e as caixas de cada tarefa, como estavam ao criar a versão) e o
// arquivo de hoje, que é o mesmo (a imagem no disco não muda). O formato do
// zip é o do módulo exports; aqui só sai o que entra nele.

import (
	"context"

	"SIDM/internal/boxes"
	"SIDM/internal/images"
)

// Frozen: uma versão pronta para o export.
type Frozen struct {
	Version *Version
	// Images: uma por item, com a anotação congelada. O Path é o da imagem
	// hoje (um pacote renomeado muda a pasta); de uma imagem apagada depois,
	// um caminho que não existe (o export a lista no FALTANDO.txt).
	Images []images.Image
	// Boxes: tarefa → sha1 → as caixas congeladas.
	Boxes map[string]map[string]boxes.Set
}

// Frozen monta uma versão (ou o "agora", com Now) para o export.
func (s *Service) Frozen(ctx context.Context, name string, number int) (*Frozen, error) {
	var v *Version
	var err error
	if number == Now {
		v, err = s.Current(ctx, name)
	} else {
		v, err = s.repo.FindVersion(ctx, name, number)
	}
	if err != nil {
		return nil, err
	}
	fz := &Frozen{Version: v, Boxes: map[string]map[string]boxes.Set{}}
	for _, t := range v.Tasks {
		fz.Boxes[t] = map[string]boxes.Set{}
	}
	for skip := int64(0); ; skip += itemsPage {
		items := v.items
		if v.Number != 0 {
			if items, _, err = s.repo.ListItems(ctx, v.ID, itemsPage, skip); err != nil {
				return nil, err
			}
		}
		paths, err := s.paths(ctx, items)
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			p, ok := paths[it.SHA1]
			if !ok {
				p = "apagada/" + it.SHA1 + it.Ext // não existe: vai para o FALTANDO.txt
			}
			fz.Images = append(fz.Images, images.Image{
				SHA1: it.SHA1, Package: it.Package, Path: p, Ext: it.Ext,
				FileName: it.FileName, OriginalName: it.OriginalName, Status: images.StatusVerified,
				Fields: normalized(it.Fields), Objects: normalizedObjects(it.Objects),
			})
			for t, set := range it.Boxes {
				if byImage, ok := fz.Boxes[t]; ok {
					byImage[it.SHA1] = boxes.Set{Task: t, ImageID: it.SHA1, Package: it.Package, Width: set.Width, Height: set.Height, Boxes: set.Boxes}
				}
			}
		}
		if v.Number == 0 || len(items) < itemsPage {
			break
		}
	}
	return fz, nil
}

// paths: onde está o arquivo de cada imagem hoje (sha1 → caminho). Só o
// caminho vem da imagem atual; a anotação é a congelada.
func (s *Service) paths(ctx context.Context, items []Item) (map[string]string, error) {
	out := make(map[string]string, len(items))
	const chunk = 500
	for start := 0; start < len(items); start += chunk {
		ids := make([]string, 0, chunk)
		for _, it := range items[start:min(start+chunk, len(items))] {
			ids = append(ids, it.SHA1)
		}
		imgs, _, err := s.images.List(ctx, images.ImageFilter{IDs: ids, Limit: int64(len(ids))})
		if err != nil {
			return nil, err
		}
		for _, img := range imgs {
			out[img.SHA1] = img.Path
		}
	}
	return out, nil
}

// ParseNumber: o número de uma versão como nas rotas ("3", "v3"; "atual" ou
// "agora" = Now).
func ParseNumber(raw string) (int, bool) { return parseNumber(raw) }
