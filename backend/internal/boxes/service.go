package boxes

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"SIDM/internal/platform"
)

// Service são as regras do módulo. O export usa FindTask e FindSets; as
// imagens avisam com DeleteImage; o catálogo avisa pelo PackageListener.
type Service struct {
	repo      Repo
	images    Images
	files     Files
	packages  Packages
	profiles  Profiles
	listeners []TaskListener
	now       func() time.Time
}

func New(repo Repo, images Images, files Files, packages Packages, profiles Profiles, listeners ...TaskListener) *Service {
	return &Service{repo: repo, images: images, files: files, packages: packages, profiles: profiles, listeners: listeners,
		now: func() time.Time { return time.Now().UTC() }}
}

// Listen inscreve mais um interessado na renomeação de tarefas (para quem é
// criado depois do boxes, como o export, que também lê as caixas).
func (s *Service) Listen(l TaskListener) { s.listeners = append(s.listeners, l) }

// ---------- tarefas ----------

var taskNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

const (
	maxTaskLabels   = 500
	maxTaskLabelLen = 64
)

// TaskInput é o que o cliente manda para criar/atualizar uma tarefa.
type TaskInput struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Labels      []string `json:"labels"`
}

// InvalidError: o pedido não passa na validação (vira 400).
type InvalidError struct{ Msg string }

func (e *InvalidError) Error() string { return e.Msg }

// normalize limpa e valida a entrada.
func (in *TaskInput) normalize() error {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	if !taskNameRe.MatchString(in.Name) {
		return &InvalidError{"nome da tarefa: de 1 a 64 caracteres, só letras minúsculas, números, - e _ (começando por letra ou número)"}
	}
	labels, err := normalizeLabels(in.Labels)
	if err != nil {
		return err
	}
	in.Labels = labels
	return nil
}

// normalizeLabels: sem espaços nas pontas, sem vazias, sem repetidas (a
// ordem é mantida: é a ordem em que aparecem na tela).
func normalizeLabels(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, l := range in {
		l = strings.TrimSpace(l)
		if l == "" || seen[l] {
			continue
		}
		if len([]rune(l)) > maxTaskLabelLen {
			return nil, &InvalidError{fmt.Sprintf("label %q passa de %d caracteres", l, maxTaskLabelLen)}
		}
		seen[l] = true
		out = append(out, l)
	}
	if len(out) > maxTaskLabels {
		return nil, &InvalidError{fmt.Sprintf("no máximo %d labels por tarefa", maxTaskLabels)}
	}
	return out, nil
}

// CreateTask valida e cadastra. platform.ErrDuplicate se o nome existe.
func (s *Service) CreateTask(ctx context.Context, in TaskInput, user string) (*Task, error) {
	if err := in.normalize(); err != nil {
		return nil, err
	}
	now := s.now()
	t := &Task{Name: in.Name, Description: in.Description, Labels: in.Labels, CreatedBy: user, CreatedAt: now, UpdatedAt: now}
	if err := s.repo.CreateTask(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// FindTask busca uma tarefa (platform.ErrNotFound se não existe).
func (s *Service) FindTask(ctx context.Context, name string) (*Task, error) {
	return s.repo.FindTask(ctx, name)
}

// ListTasks: todas, por nome.
func (s *Service) ListTasks(ctx context.Context) ([]Task, error) {
	return s.repo.ListTasks(ctx)
}

// UpdateTask grava a tarefa no lugar de oldName. Se o nome mudou, leva junto
// as caixas e avisa quem guarda a tarefa (os perfis de export). Sem
// transação (o Mongo do projeto roda sem réplica), então a tarefa é a
// ÚLTIMA a mudar: se algo falhar no meio, ela ainda tem o nome antigo e
// repetir o rename termina o serviço.
func (s *Service) UpdateTask(ctx context.Context, oldName string, in TaskInput) (*Task, error) {
	old, err := s.repo.FindTask(ctx, oldName)
	if err != nil {
		return nil, err
	}
	if err := in.normalize(); err != nil {
		return nil, err
	}
	t := &Task{Name: in.Name, Description: in.Description, Labels: in.Labels,
		CreatedBy: old.CreatedBy, CreatedAt: old.CreatedAt, UpdatedAt: s.now()}
	if t.Name != oldName {
		if _, err := s.repo.FindTask(ctx, t.Name); err == nil {
			return nil, platform.ErrDuplicate
		} else if !errors.Is(err, platform.ErrNotFound) {
			return nil, err
		}
		if err := s.repo.RenameTaskSets(ctx, oldName, t.Name); err != nil {
			return nil, err
		}
		for _, l := range s.listeners {
			if err := l.TaskRenamed(ctx, oldName, t.Name); err != nil {
				return nil, err
			}
		}
	}
	if err := s.repo.ReplaceTask(ctx, oldName, t); err != nil {
		return nil, err
	}
	return t, nil
}

// DeleteTask apaga a tarefa e todas as caixas dela. Devolve quantas imagens
// tinham caixas nela. Os perfis de export que apontam para ela ficam (a
// prévia avisa que a tarefa não existe).
func (s *Service) DeleteTask(ctx context.Context, name string) (int64, error) {
	if _, err := s.repo.FindTask(ctx, name); err != nil {
		return 0, err
	}
	// Caixas primeiro: se falhar, a tarefa continua e dá para repetir.
	n, err := s.repo.DeleteTaskSets(ctx, name)
	if err != nil {
		return 0, err
	}
	if err := s.repo.DeleteTask(ctx, name); err != nil && !errors.Is(err, platform.ErrNotFound) {
		return n, err
	}
	return n, nil
}

// Stats conta as imagens da tarefa por pacote (com quantas estão
// verificadas) e as caixas por label.
func (s *Service) Stats(ctx context.Context, task string) (*Stats, error) {
	ti, err := s.repo.TaskImages(ctx, task)
	if err != nil {
		return nil, err
	}
	st := &Stats{Negatives: ti.Negatives, Labels: ti.Labels, Packages: make([]TaskPackage, 0, len(ti.IDs))}
	for _, n := range ti.Labels {
		st.Boxes += n
	}
	for pkg, ids := range ti.IDs {
		verified, err := s.images.Verified(ctx, ids)
		if err != nil {
			return nil, err
		}
		p := TaskPackage{Name: pkg, Images: int64(len(ids))}
		for _, id := range ids {
			if verified[id] {
				p.Verified++
			}
		}
		st.Packages = append(st.Packages, p)
		st.Images += p.Images
		st.Verified += p.Verified
	}
	sort.Slice(st.Packages, func(i, j int) bool { return st.Packages[i].Name < st.Packages[j].Name })
	return st, nil
}

// ---------- caixas ----------

// FindSets: as caixas de várias imagens numa tarefa (sha1 -> caixas).
func (s *Service) FindSets(ctx context.Context, task string, ids []string) (map[string]Set, error) {
	return s.repo.FindSets(ctx, task, ids)
}

// ImageSets: as caixas de uma imagem em todas as tarefas.
func (s *Service) ImageSets(ctx context.Context, sha1 string) ([]Set, error) {
	return s.repo.FindImageSets(ctx, sha1)
}

// DeleteImage apaga as caixas de uma imagem em todas as tarefas (quando ela
// é excluída).
func (s *Service) DeleteImage(ctx context.Context, sha1 string) error {
	return s.repo.DeleteImageSets(ctx, sha1)
}

// PackageCounts: por tarefa, as imagens do pacote anotadas nela.
func (s *Service) PackageCounts(ctx context.Context, pkg string) ([]PackageCount, error) {
	return s.repo.PackageCounts(ctx, pkg)
}

// PackageRenamed e PackageDeleted: o catálogo avisa (catalog.PackageListener),
// porque as caixas guardam o pacote de cada imagem.
func (s *Service) PackageRenamed(ctx context.Context, oldName, newName string) error {
	return s.repo.RenamePackageSets(ctx, oldName, newName)
}

func (s *Service) PackageDeleted(ctx context.Context, name string) error {
	return s.repo.DeletePackageSets(ctx, name)
}
