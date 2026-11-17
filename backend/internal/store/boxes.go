package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"SIDM/internal/boxes"
)

// BoxSet são as caixas (bounding boxes) de UMA imagem, vindas do CVAT.
// Coleção própria (image_boxes), com _id = sha1 da imagem: os metadados da
// imagem e as caixas têm ciclos de vida diferentes (reimportar as caixas não
// mexe na anotação, e vice-versa).
type BoxSet struct {
	ImageID    string      `bson:"_id" json:"sha1"`
	Package    string      `bson:"package" json:"package"`
	Width      int         `bson:"width" json:"width"` // tamanho da imagem no CVAT (as caixas são em pixels dela)
	Height     int         `bson:"height" json:"height"`
	Boxes      []boxes.Box `bson:"boxes" json:"boxes"`
	Source     BoxSource   `bson:"source" json:"source"`
	ImportedBy string      `bson:"imported_by" json:"imported_by"`
	ImportedAt time.Time   `bson:"imported_at" json:"imported_at"`
}

// BoxSource diz de onde as caixas vieram.
type BoxSource struct {
	Tool     string `bson:"tool" json:"tool"`           // "cvat"
	Format   string `bson:"format" json:"format"`       // "coco"
	File     string `bson:"file" json:"file"`           // nome do arquivo importado
	FileName string `bson:"file_name" json:"file_name"` // nome da imagem dentro do arquivo
}

func (s *Store) boxes() *mongo.Collection { return s.db.Collection("image_boxes") }

// ensureBoxIndexes: o índice por pacote (apagar/renomear pacote, exportar).
// CreateOne é idempotente, então pode rodar a cada subida do servidor; assim
// funciona também num banco criado antes desta coleção existir.
func (s *Store) ensureBoxIndexes(ctx context.Context) error {
	_, err := s.boxes().Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "package", Value: 1}}})
	if err != nil {
		return fmt.Errorf("índice de image_boxes: %w", err)
	}
	return nil
}

// ReplaceBoxSets grava (ou substitui) as caixas de várias imagens.
func (s *Store) ReplaceBoxSets(ctx context.Context, sets []BoxSet) error {
	const batch = 500
	for start := 0; start < len(sets); start += batch {
		end := min(start+batch, len(sets))
		models := make([]mongo.WriteModel, 0, end-start)
		for i := start; i < end; i++ {
			models = append(models, mongo.NewReplaceOneModel().
				SetFilter(bson.D{{Key: "_id", Value: sets[i].ImageID}}).
				SetReplacement(sets[i]).
				SetUpsert(true))
		}
		if _, err := s.boxes().BulkWrite(ctx, models, options.BulkWrite().SetOrdered(false)); err != nil {
			return fmt.Errorf("gravar caixas: %w", err)
		}
	}
	return nil
}

// FindBoxSet devolve as caixas de uma imagem (ErrNotFound se não tem).
func (s *Store) FindBoxSet(ctx context.Context, sha1 string) (*BoxSet, error) {
	var set BoxSet
	err := s.boxes().FindOne(ctx, bson.D{{Key: "_id", Value: sha1}}).Decode(&set)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("buscar caixas de %s: %w", sha1, err)
	}
	return &set, nil
}

// BoxImageIDs diz quais destas imagens já têm caixas.
func (s *Store) BoxImageIDs(ctx context.Context, ids []string) (map[string]bool, error) {
	out := map[string]bool{}
	const batch = 1000
	for start := 0; start < len(ids); start += batch {
		end := min(start+batch, len(ids))
		cur, err := s.boxes().Find(ctx,
			bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: ids[start:end]}}}},
			options.Find().SetProjection(bson.D{{Key: "_id", Value: 1}}))
		if err != nil {
			return nil, fmt.Errorf("buscar caixas existentes: %w", err)
		}
		var docs []struct {
			ID string `bson:"_id"`
		}
		if err := cur.All(ctx, &docs); err != nil {
			return nil, fmt.Errorf("ler caixas existentes: %w", err)
		}
		for _, d := range docs {
			out[d.ID] = true
		}
	}
	return out, nil
}

// DeleteBoxSet apaga as caixas de uma imagem (quando ela é excluída).
func (s *Store) DeleteBoxSet(ctx context.Context, sha1 string) error {
	if _, err := s.boxes().DeleteOne(ctx, bson.D{{Key: "_id", Value: sha1}}); err != nil {
		return fmt.Errorf("apagar caixas de %s: %w", sha1, err)
	}
	return nil
}

// BoxLabelCounts conta as caixas por label num pacote (o resumo do pacote).
func (s *Store) BoxLabelCounts(ctx context.Context, pkg string) (images int64, labels map[string]int64, err error) {
	images, err = s.boxes().CountDocuments(ctx, bson.D{{Key: "package", Value: pkg}})
	if err != nil {
		return 0, nil, fmt.Errorf("contar imagens com caixas: %w", err)
	}
	cur, err := s.boxes().Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: bson.D{{Key: "package", Value: pkg}}}},
		{{Key: "$unwind", Value: "$boxes"}},
		{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$boxes.label"}, {Key: "n", Value: bson.D{{Key: "$sum", Value: 1}}}}}},
	})
	if err != nil {
		return 0, nil, fmt.Errorf("contar caixas por label: %w", err)
	}
	var rows []struct {
		Label string `bson:"_id"`
		N     int64  `bson:"n"`
	}
	if err := cur.All(ctx, &rows); err != nil {
		return 0, nil, fmt.Errorf("ler contagem de caixas: %w", err)
	}
	labels = make(map[string]int64, len(rows))
	for _, r := range rows {
		labels[r.Label] = r.N
	}
	return images, labels, nil
}
