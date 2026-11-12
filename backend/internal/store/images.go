package store

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const (
	StatusPending  = "pending"  // em staging, aguardando anotação
	StatusReviewed = "reviewed" // anotada, em dataset com nome v2
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
	Status       string `bson:"status" json:"status"`
	OriginalName string `bson:"original_name" json:"original_name"`
	FileName     string `bson:"file_name,omitempty" json:"file_name,omitempty"` // nome v2, quando reviewed
	Path         string `bson:"path" json:"path"`                               // relativo a IMAGES_DIR
	Ext          string `bson:"ext" json:"ext"`
	Size         int64  `bson:"size" json:"size"`

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

	IngestedAt time.Time `bson:"ingested_at" json:"ingested_at"`
	UpdatedAt  time.Time `bson:"updated_at" json:"updated_at"`
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

// DeleteImage remove o documento (usado para desfazer um upload que falhou no disco).
func (s *Store) DeleteImage(ctx context.Context, sha1 string) error {
	_, err := s.db.Collection("images").DeleteOne(ctx, bson.D{{Key: "_id", Value: sha1}})
	if err != nil {
		return fmt.Errorf("remover imagem %s: %w", sha1, err)
	}
	return nil
}
