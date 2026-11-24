package exports

// O export de uma VERSÃO de um dataset (o módulo versions): o mesmo zip, mas
// as imagens e a anotação são as congeladas na versão. Do perfil de export
// valem o formato, a divisão, as pastas, os nomes e as labels; os pacotes e
// os filtros dele não (quem decide as imagens é a versão).

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"SIDM/internal/annotation"
	"SIDM/internal/boxes"
	"SIDM/internal/export"
	"SIDM/internal/versions"
)

// Versions: as versões congeladas (o versions.Service).
type Versions interface {
	Frozen(ctx context.Context, dataset string, number int) (*versions.Frozen, error)
}

// UseVersions liga o export de versões (sem isso, um pedido com versão é
// recusado).
func (s *Service) UseVersions(v Versions) { s.versions = v }

// VersionRef: a versão no pedido de export. Number 0 = o "agora" (o estado
// atual do dataset, sem gravar).
type VersionRef struct {
	Dataset string `json:"dataset"`
	Number  int    `json:"number"`
}

// VersionInfo: a versão no resumo e no export_info.json do zip.
type VersionInfo struct {
	Dataset   string `json:"dataset"`
	Number    int    `json:"number"` // 0 = o estado atual, não gravado
	ID        string `json:"id,omitempty"`
	Message   string `json:"message"`
	CreatedBy string `json:"created_by,omitempty"`
	CreatedAt string `json:"created_at"`
	Profile   string `json:"annotation_profile"`
	Images    int    `json:"images"` // na versão (as que entram no zip estão no total)
}

// FindVersion busca a versão do pedido.
func (s *Service) FindVersion(ctx context.Context, ref VersionRef) (*versions.Frozen, error) {
	if s.versions == nil {
		return nil, &InvalidError{"o export de versões não está ligado"}
	}
	if strings.TrimSpace(ref.Dataset) == "" || ref.Number < 0 {
		return nil, &InvalidError{`version: esperado {"dataset": "nome", "number": 3} (0 = o estado atual)`}
	}
	number := ref.Number
	if number == 0 {
		number = versions.Now
	}
	return s.versions.Frozen(ctx, ref.Dataset, number)
}

// PrepareVersion é o Prepare de uma versão: as tarefas têm que estar
// congeladas nela (sem tarefa no pedido nem no perfil, entram todas as
// dela), e o perfil de anotação é o da versão.
func (s *Service) PrepareVersion(ctx context.Context, p *export.Profile, tasks []string, fz *versions.Frozen) error {
	v := fz.Version
	p.Normalize()
	if len(tasks) > 0 {
		p.SetTasks(tasks)
	}
	if p.NeedsTasks() && len(p.Tasks) == 0 {
		p.SetTasks(v.Tasks)
	}
	if len(p.Tasks) > export.MaxTasks {
		return &InvalidError{fmt.Sprintf("no máximo %d tarefas num dataset", export.MaxTasks)}
	}
	for _, t := range p.Tasks {
		if !slices.Contains(v.Tasks, t) {
			return &InvalidError{fmt.Sprintf("a tarefa %q não está na %s (tarefas dela: %s)", t, versionText(v), strings.Join(v.Tasks, ", "))}
		}
	}
	if p.NeedsTasks() && len(p.Tasks) == 0 {
		return &InvalidError{fmt.Sprintf("a %s não tem nenhuma tarefa com caixas: use o formato só de imagens", versionText(v))}
	}

	// Um perfil padrão serve para qualquer perfil de anotação; um salvo, só
	// para o dele (as pastas e os nomes usam os campos dele).
	want := p.AnnotationProfile
	if want == "" {
		want = annotation.DefaultProfile
	}
	if p.Builtin {
		p.AnnotationProfile = v.Profile
	} else if want != v.Profile {
		return &InvalidError{fmt.Sprintf("o perfil de export %q é do perfil de anotação %q, e a %s é do %q", p.Name, want, versionText(v), v.Profile)}
	}
	ap, err := s.AnnotationProfile(ctx, v.Profile)
	if err != nil {
		return err
	}
	p.Bind(ap)
	// Quem decide as imagens é a versão.
	p.Packages = v.Query.Packages
	p.Filters = export.Filters{}
	return nil
}

// CollectVersion: o Collect com as imagens e as caixas congeladas.
func (s *Service) CollectVersion(p *export.Profile, fz *versions.Frozen) ([]export.Item, *Preview) {
	c := s.newCollector(p)
	byTask := make([]map[string]boxes.Set, len(p.Tasks))
	for i, t := range p.Tasks {
		byTask[i] = fz.Boxes[t]
	}
	for i := range fz.Images {
		c.add(&fz.Images[i], byTask)
	}
	items, prev := c.done()
	prev.Version = versionInfo(fz)
	return items, prev
}

func versionInfo(fz *versions.Frozen) *VersionInfo {
	v := fz.Version
	return &VersionInfo{
		Dataset: v.Dataset, Number: v.Number, ID: v.ID, Message: v.Message, CreatedBy: v.CreatedBy,
		CreatedAt: v.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"), Profile: v.Profile, Images: len(fz.Images),
	}
}

// versionText: "versão 3 do dataset \"motos\"" (ou "o estado atual").
func versionText(v *versions.Version) string {
	if v.Number == 0 {
		return fmt.Sprintf("versão atual (não salva) do dataset %q", v.Dataset)
	}
	return fmt.Sprintf("versão %d do dataset %q", v.Number, v.Dataset)
}
