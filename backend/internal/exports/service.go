package exports

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"SIDM/internal/annotation"
	"SIDM/internal/boxes"
	"SIDM/internal/export"
	"SIDM/internal/images"
	"SIDM/internal/platform"
)

type Service struct {
	repo       Repo
	images     Images
	boxes      Boxes
	catalog    Catalog
	files      Files
	annotation Annotation
	versions   Versions // nil: sem o export de versões (UseVersions)
	now        func() time.Time
}

func New(repo Repo, img Images, bx Boxes, cat Catalog, fs Files, ann Annotation) *Service {
	return &Service{repo: repo, images: img, boxes: bx, catalog: cat, files: fs, annotation: ann,
		now: func() time.Time { return time.Now().UTC() }}
}

// AnnotationProfile busca o perfil de anotação de um perfil de export (vazio
// = o padrão). Um que não existe é InvalidError.
func (s *Service) AnnotationProfile(ctx context.Context, name string) (*annotation.Profile, error) {
	if name == "" {
		name = annotation.DefaultProfile
	}
	ap, err := s.annotation.FindProfile(ctx, name)
	if errors.Is(err, platform.ErrNotFound) {
		return nil, &InvalidError{fmt.Sprintf("annotation_profile: o perfil de anotação %q não existe", name)}
	}
	return ap, err
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
	for i := range saved {
		saved[i].Normalize() // os salvos antes dos perfis de anotação têm os filtros antigos
	}
	return append(export.Builtins(), saved...), nil
}

// FindProfile busca um padrão ou um salvo (platform.ErrNotFound se nenhum).
func (s *Service) FindProfile(ctx context.Context, name string) (*export.Profile, error) {
	if p, ok := export.Builtin(name); ok {
		return &p, nil
	}
	p, err := s.repo.FindExportProfile(ctx, name)
	if err != nil {
		return nil, err
	}
	p.Normalize()
	return p, nil
}

// Validate limpa e valida um perfil que veio de fora, contra o perfil de
// anotação dele.
func (s *Service) Validate(ctx context.Context, p *export.Profile) error {
	p.Builtin = false // só o servidor marca perfil padrão
	p.Normalize()
	ap, err := s.AnnotationProfile(ctx, p.AnnotationProfile)
	if err != nil {
		return err
	}
	if err := p.Validate(ap); err != nil {
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

// Prepare confere o pedido antes de montar o dataset: as tarefas do pedido
// (se vierem) valem mais que as do perfil, cada tarefa tem que existir, e os
// pacotes do perfil também (provavelmente foram renomeados ou apagados).
func (s *Service) Prepare(ctx context.Context, p *export.Profile, tasks []string) error {
	p.Normalize() // um perfil salvo antes das várias tarefas tem só "task"
	if len(tasks) > 0 {
		p.SetTasks(tasks)
	}
	if len(p.Tasks) > export.MaxTasks {
		return &InvalidError{fmt.Sprintf("no máximo %d tarefas num dataset", export.MaxTasks)}
	}
	ap, err := s.AnnotationProfile(ctx, p.AnnotationProfile)
	if err != nil {
		return err
	}
	p.Bind(ap)
	if p.NeedsTasks() && len(p.Tasks) == 0 {
		return &InvalidError{"escolha a tarefa de anotação (as caixas de qual tarefa entram no dataset)"}
	}
	for _, task := range p.Tasks {
		if _, err := s.boxes.FindTask(ctx, task); errors.Is(err, platform.ErrNotFound) {
			return &InvalidError{fmt.Sprintf("a tarefa %q não existe (foi renomeada ou apagada?)", task)}
		} else if err != nil {
			return err
		}
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
	c := s.newCollector(p)
	pkgs := p.Packages
	if len(pkgs) == 0 {
		pkgs = []string{""} // todos
	}
	fields, objs := p.FilterValues()
	objects := make([]images.ObjectFilter, 0, len(objs))
	for _, t := range sortedKeys(objs) {
		objects = append(objects, images.ObjectFilter{Type: t, Fields: objs[t]})
	}
	const pageSize = 500
	for _, pkg := range pkgs {
		for skip := int64(0); ; skip += pageSize {
			imgs, _, err := s.images.List(ctx, images.ImageFilter{
				Package: pkg, Status: images.StatusVerified,
				Fields: fields, Objects: objects,
				Limit: pageSize, Skip: skip,
			})
			if err != nil {
				return nil, nil, err
			}
			ids := make([]string, len(imgs))
			for i, img := range imgs {
				ids[i] = img.SHA1
			}
			// As caixas de cada tarefa (uma busca por tarefa e página).
			byTask := make([]map[string]boxes.Set, len(p.Tasks))
			for i, task := range p.Tasks {
				if byTask[i], err = s.boxes.FindSets(ctx, task, ids); err != nil {
					return nil, nil, err
				}
			}
			for i := range imgs {
				c.add(&imgs[i], byTask)
			}
			if len(imgs) < pageSize {
				break
			}
		}
	}
	items, prev := c.done()
	return items, prev, nil
}

// collector monta os itens e o resumo, uma imagem por vez (a mesma conta
// para as imagens de hoje e para as de uma versão).
type collector struct {
	s      *Service
	p      *export.Profile
	prev   *Preview
	labels *export.Labels
	seen   map[string]int
	items  []export.Item
}

func (s *Service) newCollector(p *export.Profile) *collector {
	prev := &Preview{
		Profile: p.Name, Task: strings.Join(p.Tasks, " + "), Tasks: p.Tasks, AllTasks: p.AllTasks, Format: p.Format, Layout: p.Layout, Split: p.Split,
		Images: map[string]int{}, Folders: map[string]int{}, Examples: []string{}, Boxes: map[string]int{}, Packages: p.Packages,
	}
	if prev.Packages == nil {
		prev.Packages = []string{}
	}
	return &collector{s: s, p: p, prev: prev, labels: export.NewLabels(p.Labels), seen: map[string]int{}}
}

// add: uma imagem verificada, com as caixas de cada tarefa (na ordem do
// perfil). Ela entra ou é contada como pulada.
func (c *collector) add(img *images.Image, byTask []map[string]boxes.Set) {
	p, prev := c.p, c.prev
	set, found := mergeSets(img.SHA1, byTask)
	switch {
	case !p.NeedsTasks(): // só imagens: toda verificada que passa nos filtros
	case found == 0:
		prev.Skipped.NotAnnotated++
		return
	case p.AllTasks && found < len(p.Tasks):
		prev.Skipped.MissingTasks++
		return
	}
	// O "nome" da imagem: o v2 (file_name) se ela tem, senão o do
	// disco (<sha1>.ext). É o que o modo de nome padrão usa.
	stored := img.FileName
	if stored == "" {
		stored = path.Base(img.Path)
	}
	meta := imageMeta(img)
	nameMeta := export.NameMeta(img.SHA1, stored, img.OriginalName, meta)
	item := export.Item{
		SHA1: img.SHA1, Width: set.Width, Height: set.Height,
		FileName: p.FileNameOf(nameMeta, path.Ext(stored)),
		Meta:     nameMeta,
		Split:    export.SplitOf(img.SHA1, p.Split),
		Open:     c.s.opener(img.Path),
	}
	if p.Layout == export.LayoutFolders {
		item.Dir = p.Dir(meta, item.Split)
	}
	for _, b := range set.Boxes {
		if name, ok := c.labels.Map(b.Label); ok {
			b.Label = name
			item.Boxes = append(item.Boxes, b)
			c.seen[name]++
		}
	}
	switch {
	case !p.NeedsTasks(): // sem caixas: não é negativa
	case len(set.Boxes) == 0: // negativa: entra sem caixas
		prev.Negatives++
	case len(item.Boxes) == 0: // tinha caixas, mas de labels que não entram
		prev.Skipped.WithoutLabels++
		return
	}
	c.items = append(c.items, item)
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

func (c *collector) done() ([]export.Item, *Preview) {
	c.prev.Total = len(c.items)
	c.prev.Classes = c.labels.Classes(c.seen)
	return c.items, c.prev
}

// mergeSets junta as caixas de uma imagem nas tarefas (na ordem do perfil)
// e diz em quantas ela está anotada. O tamanho é o da primeira tarefa; as
// caixas de uma tarefa com outro tamanho são levadas para ele (o CVAT pode ter
// recebido a imagem redimensionada).
func mergeSets(sha string, byTask []map[string]boxes.Set) (boxes.Set, int) {
	var out boxes.Set
	found := 0
	for _, sets := range byTask {
		set, ok := sets[sha]
		if !ok {
			continue
		}
		found++
		if found == 1 {
			out.Width, out.Height = set.Width, set.Height
			out.Boxes = append([]boxes.Box{}, set.Boxes...)
			continue
		}
		sx, sy := 1.0, 1.0
		if set.Width > 0 && set.Height > 0 && out.Width > 0 && out.Height > 0 {
			sx, sy = float64(out.Width)/float64(set.Width), float64(out.Height)/float64(set.Height)
		}
		for _, b := range set.Boxes {
			b.X, b.W, b.Y, b.H = b.X*sx, b.W*sx, b.Y*sy, b.H*sy
			out.Boxes = append(out.Boxes, b)
		}
	}
	return out, found
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// imageMeta: os metadados da imagem que viram pasta ou parte do nome de
// saída (as chaves de export.MetaFields). Cada campo da imagem vira
// meta[campo] (de uma lista, o primeiro valor; "conditions" também vira
// "condition"; uma data vira AAAAMMDDTHHMMSSZ e, a captured_at, também
// "date"). Do PRIMEIRO objeto de cada tipo sai meta[<tipo>_<campo>], e do
// primeiro objeto da imagem, meta[campo] (se a imagem não tem um campo com o
// mesmo nome): no perfil veiculos, isso dá vehicle_type, vehicle_color,
// plate, plate_type...
func imageMeta(img *images.Image) export.Meta {
	m := export.Meta{"package": img.Package}
	for k, v := range img.Fields {
		m[k] = metaValue(v)
	}
	if c, ok := img.Fields["conditions"].([]string); ok && len(c) > 0 {
		m["condition"] = c[0]
	}
	if t, ok := img.Fields["captured_at"].(time.Time); ok {
		m["date"] = t.UTC().Format("20060102")
	}
	seen := map[string]bool{}
	for i, o := range img.Objects {
		if seen[o.Type] {
			continue
		}
		seen[o.Type] = true
		for k, v := range o.Fields {
			if _, taken := img.Fields[k]; !taken && i == 0 {
				m[k] = metaValue(v)
			}
			m[o.Type+"_"+k] = metaValue(v)
		}
	}
	return m
}

// metaValue: o valor de um campo como texto, para pasta e nome de arquivo.
func metaValue(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []string:
		if len(x) > 0 {
			return x[0]
		}
		return ""
	case time.Time:
		return x.UTC().Format("20060102T150405Z")
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	}
	return fmt.Sprint(v)
}

func (s *Service) opener(rel string) func() (io.ReadCloser, error) {
	return func() (io.ReadCloser, error) { return os.Open(s.files.Abs(rel)) }
}
