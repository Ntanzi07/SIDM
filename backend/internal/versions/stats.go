package versions

// A pesquisa dentro de uma versão: as mesmas contas da Pesquisa (campos,
// objetos, caixas por label), mas sobre os itens congelados, com os filtros
// da query (?package=&fields.x=&objects.t.k=&task=).

import (
	"context"
	"slices"

	"SIDM/internal/annotation"
	"SIDM/internal/boxes"
	"SIDM/internal/images"
)

type VersionStats struct {
	Images *images.Stats     `json:"images"`
	Labels *boxes.LabelStats `json:"labels"`
}

const itemsPage = 1000

func (s *Service) VersionStats(ctx context.Context, name string, number int, q map[string][]string) (*VersionStats, error) {
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
	profile, err := s.annotation.FindProfile(ctx, v.Profile)
	if err != nil {
		return nil, err
	}
	// a versão só tem verificadas: o status da query não vale aqui
	clean := map[string][]string{}
	for k, vals := range q {
		if k != "status" && k != "task" {
			clean[k] = vals
		}
	}
	f, err := images.ParseFilter(clean, profile)
	if err != nil {
		return nil, err
	}

	tasks := v.Tasks
	if asked := q["task"]; len(asked) > 0 {
		tasks = nil
		for _, t := range asked {
			if slices.Contains(v.Tasks, t) && !slices.Contains(tasks, t) {
				tasks = append(tasks, t)
			}
		}
	}

	var imgs []*images.Image
	var sets []*boxes.Set
	for skip := int64(0); ; skip += itemsPage {
		items := v.items
		if v.Number != 0 {
			if items, _, err = s.repo.ListItems(ctx, v.ID, itemsPage, skip); err != nil {
				return nil, err
			}
		}
		for _, it := range items {
			img := &images.Image{SHA1: it.SHA1, Package: it.Package, Status: images.StatusVerified, Fields: normalized(it.Fields), Objects: normalizedObjects(it.Objects)}
			if !f.Matches(img) {
				continue
			}
			imgs = append(imgs, img)
			for _, t := range tasks {
				if set, ok := it.Boxes[t]; ok {
					sets = append(sets, &boxes.Set{Task: t, ImageID: it.SHA1, Package: it.Package, Width: set.Width, Height: set.Height, Boxes: set.Boxes})
				}
			}
		}
		if v.Number == 0 || len(items) < itemsPage {
			break
		}
	}

	labels, err := s.labelStats(ctx, tasks, sets, int64(len(imgs)))
	if err != nil {
		return nil, err
	}
	return &VersionStats{Images: images.CountStats(imgs, images.SpecOf(profile)), Labels: labels}, nil
}

// Os itens lidos do Mongo vêm com os tipos do bson (bson.A, int32): os
// filtros e as contas esperam os do resto ([]string, float64).
func normalized(fields map[string]any) map[string]any {
	out := make(map[string]any, len(fields))
	for k, v := range fields {
		out[k] = images.NormalizeValue(v)
	}
	return out
}

func normalizedObjects(objs []annotation.Object) []annotation.Object {
	out := make([]annotation.Object, len(objs))
	for i, o := range objs {
		out[i] = annotation.Object{Type: o.Type, Fields: normalized(o.Fields)}
	}
	return out
}

// labelStats: as caixas contadas, na ordem das tarefas, com as labels da
// tarefa que não aparecem (zero), como o /boxes/stats.
func (s *Service) labelStats(ctx context.Context, tasks []string, sets []*boxes.Set, total int64) (*boxes.LabelStats, error) {
	defs, err := s.boxes.ListTasks(ctx)
	if err != nil {
		return nil, err
	}
	counted := boxes.CountLabels(sets)
	out := &boxes.LabelStats{Images: total, Tasks: []boxes.TaskStats{}}
	for _, t := range tasks {
		ts := counted[t]
		if ts == nil {
			ts = &boxes.TaskStats{Task: t, Labels: map[string]boxes.LabelCount{}}
		}
		for _, d := range defs {
			if d.Name != t {
				continue
			}
			for _, l := range d.Labels {
				if _, ok := ts.Labels[l]; !ok {
					ts.Labels[l] = boxes.LabelCount{}
				}
			}
		}
		out.Tasks = append(out.Tasks, *ts)
	}
	return out, nil
}
