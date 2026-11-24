// Package exports é o módulo de export de datasets de treino (RF-DETR, YOLO,
// Pascal VOC): os perfis (padrões, no código, e os salvos), o preview do que
// vai sair e o zip.
//
// Entram as imagens VERIFICADAS e ANOTADAS NA TAREFA escolhida que passam
// nos filtros do perfil (as anotadas sem nenhuma caixa entram como
// negativas). A divisão train/valid/test é fixa por imagem.
//
// O formato do dataset (perfil, divisão, pastas, nomes, escrita do zip) fica
// no pacote export, que é puro (sem banco e sem HTTP). Aqui fica o que tem
// banco e HTTP. A coleção deste módulo é a export_profiles; imagens, caixas,
// pacotes e arquivos são lidos pelos Services dos outros módulos.
package exports

import (
	"context"

	"SIDM/internal/annotation"
	"SIDM/internal/boxes"
	"SIDM/internal/catalog"
	"SIDM/internal/export"
	"SIDM/internal/images"
)

// Repo é a coleção export_profiles (só os salvos: os padrões ficam no código).
type Repo interface {
	CreateExportProfile(ctx context.Context, p *export.Profile) error
	FindExportProfile(ctx context.Context, name string) (*export.Profile, error)
	ReplaceExportProfile(ctx context.Context, p *export.Profile) error
	DeleteExportProfile(ctx context.Context, name string) error
	ListExportProfiles(ctx context.Context) ([]export.Profile, error)
	// TaskRenamed troca a tarefa nos perfis que apontam para ela.
	TaskRenamed(ctx context.Context, oldName, newName string) error
}

// Images é o que o export lê das imagens.
type Images interface {
	List(ctx context.Context, f images.ImageFilter) ([]images.Image, int64, error)
}

// Boxes é o que o export lê das tarefas e caixas.
type Boxes interface {
	FindTask(ctx context.Context, name string) (*boxes.Task, error)
	FindSets(ctx context.Context, task string, shas []string) (map[string]boxes.Set, error)
}

// Annotation: os perfis de anotação (os campos dos filtros, das pastas e do
// nome de um perfil de export são os do perfil de anotação dele).
type Annotation interface {
	FindProfile(ctx context.Context, name string) (*annotation.Profile, error)
}

// Catalog: os pacotes do perfil têm que existir.
type Catalog interface {
	FindPackage(ctx context.Context, name string) (*catalog.Package, error)
}

// Files abre as imagens no disco.
type Files interface {
	Abs(rel string) string
}

// Preview é o resumo do que vai sair.
type Preview struct {
	Profile   string         `json:"profile"`
	Task      string         `json:"task"`      // as tarefas, juntas para mostrar ("parabrisa + placa")
	Tasks     []string       `json:"tasks"`     //
	AllTasks  bool           `json:"all_tasks"` // só as anotadas em todas
	Format    string         `json:"format"`
	Layout    string         `json:"layout"`
	Split     export.Split   `json:"split"`
	Classes   []string       `json:"classes"`
	Total     int            `json:"total"`     // imagens no dataset
	Negatives int            `json:"negatives"` // dessas, as sem nenhuma caixa (anotadas como vazias)
	Images    map[string]int `json:"images"`    // por grupo
	Folders   map[string]int `json:"folders"`   // por pasta (layout por pastas; vazio no por divisão)
	Examples  []string       `json:"examples"`  // os primeiros nomes de arquivo, como vão sair
	Boxes     map[string]int `json:"boxes"`     // por classe
	Skipped   Skipped        `json:"skipped"`
	Packages  []string       `json:"packages"` // os que entraram (vazio no perfil = todos)
	// Version: no export de uma versão, qual (as imagens são as dela).
	Version *VersionInfo `json:"version,omitempty"`
}

// Skipped são as imagens verificadas que ficaram de fora.
type Skipped struct {
	NotAnnotated  int `json:"not_annotated"`  // verificadas, mas não anotadas em nenhuma das tarefas
	MissingTasks  int `json:"missing_tasks"`  // com all_tasks: anotadas em só parte das tarefas
	WithoutLabels int `json:"without_labels"` // com caixas, mas nenhuma das labels escolhidas
}

// InvalidError é um pedido que não dá para atender como veio (vira 400).
type InvalidError struct{ Msg string }

func (e *InvalidError) Error() string { return e.Msg }
