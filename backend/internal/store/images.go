package store

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

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

// InsertImage grava uma imagem nova. Retorna ErrDuplicate se o sha1 já existe:
// como _id = sha1, o próprio Mongo impede a mesma imagem duas vezes.
func (s *Store) InsertImage(ctx context.Context, img *Image) error {
	_, err := s.db.Collection("images").InsertOne(ctx, img)
	if mongo.IsDuplicateKeyError(err) {
		return ErrDuplicate
	}
	if err != nil {
		return fmt.Errorf("inserir imagem: %w", err)
	}
	return nil
}

// DeleteImageHistory apaga o histórico de uma imagem (quando ela é excluída).
func (s *Store) DeleteImageHistory(ctx context.Context, sha1 string) error {
	_, err := s.db.Collection("image_history").DeleteMany(ctx, bson.D{{Key: "image_id", Value: sha1}})
	if err != nil {
		return fmt.Errorf("apagar histórico da imagem %s: %w", sha1, err)
	}
	return nil
}

// DeleteImage remove o documento. Usado ao excluir uma imagem e para desfazer
// um upload que falhou no disco.
func (s *Store) DeleteImage(ctx context.Context, sha1 string) error {
	_, err := s.db.Collection("images").DeleteOne(ctx, bson.D{{Key: "_id", Value: sha1}})
	if err != nil {
		return fmt.Errorf("remover imagem %s: %w", sha1, err)
	}
	return nil
}
