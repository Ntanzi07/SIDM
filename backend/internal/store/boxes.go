package store

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"SIDM/internal/boxes"
)

// BoxSet são as caixas (bounding boxes) de UMA imagem em UMA tarefa, vindas
// do CVAT. Coleção própria (image_boxes), uma entrada por (tarefa, imagem):
// os metadados da imagem e as caixas têm ciclos de vida diferentes, e a mesma
// imagem pode estar em várias tarefas.
//
// Boxes vazio = a imagem foi anotada na tarefa e não tem nenhum objeto
// (negativa): entra no export como imagem sem caixas.
type BoxSet struct {
	ID         any         `bson:"_id,omitempty" json:"-"`
	Task       string      `bson:"task" json:"task"`
	ImageID    string      `bson:"image" json:"sha1"`
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

// ensureBoxIndexes migra as caixas de antes das tarefas e cria os índices.
// Idempotente: roda a cada subida do servidor.
//
// Antes, cada imagem tinha um conjunto só, com _id = sha1. Agora a chave é
// (task, image). A migração põe essas caixas antigas na tarefa "padrao"
// (criada com as labels delas); o _id antigo fica como está (o Mongo não
// deixa mudar _id, e nada depende dele).
func (s *Store) ensureBoxIndexes(ctx context.Context) error {
	res, err := s.boxes().UpdateMany(ctx,
		bson.D{{Key: "task", Value: bson.D{{Key: "$exists", Value: false}}}},
		mongo.Pipeline{{{Key: "$set", Value: bson.D{
			{Key: "task", Value: DefaultTask},
			{Key: "image", Value: "$_id"},
		}}}})
	if err != nil {
		return fmt.Errorf("migrar caixas para a tarefa %q: %w", DefaultTask, err)
	}
	if res.ModifiedCount > 0 {
		if err := s.ensureDefaultTask(ctx); err != nil {
			return err
		}
	}

	_, err = s.boxes().Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "task", Value: 1}, {Key: "image", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "image", Value: 1}}},
		{Keys: bson.D{{Key: "package", Value: 1}}}, // apagar/renomear pacote
	})
	if err != nil {
		return fmt.Errorf("índices de image_boxes: %w", err)
	}
	return nil
}

// ensureDefaultTask cria a tarefa "padrao" (se não existe) com as labels das
// caixas migradas.
func (s *Store) ensureDefaultTask(ctx context.Context) error {
	var labels []string
	if err := s.boxes().Distinct(ctx, "boxes.label", bson.D{{Key: "task", Value: DefaultTask}}).Decode(&labels); err != nil {
		return fmt.Errorf("labels das caixas migradas: %w", err)
	}
	if labels == nil {
		labels = []string{}
	}
	now := time.Now().UTC()
	err := s.CreateTask(ctx, &Task{
		Name: DefaultTask, Labels: labels, CreatedBy: "migração", CreatedAt: now, UpdatedAt: now,
		Description: "Caixas importadas antes de existirem tarefas.",
	})
	if err == ErrDuplicate {
		return s.AddTaskLabels(ctx, DefaultTask, labels, now)
	}
	return err
}

// ReplaceBoxSets grava (ou substitui) as caixas de várias imagens, cada uma
// na sua tarefa.
func (s *Store) ReplaceBoxSets(ctx context.Context, sets []BoxSet) error {
	const batch = 500
	for start := 0; start < len(sets); start += batch {
		end := min(start+batch, len(sets))
		models := make([]mongo.WriteModel, 0, end-start)
		for i := start; i < end; i++ {
			set := sets[i]
			set.ID = nil // o replace mantém o _id do documento existente
			if set.Boxes == nil {
				set.Boxes = []boxes.Box{} // negativa: [] e não null
			}
			models = append(models, mongo.NewReplaceOneModel().
				SetFilter(bson.D{{Key: "task", Value: set.Task}, {Key: "image", Value: set.ImageID}}).
				SetReplacement(set).
				SetUpsert(true))
		}
		if _, err := s.boxes().BulkWrite(ctx, models, options.BulkWrite().SetOrdered(false)); err != nil {
			return fmt.Errorf("gravar caixas: %w", err)
		}
	}
	return nil
}

// FindImageBoxSets devolve as caixas de uma imagem em todas as tarefas.
func (s *Store) FindImageBoxSets(ctx context.Context, sha1 string) ([]BoxSet, error) {
	cur, err := s.boxes().Find(ctx, bson.D{{Key: "image", Value: sha1}}, options.Find().SetSort(bson.D{{Key: "task", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("buscar caixas de %s: %w", sha1, err)
	}
	out := []BoxSet{}
	if err := cur.All(ctx, &out); err != nil {
		return nil, fmt.Errorf("ler caixas de %s: %w", sha1, err)
	}
	return out, nil
}

// FindBoxSets devolve as caixas de várias imagens numa tarefa (sha1 -> caixas).
func (s *Store) FindBoxSets(ctx context.Context, task string, ids []string) (map[string]BoxSet, error) {
	out := make(map[string]BoxSet, len(ids))
	const batch = 1000
	for start := 0; start < len(ids); start += batch {
		end := min(start+batch, len(ids))
		cur, err := s.boxes().Find(ctx, bson.D{
			{Key: "task", Value: task},
			{Key: "image", Value: bson.D{{Key: "$in", Value: ids[start:end]}}},
		})
		if err != nil {
			return nil, fmt.Errorf("buscar caixas: %w", err)
		}
		var sets []BoxSet
		if err := cur.All(ctx, &sets); err != nil {
			return nil, fmt.Errorf("ler caixas: %w", err)
		}
		for _, set := range sets {
			out[set.ImageID] = set
		}
	}
	return out, nil
}

// DeleteImageBoxSets apaga as caixas de uma imagem em todas as tarefas
// (quando ela é excluída).
func (s *Store) DeleteImageBoxSets(ctx context.Context, sha1 string) error {
	if _, err := s.boxes().DeleteMany(ctx, bson.D{{Key: "image", Value: sha1}}); err != nil {
		return fmt.Errorf("apagar caixas de %s: %w", sha1, err)
	}
	return nil
}

// PackageTaskCount: quantas imagens de um pacote estão numa tarefa.
type PackageTaskCount struct {
	Task   string `bson:"_id" json:"task"`
	Images int64  `bson:"images" json:"images"`
	Boxes  int64  `bson:"boxes" json:"boxes"`
}

// PackageBoxCounts conta, por tarefa, as imagens do pacote anotadas nela.
func (s *Store) PackageBoxCounts(ctx context.Context, pkg string) ([]PackageTaskCount, error) {
	cur, err := s.boxes().Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: bson.D{{Key: "package", Value: pkg}}}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$task"},
			{Key: "images", Value: bson.D{{Key: "$sum", Value: 1}}},
			{Key: "boxes", Value: bson.D{{Key: "$sum", Value: bson.D{{Key: "$size", Value: bson.D{{Key: "$ifNull", Value: bson.A{"$boxes", bson.A{}}}}}}}}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
	})
	if err != nil {
		return nil, fmt.Errorf("contar caixas do pacote %s: %w", pkg, err)
	}
	out := []PackageTaskCount{}
	if err := cur.All(ctx, &out); err != nil {
		return nil, fmt.Errorf("ler contagem de caixas do pacote %s: %w", pkg, err)
	}
	return out, nil
}
