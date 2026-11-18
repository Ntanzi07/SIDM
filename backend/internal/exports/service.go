package exports

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"SIDM/internal/export"
	"SIDM/internal/images"
	"SIDM/internal/platform"
)

type Service struct {
	repo    Repo
	images  Images
	boxes   Boxes
	catalog Catalog
	files   Files
	now     func() time.Time
}

func New(repo Repo, img Images, bx Boxes, cat Catalog, fs Files) *Service {
	return &Service{repo: repo, images: img, boxes: bx, catalog: cat, files: fs,
		now: func() time.Time { return time.Now().UTC() }}
}

// TaskRenamed: os perfis de export guardam a tarefa (boxes.TaskListener).
func (s *Service) TaskRenamed(ctx context.Context, oldName, newName string) error {
	return s.repo.TaskRenamed(ctx, oldName, newName)
}

// ---------- perfis ----------

// ListProfiles: os padrões e depois os salvos, por nome.
func (s *Service) ListProfiles(ctx context.Context) ([]export.Profile, error) {
	saved, err := s.repo.ListExportProfiles(ctx)
	if err != nil {
		return nil, err
	}
	return append(export.Builtins(), saved...), nil
}

// FindProfile busca um padrão ou um salvo (platform.ErrNotFound se nenhum).
func (s *Service) FindProfile(ctx context.Context, name string) (*export.Profile, error) {
	if p, ok := export.Builtin(name); ok {
		return &p, nil
	}
	return s.repo.FindExportProfile(ctx, name)
}

// Validate limpa e valida um perfil que veio de fora.
func Validate(p *export.Profile) error {
	p.Builtin = false // só o servidor marca perfil padrão
	p.Normalize()
	if err := p.Validate(); err != nil {
		return &InvalidError{err.Error()}
	}
	return nil
}

// CreateProfile cadastra um perfil já validado (platform.ErrDuplicate se o
// nome já existe).
func (s *Service) CreateProfile(ctx context.Context, p *export.Profile, user string) error {
	p.CreatedBy, p.UpdatedAt = user, s.now()
	return s.repo.CreateExportProfile(ctx, p)
}

// Editable: os padrões não podem ser alterados nem apagados.
func Editable(name, action string) error {
	if _, builtin := export.Builtin(name); builtin {
		if action == "apagado" {
			return &InvalidError{fmt.Sprintf("%q é um perfil padrão e não pode ser apagado", name)}
		}
		return &InvalidError{fmt.Sprintf("%q é um perfil padrão e não pode ser alterado: salve uma cópia com outro nome", name)}
	}
	return nil
}

// ReplaceProfile grava a nova versão de um perfil salvo, mantendo quem criou.
func (s *Service) ReplaceProfile(ctx context.Context, p *export.Profile) error {
	if err := Editable(p.Name, "alterado"); err != nil {
		return err
	}
	old, err := s.repo.FindExportProfile(ctx, p.Name)
	if err != nil {
		return err
	}
	p.CreatedBy, p.UpdatedAt = old.CreatedBy, s.now()
	return s.repo.ReplaceExportProfile(ctx, p)
}

// DeleteProfile apaga um perfil salvo.
func (s *Service) DeleteProfile(ctx context.Context, name string) error {
	if err := Editable(name, "apagado"); err != nil {
		return err
	}
	return s.repo.DeleteExportProfile(ctx, name)
}

// ---------- dataset ----------

// Prepare confere o pedido antes de montar o dataset: task (se vier) vale
// mais que a tarefa do perfil, a tarefa tem que existir, e os pacotes do
// perfil também (provavelmente foram renomeados ou apagados).
func (s *Service) Prepare(ctx context.Context, p *export.Profile, task string) error {
	if task = strings.TrimSpace(task); task != "" {
		p.Task = task
	}
	if p.Task == "" {
		return &InvalidError{"escolha a tarefa de anotação (as caixas de qual tarefa entram no dataset)"}
	}
	if _, err := s.boxes.FindTask(ctx, p.Task); errors.Is(err, platform.ErrNotFound) {
		return &InvalidError{fmt.Sprintf("a tarefa %q não existe (foi renomeada ou apagada?)", p.Task)}
	} else if err != nil {
		return err
	}
	for _, name := range p.Packages {
		if _, err := s.catalog.FindPackage(ctx, name); errors.Is(err, platform.ErrNotFound) {
			return &InvalidError{fmt.Sprintf("o pacote %q do perfil não existe (foi renomeado ou apagado?)", name)}
		} else if err != nil {
			return err
		}
	}
	return nil
}

// Collect passa pelas imagens do perfil e monta os itens do dataset e o
// resumo. Os arquivos só são abertos na hora de escrever o zip.
func (s *Service) Collect(ctx context.Context, p *export.Profile) ([]export.Item, *Preview, error) {
	prev := &Preview{
		Profile: p.Name, Task: p.Task, Format: p.Format, Layout: p.Layout, Split: p.Split,
		Images: map[string]int{}, Folders: map[string]int{}, Examples: []string{}, Boxes: map[string]int{}, Packages: p.Packages,
	}
	if prev.Packages == nil {
		prev.Packages = []string{}
	}
	labels := export.NewLabels(p.Labels)
	seen := map[string]int{}
	var items []export.Item

	pkgs := p.Packages
	if len(pkgs) == 0 {
		pkgs = []string{""} // todos
	}
	f := p.Filters
	const pageSize = 500
	for _, pkg := range pkgs {
		for skip := int64(0); ; skip += pageSize {
			imgs, _, err := s.images.List(ctx, images.ImageFilter{
				Package: pkg, Status: images.StatusVerified,
				Equipment: f.Equipment, Camera: f.Camera, Direction: f.Direction, DayTime: f.DayTime,
				Weather: f.Weather, VehicleType: f.VehicleType, VehicleColor: f.VehicleColor,
				Limit: pageSize, Skip: skip,
			})
			if err != nil {
				return nil, nil, err
			}
			ids := make([]string, len(imgs))
			for i, img := range imgs {
				ids[i] = img.SHA1
			}
			sets, err := s.boxes.FindSets(ctx, p.Task, ids)
			if err != nil {
				return nil, nil, err
			}
			for _, img := range imgs {
				set, ok := sets[img.SHA1]
				if !ok {
					prev.Skipped.NotAnnotated++
					continue
				}
				stored := path.Base(img.Path)
				meta := imageMeta(&img)
				item := export.Item{
					SHA1: img.SHA1, Width: set.Width, Height: set.Height,
					FileName: p.FileNameOf(export.NameMeta(img.SHA1, stored, img.OriginalName, meta), path.Ext(stored)),
					Split:    export.SplitOf(img.SHA1, p.Split),
					Open:     s.opener(img.Path),
				}
				if p.Layout == export.LayoutFolders {
					item.Dir = p.Dir(meta, item.Split)
				}
				for _, b := range set.Boxes {
					if name, ok := labels.Map(b.Label); ok {
						b.Label = name
						item.Boxes = append(item.Boxes, b)
						seen[name]++
					}
				}
				switch {
				case len(set.Boxes) == 0: // negativa: entra sem caixas
					prev.Negatives++
				case len(item.Boxes) == 0: // tinha caixas, mas de labels que não entram
					prev.Skipped.WithoutLabels++
					continue
				}
				items = append(items, item)
				if len(prev.Examples) < 3 {
					prev.Examples = append(prev.Examples, path.Join(item.Dir, item.FileName))
				}
				prev.Images[item.Split]++
				if item.Dir != "" {
					prev.Folders[item.Dir]++
				}
				for _, b := range item.Boxes {
					prev.Boxes[b.Label]++
				}
			}
			if len(imgs) < pageSize {
				break
			}
		}
	}
	prev.Total = len(items)
	prev.Classes = labels.Classes(seen)
	return items, prev, nil
}

// imageMeta: os metadados da imagem que viram pasta ou parte do nome de
// saída (os do veículo vêm do primeiro veículo; a condição é a primeira).
func imageMeta(img *images.Image) export.Meta {
	m := export.Meta{
		"resolution": img.Resolution, "direction": img.Direction, "day_time": img.DayTime,
		"weather": img.Weather, "capture_angle": img.CaptureAngle, "image_range": img.ImageRange,
		"equipment": img.Equipment, "camera": img.Camera, "package": img.Package,
	}
	if len(img.Vehicles) > 0 {
		v := img.Vehicles[0]
		m["vehicle_type"], m["plate_type"], m["plate_category"], m["vehicle_color"] = v.Type, v.PlateType, v.PlateCategory, v.Color
		m["plate"] = v.Plate
	}
	if len(img.Conditions) > 0 {
		m["condition"] = img.Conditions[0]
	}
	if img.CapturedAt != nil {
		t := img.CapturedAt.UTC()
		m["captured_at"], m["date"] = t.Format("20060102T150405Z"), t.Format("20060102")
	}
	return m
}

func (s *Service) opener(rel string) func() (io.ReadCloser, error) {
	return func() (io.ReadCloser, error) { return os.Open(s.files.Abs(rel)) }
}
