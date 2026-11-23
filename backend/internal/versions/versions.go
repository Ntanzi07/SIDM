// Package versions é o versionamento de datasets: um dataset é uma pesquisa
// salva (o perfil de anotação, os pacotes, os filtros e as tarefas, como na
// tela de Pesquisa) e cada VERSÃO é uma foto congelada dele: quais imagens
// verificadas entravam, a anotação de cada uma (campos, objetos e as caixas
// de cada tarefa), quem criou, quando, uma mensagem e um resumo pronto para o
// dashboard.
//
// As imagens não são copiadas: no disco elas já são imutáveis (<sha1>.ext).
// O que a versão congela é a anotação, que muda com o tempo. Uma versão nunca
// muda depois de criada: corrigir uma anotação gera a PRÓXIMA versão.
//
// Coleções: datasets, versions e version_items (uma por imagem da versão,
// fora do documento da versão por causa do limite de 16 MB do Mongo).
package versions

import (
	"context"
	"net/url"
	"time"

	"SIDM/internal/annotation"
	"SIDM/internal/boxes"
	"SIDM/internal/images"
	"SIDM/internal/platform"
)

// Filter: um filtro da Pesquisa ("fields.<campo>" ou "objects.<tipo>.<campo>"
// = valor). Em lista, e não em mapa, porque a chave tem pontos.
type Filter struct {
	Key   string `bson:"key" json:"key"`
	Value string `bson:"value" json:"value"`
}

// Query: quais imagens entram no dataset. São sempre as VERIFICADAS (uma
// versão é algo pronto para treinar).
type Query struct {
	Packages []string `bson:"packages" json:"packages"` // vazio = todos os pacotes do perfil
	Filters  []Filter `bson:"filters" json:"filters"`
	Tasks    []string `bson:"tasks" json:"tasks"` // as caixas destas tarefas; vazio = todas
}

// values: a query no formato do GET /images/stats (o images filtra).
func (q Query) values(profile string) url.Values {
	v := url.Values{"profile": {profile}, "status": {images.StatusVerified}}
	for _, p := range q.Packages {
		v.Add("package", p)
	}
	for _, f := range q.Filters {
		v.Set(f.Key, f.Value)
	}
	return v
}

// Dataset é um documento da coleção datasets.
type Dataset struct {
	Name        string    `bson:"name" json:"name"`
	Profile     string    `bson:"profile" json:"profile"` // perfil de anotação
	Description string    `bson:"description" json:"description"`
	Query       Query     `bson:"query" json:"query"`
	CreatedBy   string    `bson:"created_by" json:"created_by"`
	CreatedAt   time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time `bson:"updated_at" json:"updated_at"`

	// Latest: a versão mais nova, sem o resumo (só na listagem; não é
	// gravado).
	Latest *VersionInfo `bson:"-" json:"latest,omitempty"`
}

// VersionInfo: o essencial de uma versão (para listas).
type VersionInfo struct {
	Number    int       `json:"number"`
	Message   string    `json:"message"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	Images    int64     `json:"images"`
}

// Version é um documento da coleção versions: a foto congelada.
type Version struct {
	ID        string    `bson:"_id" json:"id"`
	Dataset   string    `bson:"dataset" json:"dataset"`
	Number    int       `bson:"number" json:"number"` // 1, 2, 3... por dataset
	Message   string    `bson:"message" json:"message"`
	CreatedBy string    `bson:"created_by" json:"created_by"`
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
	// Profile e Query: como o dataset estava ao criar a versão (o dataset
	// pode mudar depois; a versão não).
	Profile string   `bson:"profile" json:"profile"`
	Query   Query    `bson:"query" json:"query"`
	Tasks   []string `bson:"tasks" json:"tasks"` // as tarefas congeladas (a lista real, mesmo quando a query diz "todas")
	Summary Summary  `bson:"summary" json:"summary"`

	// items: só no "agora" (Current), que não é gravado: os itens montados
	// na hora, no lugar dos que uma versão guarda no banco.
	items []Item
}

// Now: o "agora" nas rotas ({number} ou ?to= = "atual"): o estado atual do
// dataset, montado na hora como uma versão, mas sem gravar. Volta com
// number 0.
const Now = -1

func (v *Version) info() *VersionInfo {
	return &VersionInfo{Number: v.Number, Message: v.Message, CreatedBy: v.CreatedBy, CreatedAt: v.CreatedAt, Images: v.Summary.Images.Total}
}

// Summary: o resumo para o dashboard, contado na hora de criar (uma versão
// antiga abre sem recalcular nada). As mesmas contas da Pesquisa
// (images.CountStats e boxes.CountLabels).
type Summary struct {
	Images images.Stats      `bson:"images" json:"images"`
	Labels []boxes.TaskStats `bson:"labels" json:"labels"`
}

// Item é um documento da coleção version_items: uma imagem da versão, com
// a anotação congelada.
type Item struct {
	Version  string `bson:"version" json:"-"`
	SHA1     string `bson:"sha1" json:"sha1"`
	Package  string `bson:"package" json:"package"`
	FileName string `bson:"file_name" json:"file_name"` // o nome que o export usa
	// OriginalName: o nome com que a imagem subiu (o que a pessoa reconhece).
	OriginalName string              `bson:"original_name" json:"original_name"`
	Ext          string              `bson:"ext" json:"ext"`
	Fields       map[string]any      `bson:"fields" json:"fields"`
	Objects      []annotation.Object `bson:"objects" json:"objects"`
	Boxes        map[string]TaskSet  `bson:"boxes" json:"boxes"` // tarefa → as caixas
	// Hash: o sha1 da anotação (campos, objetos e caixas). Duas versões com a
	// mesma imagem e hashes diferentes = a anotação mudou (o diff usa).
	Hash string `bson:"hash" json:"hash"`
}

// TaskSet: as caixas de uma tarefa, com o tamanho da imagem no CVAT (as
// caixas são em pixels dele).
type TaskSet struct {
	Width  int         `bson:"width" json:"width"`
	Height int         `bson:"height" json:"height"`
	Boxes  []boxes.Box `bson:"boxes" json:"boxes"`
}

// Repo são as coleções do módulo.
type Repo interface {
	CreateDataset(ctx context.Context, d *Dataset) error // platform.ErrDuplicate
	FindDataset(ctx context.Context, name string) (*Dataset, error)
	ListDatasets(ctx context.Context) ([]Dataset, error)
	ReplaceDataset(ctx context.Context, d *Dataset) error
	DeleteDataset(ctx context.Context, name string) error

	// LastVersion: a versão de número mais alto do dataset (nil se não há).
	LastVersion(ctx context.Context, dataset string) (*Version, error)
	// InsertVersion: platform.ErrDuplicate se o número já existe no dataset
	// (alguém criou ao mesmo tempo).
	InsertVersion(ctx context.Context, v *Version) error
	FindVersion(ctx context.Context, dataset string, number int) (*Version, error)
	// ListVersions: da mais nova para a mais velha.
	ListVersions(ctx context.Context, dataset string) ([]Version, error)

	InsertItems(ctx context.Context, items []Item) error
	DeleteItems(ctx context.Context, version string) error
	// ListItems: em ordem de sha1.
	ListItems(ctx context.Context, version string, limit, skip int64) ([]Item, int64, error)
	// ItemRefs: todos os itens da versão, só sha1, pacote, nome e hash, em
	// ordem de sha1 (o diff).
	ItemRefs(ctx context.Context, version string) ([]ItemRef, error)
	// FindItem: um item inteiro (platform.ErrNotFound se a imagem não está
	// na versão).
	FindItem(ctx context.Context, version, sha1 string) (*Item, error)
}

// Images é o que o módulo lê das imagens: quem passa na pesquisa e a
// anotação de cada uma.
type Images interface {
	SearchIDs(ctx context.Context, q url.Values) ([]string, error)
	List(ctx context.Context, f images.ImageFilter) ([]images.Image, int64, error)
}

// Boxes é o que o módulo lê das tarefas e caixas.
type Boxes interface {
	ListTasks(ctx context.Context) ([]boxes.Task, error)
	FindSets(ctx context.Context, task string, shas []string) (map[string]boxes.Set, error)
}

// Annotation: os perfis de anotação.
type Annotation interface {
	FindProfile(ctx context.Context, name string) (*annotation.Profile, error)
}

// InvalidError: um pedido que não dá para atender (vira 400). Embrulha o
// platform.ErrInvalid.
type InvalidError struct{ Msg string }

func (e *InvalidError) Error() string { return e.Msg }
func (e *InvalidError) Unwrap() error { return platform.ErrInvalid }

// ConflictError: o pedido conflita com o que existe (vira 409).
type ConflictError struct{ Msg string }

func (e *ConflictError) Error() string { return e.Msg }
