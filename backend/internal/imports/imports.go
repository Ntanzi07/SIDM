// Package imports é o módulo de import de anotações: os perfis (como ler um
// CSV, ou o INI embutido nas fotos, e traduzir para os campos da imagem), o
// raio-x de um arquivo novo e o import em si, que preenche o RASCUNHO das
// imagens pendentes de um pacote.
//
// A leitura e a tradução ficam no pacote importer, que é puro (sem banco e
// sem HTTP) e é usado também pelo upload. Aqui fica o que tem banco e HTTP.
// A coleção deste módulo é a import_profiles; as imagens e o cadastro são
// mexidos pelos Services deles (interfaces Images e Catalog).
package imports

import (
	"context"
	"time"

	"SIDM/internal/annotation"
	"SIDM/internal/catalog"
	"SIDM/internal/images"
	"SIDM/internal/importer"
)

// Repo é a coleção import_profiles.
//
// O MongoRepo também serve para o módulo images (que só precisa do
// FindProfile para aplicar o perfil de INI no upload), sem depender do
// Service daqui.
type Repo interface {
	CreateProfile(ctx context.Context, p *importer.Profile) error
	FindProfile(ctx context.Context, name string) (*importer.Profile, error)
	ReplaceProfile(ctx context.Context, p *importer.Profile) error
	RenameProfile(ctx context.Context, oldName, newName string, now time.Time) error
	DeleteProfile(ctx context.Context, name string) error
	ListProfiles(ctx context.Context, q string, limit, skip int64) ([]importer.Profile, int64, error)
}

// Images é o que o import usa do módulo images.
type Images interface {
	List(ctx context.Context, f images.ImageFilter) ([]images.Image, int64, error)
	FindByKeys(ctx context.Context, pkg, by string, keys []string) ([]images.Image, error)
	Replace(ctx context.Context, img *images.Image, prevUpdatedAt time.Time) error
	InsertHistory(ctx context.Context, entries []images.HistoryEntry) error
	// ProfileOf: o perfil de anotação do pacote (o import só preenche o que
	// ele tem).
	ProfileOf(ctx context.Context, pkg string) (*annotation.Profile, error)
}

// Catalog é o que o import usa do catálogo: o pacote tem que existir, e os
// equipamentos e câmeras novos do CSV são cadastrados.
type Catalog interface {
	FindPackage(ctx context.Context, name string) (*catalog.Package, error)
	EquipmentExists(ctx context.Context, name string) (bool, error)
	CameraExists(ctx context.Context, name string) (bool, error)
	CreateEquipment(ctx context.Context, name string) error
	CreateCamera(ctx context.Context, name string) error
}

// Run é um pedido de import.
type Run struct {
	Package   string
	Profile   *importer.Profile
	User      string
	DryRun    bool // só o relatório, nada é gravado
	Overwrite bool // o valor do CSV substitui o do rascunho (senão, só preenche vazios)
}

// Report é a resposta do import (e do ensaio).
type Report struct {
	DryRun    bool   `json:"dry_run"`
	Package   string `json:"package"`
	Profile   string `json:"profile"`
	Overwrite bool   `json:"overwrite"`

	importer.Report // linhas lidas, valores sem tradução, erros de data...

	WithoutINI      int      `json:"without_ini"`      // perfil de INI: imagens pendentes sem INI embutido
	Matched         int      `json:"matched"`          // linhas que acharam a imagem
	NotFound        []string `json:"not_found"`        // chaves sem imagem no pacote (até 100)
	NotFoundCount   int      `json:"not_found_count"`  //
	Ambiguous       []string `json:"ambiguous"`        // nome original repetido no pacote: não aplicado
	SkippedVerified int      `json:"skipped_verified"` // já verificadas: o import não mexe
	SkippedClaimed  int      `json:"skipped_claimed"`  // alguém está anotando agora
	Unchanged       int      `json:"unchanged"`        // o import não tinha nada novo
	Applied         int      `json:"applied"`          // rascunhos atualizados (no ensaio: "seriam")
	Conflicts       int      `json:"conflicts"`        // alterada por alguém durante o import

	// Equipamentos e câmeras do CSV que ainda não existiam no cadastro.
	// Ao aplicar, são CRIADOS automaticamente; no ensaio, só listados.
	NewEquipments []string `json:"new_equipments"`
	NewCameras    []string `json:"new_cameras"`
	// Nomes que não dá para sanitizar (ex: "8003/MF"): não são criados, e o
	// campo fica vazio na imagem. Corrija no CSV ou use um "map" no perfil.
	InvalidNames []RowName `json:"invalid_names"`
	// Valores que o perfil de anotação do pacote não tem ou não aceita (ex.:
	// clima num pacote de perfil sem clima): ficam de fora (até 100).
	OutsideProfile      []string `json:"outside_profile"`
	OutsideProfileCount int      `json:"outside_profile_count"`
}

// RowName é um nome de equipamento/câmera do CSV que não deu para usar.
type RowName struct {
	Line  int    `json:"line"`
	Field string `json:"field"`
	Value string `json:"value"`
	Error string `json:"error"`
}

// InvalidError é um pedido que não dá para atender como veio (vira 400).
type InvalidError struct{ Msg string }

func (e *InvalidError) Error() string { return e.Msg }
