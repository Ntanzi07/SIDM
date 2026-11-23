package versions

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"SIDM/internal/annotation"
	"SIDM/internal/boxes"
	"SIDM/internal/images"
	"SIDM/internal/platform"
)

// Service é o módulo de versões.
type Service struct {
	repo       Repo
	images     Images
	boxes      Boxes
	annotation Annotation
	now        func() time.Time
}

func New(repo Repo, img Images, bx Boxes, ann Annotation) *Service {
	return &Service{repo: repo, images: img, boxes: bx, annotation: ann,
		now: func() time.Time { return time.Now().UTC().Truncate(time.Millisecond) }}
}

// DatasetInput é o corpo do POST/PATCH de um dataset.
type DatasetInput struct {
	Name        string `json:"name"`
	Profile     string `json:"profile"`
	Description string `json:"description"`
	Query       Query  `json:"query"`
}

// chunk: quantas imagens por consulta ao montar uma versão.
const chunk = 500

func cleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return "", &InvalidError{"informe o nome do dataset"}
	case len(name) > 80:
		return "", &InvalidError{"o nome do dataset tem no máximo 80 caracteres"}
	case strings.ContainsAny(name, "/\\?#%"):
		return "", &InvalidError{"o nome do dataset não pode ter / \\ ? # %"}
	}
	return name, nil
}

// cleanQuery tira os vazios e as repetições, e confere tudo: o perfil, as
// tarefas e os filtros (o images confere os filtros e os pacotes rodando a
// pesquisa: o mesmo erro da tela de Pesquisa).
func (s *Service) cleanQuery(ctx context.Context, profile string, q Query) (Query, error) {
	out := Query{Packages: []string{}, Filters: []Filter{}, Tasks: []string{}}
	for _, p := range q.Packages {
		if p = strings.TrimSpace(p); p != "" && !slices.Contains(out.Packages, p) {
			out.Packages = append(out.Packages, p)
		}
	}
	for _, f := range q.Filters {
		f.Key, f.Value = strings.TrimSpace(f.Key), strings.TrimSpace(f.Value)
		if f.Key == "" || f.Value == "" {
			continue
		}
		if !strings.HasPrefix(f.Key, "fields.") && !strings.HasPrefix(f.Key, "objects.") {
			return out, &InvalidError{fmt.Sprintf("filtro %q: use fields.<campo> ou objects.<tipo>.<campo>", f.Key)}
		}
		out.Filters = slices.DeleteFunc(out.Filters, func(g Filter) bool { return g.Key == f.Key })
		out.Filters = append(out.Filters, f)
	}
	if len(q.Tasks) > 0 {
		all, err := s.boxes.ListTasks(ctx)
		if err != nil {
			return out, err
		}
		for _, t := range q.Tasks {
			t = strings.TrimSpace(t)
			if t == "" || slices.Contains(out.Tasks, t) {
				continue
			}
			if !slices.ContainsFunc(all, func(x boxes.Task) bool { return x.Name == t }) {
				return out, &InvalidError{fmt.Sprintf("a tarefa %q não existe", t)}
			}
			out.Tasks = append(out.Tasks, t)
		}
	}
	if _, err := s.annotation.FindProfile(ctx, profile); errors.Is(err, platform.ErrNotFound) {
		return out, &InvalidError{fmt.Sprintf("o perfil de anotação %q não existe", profile)}
	} else if err != nil {
		return out, err
	}
	if _, err := s.images.SearchIDs(ctx, out.values(profile)); err != nil {
		return out, err
	}
	return out, nil
}

func (s *Service) CreateDataset(ctx context.Context, in DatasetInput, user string) (*Dataset, error) {
	name, err := cleanName(in.Name)
	if err != nil {
		return nil, err
	}
	profile := strings.TrimSpace(in.Profile)
	if profile == "" {
		return nil, &InvalidError{"informe o perfil de anotação do dataset"}
	}
	q, err := s.cleanQuery(ctx, profile, in.Query)
	if err != nil {
		return nil, err
	}
	now := s.now()
	d := &Dataset{Name: name, Profile: profile, Description: strings.TrimSpace(in.Description), Query: q,
		CreatedBy: user, CreatedAt: now, UpdatedAt: now}
	if err := s.repo.CreateDataset(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *Service) FindDataset(ctx context.Context, name string) (*Dataset, error) {
	d, err := s.repo.FindDataset(ctx, name)
	if err != nil {
		return nil, err
	}
	if err := s.fillLatest(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// ListDatasets: todos, com a versão mais nova de cada um.
func (s *Service) ListDatasets(ctx context.Context) ([]Dataset, error) {
	list, err := s.repo.ListDatasets(ctx)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if err := s.fillLatest(ctx, &list[i]); err != nil {
			return nil, err
		}
	}
	return list, nil
}

func (s *Service) fillLatest(ctx context.Context, d *Dataset) error {
	v, err := s.repo.LastVersion(ctx, d.Name)
	if err != nil {
		return err
	}
	if v != nil {
		d.Latest = v.info()
	}
	return nil
}

// UpdateDataset troca a descrição, o perfil e a query (o nome não muda: as
// versões apontam para ele). As versões já criadas não mudam.
func (s *Service) UpdateDataset(ctx context.Context, name string, in DatasetInput) (*Dataset, error) {
	d, err := s.repo.FindDataset(ctx, name)
	if err != nil {
		return nil, err
	}
	if n := strings.TrimSpace(in.Name); n != "" && n != d.Name {
		return nil, &InvalidError{"o nome de um dataset não muda (as versões apontam para ele)"}
	}
	if p := strings.TrimSpace(in.Profile); p != "" {
		d.Profile = p
	}
	q, err := s.cleanQuery(ctx, d.Profile, in.Query)
	if err != nil {
		return nil, err
	}
	d.Description, d.Query, d.UpdatedAt = strings.TrimSpace(in.Description), q, s.now()
	if err := s.repo.ReplaceDataset(ctx, d); err != nil {
		return nil, err
	}
	if err := s.fillLatest(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// DeleteDataset: só sem versões (uma versão pode ter treinado um modelo).
func (s *Service) DeleteDataset(ctx context.Context, name string) error {
	v, err := s.repo.LastVersion(ctx, name)
	if err != nil {
		return err
	}
	if v != nil {
		return &ConflictError{fmt.Sprintf("o dataset %q tem versões e não pode ser apagado", name)}
	}
	return s.repo.DeleteDataset(ctx, name)
}

// CreateVersion congela o dataset como está agora: as imagens verificadas que
// passam na query, com a anotação e as caixas de cada uma, e o resumo. O
// número é o próximo do dataset.
func (s *Service) CreateVersion(ctx context.Context, name, message, user string) (*Version, error) {
	d, err := s.repo.FindDataset(ctx, name)
	if err != nil {
		return nil, err
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return nil, &InvalidError{"escreva uma mensagem para a versão (o que mudou)"}
	}
	p, err := s.annotation.FindProfile(ctx, d.Profile)
	if err != nil {
		return nil, fmt.Errorf("perfil de anotação %q: %w", d.Profile, err)
	}
	ids, err := s.images.SearchIDs(ctx, d.Query.values(d.Profile))
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, &InvalidError{"nenhuma imagem verificada passa nos filtros do dataset"}
	}

	// as tarefas: as da query, ou todas
	all, err := s.boxes.ListTasks(ctx)
	if err != nil {
		return nil, err
	}
	tasks := d.Query.Tasks
	if len(tasks) == 0 {
		for _, t := range all {
			tasks = append(tasks, t.Name)
		}
		slices.Sort(tasks)
	}

	items := make([]Item, 0, len(ids))
	var imgs []*images.Image
	var sets []*boxes.Set
	for start := 0; start < len(ids); start += chunk {
		part := ids[start:min(start+chunk, len(ids))]
		// verificadas de novo: alguém pode ter reaberto uma no meio
		list, _, err := s.images.List(ctx, images.ImageFilter{IDs: part, Status: images.StatusVerified, Limit: int64(len(part))})
		if err != nil {
			return nil, err
		}
		byTask := map[string]map[string]boxes.Set{}
		for _, t := range tasks {
			if byTask[t], err = s.boxes.FindSets(ctx, t, part); err != nil {
				return nil, err
			}
		}
		for i := range list {
			img := &list[i]
			it := Item{SHA1: img.SHA1, Package: img.Package, FileName: img.FileName, OriginalName: img.OriginalName, Ext: img.Ext,
				Fields: img.Fields, Objects: img.Objects, Boxes: map[string]TaskSet{}}
			if it.FileName == "" {
				it.FileName = img.OriginalName
			}
			if it.Objects == nil {
				it.Objects = []annotation.Object{}
			}
			for _, t := range tasks {
				if set, ok := byTask[t][img.SHA1]; ok {
					it.Boxes[t] = TaskSet{Width: set.Width, Height: set.Height, Boxes: set.Boxes}
					sets = append(sets, &set)
				}
			}
			it.Hash = hashOf(&it)
			items = append(items, it)
			imgs = append(imgs, img)
		}
	}
	if len(items) == 0 {
		return nil, &InvalidError{"nenhuma imagem verificada passa nos filtros do dataset"}
	}
	slices.SortFunc(items, func(a, b Item) int { return strings.Compare(a.SHA1, b.SHA1) })

	summary := Summary{Images: *images.CountStats(imgs, images.SpecOf(p)), Labels: labelsOf(tasks, all, sets)}
	summary.Images.Profile = p.Name

	// O número: o próximo. Se outra pessoa criar ao mesmo tempo, o índice
	// único (dataset, número) recusa a segunda, que tenta o seguinte.
	for range 5 {
		last, err := s.repo.LastVersion(ctx, d.Name)
		if err != nil {
			return nil, err
		}
		v := &Version{ID: newID(), Dataset: d.Name, Number: 1, Message: message, CreatedBy: user, CreatedAt: s.now(),
			Profile: d.Profile, Query: d.Query, Tasks: tasks, Summary: summary}
		if last != nil {
			v.Number = last.Number + 1
		}
		for i := range items {
			items[i].Version = v.ID
		}
		// os itens antes da versão: quem lê só enxerga a versão pronta
		for start := 0; start < len(items); start += 1000 {
			if err := s.repo.InsertItems(ctx, items[start:min(start+1000, len(items))]); err != nil {
				_ = s.repo.DeleteItems(ctx, v.ID)
				return nil, err
			}
		}
		err = s.repo.InsertVersion(ctx, v)
		if errors.Is(err, platform.ErrDuplicate) {
			if derr := s.repo.DeleteItems(ctx, v.ID); derr != nil {
				return nil, derr
			}
			continue
		}
		if err != nil {
			_ = s.repo.DeleteItems(ctx, v.ID)
			return nil, err
		}
		return v, nil
	}
	return nil, &ConflictError{"muitas versões criadas ao mesmo tempo; tente de novo"}
}

// labelsOf: as caixas por tarefa, na ordem das tarefas, com as labels da
// tarefa que não aparecem com zero (como o GET /boxes/stats).
func labelsOf(tasks []string, all []boxes.Task, sets []*boxes.Set) []boxes.TaskStats {
	counted := boxes.CountLabels(sets)
	out := make([]boxes.TaskStats, 0, len(tasks))
	for _, name := range tasks {
		ts := boxes.TaskStats{Task: name, Labels: map[string]boxes.LabelCount{}}
		if c := counted[name]; c != nil {
			ts = *c
		}
		for _, t := range all {
			if t.Name != name {
				continue
			}
			for _, l := range t.Labels {
				if _, ok := ts.Labels[l]; !ok {
					ts.Labels[l] = boxes.LabelCount{}
				}
			}
		}
		out = append(out, ts)
	}
	return out
}

// hashOf: o sha1 da anotação. O json ordena as chaves dos mapas, então a
// mesma anotação dá sempre o mesmo hash.
func hashOf(it *Item) string {
	data, _ := json.Marshal(struct {
		Fields  map[string]any     `json:"f"`
		Objects any                `json:"o"`
		Boxes   map[string]TaskSet `json:"b"`
	}{it.Fields, it.Objects, it.Boxes})
	sum := sha1.Sum(data)
	return hex.EncodeToString(sum[:])
}

func newID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Service) ListVersions(ctx context.Context, name string) ([]Version, error) {
	if _, err := s.repo.FindDataset(ctx, name); err != nil {
		return nil, err
	}
	return s.repo.ListVersions(ctx, name)
}

func (s *Service) FindVersion(ctx context.Context, name string, number int) (*Version, error) {
	return s.repo.FindVersion(ctx, name, number)
}

func (s *Service) Items(ctx context.Context, name string, number int, limit, skip int64) ([]Item, int64, error) {
	v, err := s.repo.FindVersion(ctx, name, number)
	if err != nil {
		return nil, 0, err
	}
	return s.repo.ListItems(ctx, v.ID, limit, skip)
}
