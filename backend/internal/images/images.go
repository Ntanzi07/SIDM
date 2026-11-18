// Package images é o módulo das IMAGENS: upload (com o hash calculado na
// gravação e as imagens criptografadas abertas), o INI embutido no JPEG, a
// fila de anotação, a verificação (que renomeia o arquivo para o nome v2), o
// histórico de alterações e a exclusão.
//
// O módulo é dono das coleções images e image_history. Ele faz o lado das
// imagens do que os outros módulos precisam: o catálogo (progresso dos
// pacotes, cascatas de pacote e de equipamento/câmera: catalog.Images) e as
// caixas (achar imagens, quais estão verificadas: boxes.Images). O que ele
// precisa dos outros pede por interfaces (Catalog, Profiles) e avisa quando
// uma imagem é apagada (Listener).
//
// Os campos de anotação (veículo, direção, período…) ainda são fixos: o
// perfil de anotação configurável vai substituir isso (ver annotate.go, onde
// eles estão concentrados).
package images

import (
	"context"
	"errors"
	"time"

	"SIDM/internal/catalog"
	"SIDM/internal/importer"
)

const (
	StatusPending  = "pending"  // no pacote com nome = hash, aguardando verificação
	StatusVerified = "verified" // anotada e conferida, renomeada para o padrão v2
)

type Vehicle struct {
	Plate         string `bson:"plate" json:"plate"`
	PlateType     string `bson:"plate_type" json:"plate_type"`
	PlateCategory string `bson:"plate_category" json:"plate_category"`
	Type          string `bson:"type" json:"type"`
	Color         string `bson:"color" json:"color"`
}

// Image é o documento da collection images.
// Campos de anotação ficam vazios (omitempty) enquanto a imagem está pending,
// a não ser que o nome original já viesse no padrão v2 (pré-preenchido).
type Image struct {
	SHA1         string `bson:"_id" json:"sha1"`
	Package      string `bson:"package" json:"package"` // cada imagem pertence a UM pacote
	Status       string `bson:"status" json:"status"`
	OriginalName string `bson:"original_name" json:"original_name"`
	FileName     string `bson:"file_name,omitempty" json:"file_name,omitempty"` // nome v2, quando verified
	Path         string `bson:"path" json:"path"`                               // relativo a IMAGES_DIR
	Ext          string `bson:"ext" json:"ext"`
	Size         int64  `bson:"size" json:"size"`
	// Encryption: se o arquivo chegou criptografado, qual formato foi aberto
	// (ex: "rc4-legacy"). O que fica no disco é sempre o original aberto.
	Encryption string `bson:"encryption,omitempty" json:"encryption,omitempty"`

	PatternVersion string     `bson:"pattern_version,omitempty" json:"pattern_version,omitempty"`
	Equipment      string     `bson:"equipment,omitempty" json:"equipment,omitempty"`
	Camera         string     `bson:"camera,omitempty" json:"camera,omitempty"`
	Direction      string     `bson:"direction,omitempty" json:"direction,omitempty"`
	Resolution     string     `bson:"resolution,omitempty" json:"resolution,omitempty"`
	CaptureAngle   string     `bson:"capture_angle,omitempty" json:"capture_angle,omitempty"`
	DayTime        string     `bson:"day_time,omitempty" json:"day_time,omitempty"`
	ImageRange     string     `bson:"image_range,omitempty" json:"image_range,omitempty"`
	Weather        string     `bson:"weather,omitempty" json:"weather,omitempty"`
	Conditions     []string   `bson:"conditions,omitempty" json:"conditions,omitempty"`
	CapturedAt     *time.Time `bson:"captured_at,omitempty" json:"captured_at,omitempty"` // ponteiro: pending não tem data
	Vehicles       []Vehicle  `bson:"vehicles,omitempty" json:"vehicles,omitempty"`

	// INI: os metadados que o equipamento gravou DENTRO do arquivo (depois do
	// fim do JPEG). Cópia para busca e para os perfis de import; o arquivo
	// original continua intacto no disco. A assinatura não é guardada.
	INI []importer.INIEntry `bson:"ini,omitempty" json:"ini,omitempty"`

	// Fila de trabalho: quem está com a imagem e até quando (ver ClaimNext).
	// Sem claimed_until, ou com ele no passado, a imagem está livre.
	ClaimedBy    string     `bson:"claimed_by,omitempty" json:"claimed_by,omitempty"`
	ClaimedUntil *time.Time `bson:"claimed_until,omitempty" json:"claimed_until,omitempty"`
	// QueuedAt define a ordem da fila. Começa igual ao ingested_at; quando
	// alguém pula a imagem, vira "agora" e ela vai para o fim da fila.
	QueuedAt time.Time `bson:"queued_at" json:"queued_at"`

	IngestedAt time.Time `bson:"ingested_at" json:"ingested_at"`
	UpdatedAt  time.Time `bson:"updated_at" json:"updated_at"`
}

// ClaimedByOther diz se outra pessoa está com a imagem agora.
func (img *Image) ClaimedByOther(user string, now time.Time) bool {
	return img.ClaimedBy != "" && img.ClaimedBy != user &&
		img.ClaimedUntil != nil && img.ClaimedUntil.After(now)
}

// ErrConflict indica que o documento mudou desde que foi lido.
var ErrConflict = errors.New("alterado por outra pessoa")

// ImageFilter: campos vazios são ignorados.
type ImageFilter struct {
	Package      string
	Status       string
	Equipment    string
	Camera       string
	DayTime      string
	Weather      string
	Direction    string
	VehicleType  string
	VehicleColor string
	Plate        string
	Limit        int64
	Skip         int64
}

// HistoryEntry é um documento da collection image_history.
type HistoryEntry struct {
	ImageID   string    `bson:"image_id" json:"image_id"`
	Field     string    `bson:"field" json:"field"`
	Old       any       `bson:"old" json:"old"`
	New       any       `bson:"new" json:"new"`
	ChangedBy string    `bson:"changed_by" json:"changed_by"`
	ChangedAt time.Time `bson:"changed_at" json:"changed_at"`
}

// ExistingImage diz onde uma imagem já cadastrada está e em que estado.
type ExistingImage struct {
	Package string
	Status  string
}

// Repo são as coleções do módulo (Mongo em produção, memória nos testes).
type Repo interface {
	InsertImage(ctx context.Context, img *Image) error
	FindImage(ctx context.Context, sha1 string) (*Image, error)
	ReplaceImage(ctx context.Context, img *Image, prevUpdatedAt time.Time) error
	ListImages(ctx context.Context, f ImageFilter) ([]Image, int64, error)
	DeleteImage(ctx context.Context, sha1 string) error
	DeleteImageHistory(ctx context.Context, sha1 string) error
	InsertHistory(ctx context.Context, entries []HistoryEntry) error
	ExistingImages(ctx context.Context, ids []string) (map[string]ExistingImage, error)
	FindImagesByKeys(ctx context.Context, pkg, by string, keys []string) ([]Image, error)
	ClaimNext(ctx context.Context, pkg, user, skip string, lease time.Duration, now time.Time) (*Image, error)
	ReleaseClaim(ctx context.Context, sha1, user string) error
	VerifiedIDs(ctx context.Context, ids []string) (map[string]bool, error)

	PackageProgress(ctx context.Context, names []string) (map[string]catalog.Progress, error)
	RenamePackageImages(ctx context.Context, oldName, newName string) (int64, error)
	DeletePackageImages(ctx context.Context, name string) (int64, error)
	CountUsing(ctx context.Context, reg catalog.Registry, name string) (int64, error)
	RenameOnPending(ctx context.Context, reg catalog.Registry, oldName, newName string, now time.Time) (int64, error)
}

// Catalog é o que o módulo precisa do catálogo (o *catalog.Service).
type Catalog interface {
	FindPackage(ctx context.Context, name string) (*catalog.Package, error)
	EquipmentExists(ctx context.Context, name string) (bool, error)
	CameraExists(ctx context.Context, name string) (bool, error)
	CreateEquipment(ctx context.Context, name string) error
	CreateCamera(ctx context.Context, name string) error
}

// Profiles: o perfil de import (jpeg-ini) aplicado no upload.
type Profiles interface {
	FindProfile(ctx context.Context, name string) (*importer.Profile, error)
}

// Listener é avisado quando uma imagem é apagada (ex.: as caixas dela).
type Listener interface {
	DeleteImage(ctx context.Context, sha1 string) error
}
