// Package catalog é o cadastro do que organiza as imagens: os PACOTES (lotes
// de imagens, cada um uma pasta em dataset/) e os EQUIPAMENTOS e CÂMERAS
// (os cadastros usados na anotação e no nome v2).
//
// O catálogo é dono das coleções packages, equipments e cameras, e de mais
// nenhuma. O que acontece com as imagens quando um pacote é renomeado ou
// apagado, ou quando um equipamento muda de nome, não é com ele: ele avisa
// quem cuida delas (interfaces Images e PackageListener).
package catalog

import (
	"context"
	"time"
)

// Package é um lote de imagens enviado junto. Vira uma pasta em dataset/.
type Package struct {
	Name      string    `bson:"name" json:"name"`
	CreatedBy string    `bson:"created_by" json:"created_by"`
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
}

// Progress é o andamento da verificação de um pacote.
type Progress struct {
	Total      int64 `json:"total"`
	Verified   int64 `json:"verified"`
	Pending    int64 `json:"pending"`
	InProgress int64 `json:"in_progress"` // pendentes que alguém está anotando agora
}

// PackageSummary é o pacote com o progresso (o item de GET /packages).
type PackageSummary struct {
	Package
	Progress
}

// Registry é um dos cadastros simples: equipamentos ou câmeras. Os dois
// funcionam igual; só mudam a coleção e o campo na imagem.
type Registry string

const (
	Equipments Registry = "equipments"
	Cameras    Registry = "cameras"
)

// ImageField é o campo da imagem que aponta para este cadastro.
func (r Registry) ImageField() string {
	if r == Cameras {
		return "camera"
	}
	return "equipment"
}

// RegistryEntry é um item de cadastro.
type RegistryEntry struct {
	Name   string `bson:"name" json:"name"`
	Active bool   `bson:"active" json:"active"`
}

// RenameResult é o que aconteceu nas imagens ao renomear um equipamento ou
// câmera.
type RenameResult struct {
	PendingUpdated  int64    // pendentes que trocaram de nome (só no banco)
	VerifiedRenamed int      // verificadas: arquivo renomeado para o novo nome v2
	Failed          []string // sha1 das que não deu para renomear agora
}

// Repo são as coleções do catálogo (Mongo em produção, memória nos testes).
type Repo interface {
	CreatePackage(ctx context.Context, p Package) error
	FindPackage(ctx context.Context, name string) (*Package, error)
	// ListPackages: mais novos primeiro; q filtra por nome (contém).
	ListPackages(ctx context.Context, q string, limit, skip int64) ([]Package, int64, error)
	RenamePackage(ctx context.Context, oldName, newName string) error
	DeletePackage(ctx context.Context, name string) error

	// Search: os ATIVOS cujo nome contém q, em ordem alfabética.
	Search(ctx context.Context, reg Registry, q string, limit, skip int64) ([]string, int64, error)
	Create(ctx context.Context, reg Registry, name string) error
	Find(ctx context.Context, reg Registry, name string) (*RegistryEntry, error)
	ActiveExists(ctx context.Context, reg Registry, name string) (bool, error)
	SetActive(ctx context.Context, reg Registry, name string, active bool) error
	Delete(ctx context.Context, reg Registry, name string) error
}

// Images é o que o catálogo precisa de quem cuida das imagens.
type Images interface {
	// PackageProgress: o andamento de cada pacote pedido (ausente = vazio).
	PackageProgress(ctx context.Context, names []string) (map[string]Progress, error)
	// RenamePackageImages leva as imagens (pacote e caminho) para o nome novo.
	RenamePackageImages(ctx context.Context, oldName, newName string) (int64, error)
	// DeletePackageImages apaga as imagens do pacote e o histórico delas.
	DeletePackageImages(ctx context.Context, name string) (int64, error)
	// CountUsing: quantas imagens (de qualquer status) usam o item.
	CountUsing(ctx context.Context, reg Registry, name string) (int64, error)
	// RenameRegistry troca o nome nas imagens: nas pendentes só no banco; nas
	// verificadas, renomeia também o arquivo (o nome v2 tem o equipamento).
	RenameRegistry(ctx context.Context, reg Registry, oldName, newName, user string) (RenameResult, error)
}

// PackageListener é avisado quando um pacote muda (ex.: as caixas do CVAT,
// que guardam o pacote de cada imagem).
type PackageListener interface {
	PackageRenamed(ctx context.Context, oldName, newName string) error
	PackageDeleted(ctx context.Context, name string) error
}

// Dirs são as pastas dos pacotes no disco (o *files.Storage).
type Dirs interface {
	EnsurePackageDir(name string) error
	RenamePackageDir(oldName, newName string) error
	RemovePackageDir(name string) error
}
