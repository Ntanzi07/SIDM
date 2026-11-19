// Package boxes é o módulo das caixas (bounding boxes) anotadas no CVAT,
// organizadas em TAREFAS (ex.: "capacete", "placa-letras"): cada tarefa tem
// as suas labels, e a mesma imagem pode estar em várias tarefas sem uma
// sobrescrever a outra.
//
// O SIDM não desenha caixas: manda as imagens para o CVAT (cvat.zip) e
// importa o export COCO 1.0 de volta, na tarefa (coco.go lê o arquivo).
//
// O módulo é dono das coleções annotation_tasks e image_boxes. O que ele
// precisa das imagens (achar por sha1, saber quais estão verificadas, ler o
// arquivo) pede pela interface Images; o pacote da URL vem do catálogo
// (Packages). Ele é avisado quando um pacote muda (catalog.PackageListener)
// e avisa quando uma tarefa é renomeada (TaskListener).
package boxes

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"SIDM/internal/annotation"
	"SIDM/internal/catalog"
)

// Task é uma tarefa de anotação de caixas. Os pacotes de uma tarefa não são
// guardados: saem das imagens que foram anotadas nela (Stats).
type Task struct {
	Name        string    `bson:"name" json:"name"`
	Description string    `bson:"description" json:"description"`
	Labels      []string  `bson:"labels" json:"labels"`
	CreatedBy   string    `bson:"created_by" json:"created_by"`
	CreatedAt   time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time `bson:"updated_at" json:"updated_at"`
}

// DefaultTask recebe as caixas importadas antes de existirem tarefas.
const DefaultTask = "padrao"

// Set são as caixas de UMA imagem em UMA tarefa (um documento de
// image_boxes). Os metadados da imagem e as caixas têm ciclos de vida
// diferentes, por isso a coleção é separada.
//
// Boxes vazio = a imagem foi anotada na tarefa e não tem nenhum objeto
// (negativa): entra no export como imagem sem caixas.
type Set struct {
	ID         any       `bson:"_id,omitempty" json:"-"`
	Task       string    `bson:"task" json:"task"`
	ImageID    string    `bson:"image" json:"sha1"`
	Package    string    `bson:"package" json:"package"`
	Width      int       `bson:"width" json:"width"` // tamanho da imagem no CVAT (as caixas são em pixels dela)
	Height     int       `bson:"height" json:"height"`
	Boxes      []Box     `bson:"boxes" json:"boxes"`
	Source     Source    `bson:"source" json:"source"`
	ImportedBy string    `bson:"imported_by" json:"imported_by"`
	ImportedAt time.Time `bson:"imported_at" json:"imported_at"`
	// UpdatedBy/UpdatedAt: quem editou os atributos no SIDM, e quando. Vazio
	// = como veio do CVAT (o reimport zera: o CVAT é a fonte).
	UpdatedBy string     `bson:"updated_by,omitempty" json:"updated_by,omitempty"`
	UpdatedAt *time.Time `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

// Version é a versão do Set para a edição (optimistic locking): a última
// edição no SIDM ou, sem edição, o import.
func (s *Set) Version() time.Time {
	if s.UpdatedAt != nil {
		return *s.UpdatedAt
	}
	return s.ImportedAt
}

// Source diz de onde as caixas vieram.
type Source struct {
	Tool     string `bson:"tool" json:"tool"`           // "cvat"
	Format   string `bson:"format" json:"format"`       // "coco"
	File     string `bson:"file" json:"file"`           // nome do arquivo importado
	FileName string `bson:"file_name" json:"file_name"` // nome da imagem dentro do arquivo
}

// Stats é o resumo de uma tarefa.
type Stats struct {
	Images    int64            `json:"images"`    // imagens anotadas na tarefa
	Verified  int64            `json:"verified"`  // dessas, as verificadas (entram no export)
	Negatives int64            `json:"negatives"` // anotadas sem nenhuma caixa
	Boxes     int64            `json:"boxes"`
	Labels    map[string]int64 `json:"labels"` // caixas por label
	Packages  []TaskPackage    `json:"packages"`
}

// TaskPackage: quantas imagens de um pacote estão na tarefa.
type TaskPackage struct {
	Name     string `json:"name"`
	Images   int64  `json:"images"`
	Verified int64  `json:"verified"`
}

// PackageCount: quantas imagens de um pacote estão numa tarefa.
type PackageCount struct {
	Task   string `bson:"_id" json:"task"`
	Images int64  `bson:"images" json:"images"`
	Boxes  int64  `bson:"boxes" json:"boxes"`
}

// TaskImages é o que o repositório sabe de uma tarefa: os sha1 anotados,
// por pacote, quantos sem caixa e as caixas por label.
type TaskImages struct {
	IDs       map[string][]string // pacote -> sha1
	Negatives int64
	Labels    map[string]int64
}

// Repo são as coleções do módulo (Mongo em produção, memória nos testes).
type Repo interface {
	CreateTask(ctx context.Context, t *Task) error
	FindTask(ctx context.Context, name string) (*Task, error)
	ListTasks(ctx context.Context) ([]Task, error)
	// ReplaceTask grava t no lugar de oldName (o nome pode mudar).
	ReplaceTask(ctx context.Context, oldName string, t *Task) error
	AddTaskLabels(ctx context.Context, name string, labels []string, now time.Time) error
	DeleteTask(ctx context.Context, name string) error
	TaskImages(ctx context.Context, task string) (*TaskImages, error)

	ReplaceSets(ctx context.Context, sets []Set) error
	FindImageSets(ctx context.Context, sha1 string) ([]Set, error)
	FindSets(ctx context.Context, task string, ids []string) (map[string]Set, error)
	DeleteImageSets(ctx context.Context, sha1 string) error
	// UpdateSet grava as caixas editadas de UM Set, só se ele ainda está na
	// versão prev (senão ErrConflict; platform.ErrNotFound se sumiu).
	UpdateSet(ctx context.Context, set *Set, prev time.Time) error
	PackageCounts(ctx context.Context, pkg string) ([]PackageCount, error)
	// LabelStats conta, nas tarefas, as caixas das imagens ids: por tarefa e
	// por label (ver stats.go).
	LabelStats(ctx context.Context, tasks []string, ids []string) (map[string]*TaskStats, error)
	// RenameTaskSets / DeleteTaskSets: as caixas de uma tarefa.
	RenameTaskSets(ctx context.Context, oldName, newName string) error
	DeleteTaskSets(ctx context.Context, task string) (int64, error)
	// RenamePackageSets / DeletePackageSets: as caixas das imagens de um pacote.
	RenamePackageSets(ctx context.Context, oldName, newName string) error
	DeletePackageSets(ctx context.Context, pkg string) error
}

// ImageRef é o que o módulo precisa saber de uma imagem.
type ImageRef struct {
	SHA1         string
	Package      string
	OriginalName string
	Path         string // relativo à pasta das imagens
}

// Images é o que o módulo precisa de quem cuida das imagens.
type Images interface {
	// BySHA1 / ByOriginalName: em TODOS os pacotes (o import procura no banco
	// todo). Por nome original pode voltar mais de uma por nome.
	BySHA1(ctx context.Context, shas []string) ([]ImageRef, error)
	ByOriginalName(ctx context.Context, names []string) ([]ImageRef, error)
	Exists(ctx context.Context, sha1 string) (bool, error)
	// InPackage: uma página das imagens do pacote (para o cvat.zip).
	InPackage(ctx context.Context, pkg string, limit, skip int64) ([]ImageRef, int64, error)
	// SearchIDs: os sha1 das imagens que passam na pesquisa da query (os
	// parâmetros do GET /images/stats: profile, status, package, fields.*,
	// objects.*). Erro que embrulha platform.ErrInvalid = pedido inválido.
	SearchIDs(ctx context.Context, q url.Values) ([]string, error)
	// Verified: quais destes sha1 estão verificados.
	Verified(ctx context.Context, ids []string) (map[string]bool, error)
}

// Profiles: o perfil de anotação do pacote (o *images.Service). Os
// atributos de uma caixa seguem o tipo de objeto com caixa cuja chave é a
// label.
type Profiles interface {
	ProfileOf(ctx context.Context, pkg string) (*annotation.Profile, error)
}

// ErrConflict: as caixas mudaram (outra edição ou um reimport) desde que
// foram lidas.
var ErrConflict = errors.New("as caixas mudaram desde que foram lidas")

// Files: onde fica o arquivo de uma imagem (o *files.Storage).
type Files interface {
	Abs(path string) string
}

// Packages: o pacote da URL (o *catalog.Service).
type Packages interface {
	PackageByName(w http.ResponseWriter, r *http.Request, name string) (*catalog.Package, bool)
}

// TaskListener é avisado quando uma tarefa muda de nome (ex.: os perfis de
// export, que guardam a tarefa).
type TaskListener interface {
	TaskRenamed(ctx context.Context, oldName, newName string) error
}
