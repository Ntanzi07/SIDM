package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Task é uma tarefa de anotação de caixas (ex.: "capacete", "placa-letras").
// As caixas de uma imagem são guardadas POR TAREFA: a mesma imagem pode estar
// em várias tarefas sem uma sobrescrever a outra. Os pacotes de uma tarefa não
// são guardados: saem das imagens que foram anotadas nela (TaskStats).
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

func (s *Store) tasks() *mongo.Collection { return s.db.Collection("annotation_tasks") }

// ensureTaskIndexes: nome único. Idempotente.
func (s *Store) ensureTaskIndexes(ctx context.Context) error {
	_, err := s.tasks().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "name", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("índice de annotation_tasks: %w", err)
	}
	return nil
}

func (s *Store) CreateTask(ctx context.Context, t *Task) error {
	_, err := s.tasks().InsertOne(ctx, t)
	if mongo.IsDuplicateKeyError(err) {
		return ErrDuplicate
	}
	if err != nil {
		return fmt.Errorf("criar tarefa %s: %w", t.Name, err)
	}
	return nil
}

func (s *Store) FindTask(ctx context.Context, name string) (*Task, error) {
	var t Task
	err := s.tasks().FindOne(ctx, bson.D{{Key: "name", Value: name}}).Decode(&t)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("buscar tarefa %s: %w", name, err)
	}
	return &t, nil
}

// ListTasks: todas, por nome (são poucas).
func (s *Store) ListTasks(ctx context.Context) ([]Task, error) {
	cur, err := s.tasks().Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("listar tarefas: %w", err)
	}
	out := []Task{}
	if err := cur.All(ctx, &out); err != nil {
		return nil, fmt.Errorf("ler tarefas: %w", err)
	}
	return out, nil
}

// UpdateTask grava a tarefa t no lugar de oldName. Se o nome mudou, leva junto
// as caixas e os perfis de export que apontam para ela. Sem transação (o
// Mongo do projeto roda sem réplica), então a tarefa é a ÚLTIMA a mudar: se
// algo falhar no meio, ela ainda tem o nome antigo e repetir o rename termina
// o serviço.
func (s *Store) UpdateTask(ctx context.Context, oldName string, t *Task) error {
	if oldName != t.Name {
		if _, err := s.FindTask(ctx, t.Name); err == nil {
			return ErrDuplicate
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
		if _, err := s.FindTask(ctx, oldName); err != nil {
			return err
		}
		set := bson.D{{Key: "$set", Value: bson.D{{Key: "task", Value: t.Name}}}}
		if _, err := s.boxes().UpdateMany(ctx, bson.D{{Key: "task", Value: oldName}}, set); err != nil {
			return fmt.Errorf("renomear a tarefa nas caixas: %w", err)
		}
		if _, err := s.exportProfiles().UpdateMany(ctx, bson.D{{Key: "task", Value: oldName}}, set); err != nil {
			return fmt.Errorf("renomear a tarefa nos perfis de export: %w", err)
		}
	}
	res, err := s.tasks().ReplaceOne(ctx, bson.D{{Key: "name", Value: oldName}}, t)
	if mongo.IsDuplicateKeyError(err) {
		return ErrDuplicate
	}
	if err != nil {
		return fmt.Errorf("atualizar tarefa %s: %w", oldName, err)
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// AddTaskLabels acrescenta labels novas no fim da lista (as que já existem
// ficam onde estão).
func (s *Store) AddTaskLabels(ctx context.Context, name string, labels []string, now time.Time) error {
	if len(labels) == 0 {
		return nil
	}
	res, err := s.tasks().UpdateOne(ctx, bson.D{{Key: "name", Value: name}}, bson.D{
		{Key: "$addToSet", Value: bson.D{{Key: "labels", Value: bson.D{{Key: "$each", Value: labels}}}}},
		{Key: "$set", Value: bson.D{{Key: "updated_at", Value: now}}},
	})
	if err != nil {
		return fmt.Errorf("acrescentar labels na tarefa %s: %w", name, err)
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteTask apaga a tarefa e todas as caixas dela. Devolve quantas imagens
// tinham caixas nela. Os perfis de export que apontam para ela ficam (a
// prévia avisa que a tarefa não existe).
func (s *Store) DeleteTask(ctx context.Context, name string) (int64, error) {
	if _, err := s.FindTask(ctx, name); err != nil {
		return 0, err
	}
	// Caixas primeiro: se falhar, a tarefa continua e dá para repetir.
	res, err := s.boxes().DeleteMany(ctx, bson.D{{Key: "task", Value: name}})
	if err != nil {
		return 0, fmt.Errorf("apagar caixas da tarefa %s: %w", name, err)
	}
	if _, err := s.tasks().DeleteOne(ctx, bson.D{{Key: "name", Value: name}}); err != nil {
		return res.DeletedCount, fmt.Errorf("apagar tarefa %s: %w", name, err)
	}
	return res.DeletedCount, nil
}

// TaskStats é o resumo de uma tarefa.
type TaskStats struct {
	Images    int64            `json:"images"`    // imagens anotadas na tarefa
	Verified  int64            `json:"verified"`  // dessas, as verificadas (entram no export)
	Negatives int64            `json:"negatives"` // anotadas sem nenhuma caixa
	Boxes     int64            `json:"boxes"`
	Labels    map[string]int64 `json:"labels"` // caixas por label
	Packages  []TaskPackage    `json:"packages"`
}

// TaskPackage: quantas imagens de um pacote estão na tarefa.
type TaskPackage struct {
	Name     string `bson:"_id" json:"name"`
	Images   int64  `bson:"images" json:"images"`
	Verified int64  `bson:"verified" json:"verified"`
}

// TaskStats conta as imagens da tarefa por pacote (com quantas estão
// verificadas) e as caixas por label.
func (s *Store) TaskStats(ctx context.Context, task string) (*TaskStats, error) {
	match := bson.D{{Key: "$match", Value: bson.D{{Key: "task", Value: task}}}}
	cur, err := s.boxes().Aggregate(ctx, mongo.Pipeline{
		match,
		{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: "images"},
			{Key: "localField", Value: "image"},
			{Key: "foreignField", Value: "_id"},
			{Key: "as", Value: "img"},
		}}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$package"},
			{Key: "images", Value: bson.D{{Key: "$sum", Value: 1}}},
			{Key: "verified", Value: bson.D{{Key: "$sum", Value: bson.D{{Key: "$cond", Value: bson.A{
				bson.D{{Key: "$in", Value: bson.A{StatusVerified, "$img.status"}}}, 1, 0,
			}}}}}},
			{Key: "negatives", Value: bson.D{{Key: "$sum", Value: bson.D{{Key: "$cond", Value: bson.A{
				bson.D{{Key: "$eq", Value: bson.A{bson.D{{Key: "$size", Value: bson.D{{Key: "$ifNull", Value: bson.A{"$boxes", bson.A{}}}}}}, 0}}}, 1, 0,
			}}}}}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
	})
	if err != nil {
		return nil, fmt.Errorf("resumo da tarefa %s: %w", task, err)
	}
	var rows []struct {
		TaskPackage `bson:",inline"`
		Negatives   int64 `bson:"negatives"`
	}
	if err := cur.All(ctx, &rows); err != nil {
		return nil, fmt.Errorf("ler resumo da tarefa %s: %w", task, err)
	}
	st := &TaskStats{Labels: map[string]int64{}, Packages: make([]TaskPackage, 0, len(rows))}
	for _, r := range rows {
		st.Packages = append(st.Packages, r.TaskPackage)
		st.Images += r.Images
		st.Verified += r.Verified
		st.Negatives += r.Negatives
	}

	cur, err = s.boxes().Aggregate(ctx, mongo.Pipeline{
		match,
		{{Key: "$unwind", Value: "$boxes"}},
		{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$boxes.label"}, {Key: "n", Value: bson.D{{Key: "$sum", Value: 1}}}}}},
	})
	if err != nil {
		return nil, fmt.Errorf("contar caixas da tarefa %s: %w", task, err)
	}
	var labels []struct {
		Label string `bson:"_id"`
		N     int64  `bson:"n"`
	}
	if err := cur.All(ctx, &labels); err != nil {
		return nil, fmt.Errorf("ler caixas da tarefa %s: %w", task, err)
	}
	for _, l := range labels {
		st.Labels[l.Label] = l.N
		st.Boxes += l.N
	}
	return st, nil
}
