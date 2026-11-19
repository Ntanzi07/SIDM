package boxes

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"SIDM/internal/platform"
)

// MongoRepo guarda as tarefas (annotation_tasks) e as caixas (image_boxes).
type MongoRepo struct {
	db *mongo.Database
}

func NewMongoRepo(db *mongo.Database) *MongoRepo { return &MongoRepo{db: db} }

func (m *MongoRepo) tasks() *mongo.Collection { return m.db.Collection("annotation_tasks") }
func (m *MongoRepo) sets() *mongo.Collection  { return m.db.Collection("image_boxes") }

// EnsureIndexes cria os índices e migra as caixas de antes das tarefas.
// Idempotente: roda a cada subida do servidor.
//
// Antes, cada imagem tinha um conjunto só, com _id = sha1. Agora a chave é
// (task, image). A migração põe essas caixas antigas na tarefa "padrao"
// (criada com as labels delas); o _id antigo fica como está (o Mongo não
// deixa mudar _id, e nada depende dele). A migração vem ANTES do índice
// único: as caixas antigas não têm task nem image.
func (m *MongoRepo) EnsureIndexes(ctx context.Context) error {
	if _, err := m.tasks().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "name", Value: 1}},
		Options: options.Index().SetUnique(true),
	}); err != nil {
		return fmt.Errorf("índice de annotation_tasks: %w", err)
	}

	res, err := m.sets().UpdateMany(ctx,
		bson.D{{Key: "task", Value: bson.D{{Key: "$exists", Value: false}}}},
		mongo.Pipeline{{{Key: "$set", Value: bson.D{
			{Key: "task", Value: DefaultTask},
			{Key: "image", Value: "$_id"},
		}}}})
	if err != nil {
		return fmt.Errorf("migrar caixas para a tarefa %q: %w", DefaultTask, err)
	}
	if res.ModifiedCount > 0 {
		if err := m.ensureDefaultTask(ctx); err != nil {
			return err
		}
	}

	if _, err := m.sets().Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "task", Value: 1}, {Key: "image", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "image", Value: 1}}},
		{Keys: bson.D{{Key: "package", Value: 1}}}, // apagar/renomear pacote
	}); err != nil {
		return fmt.Errorf("índices de image_boxes: %w", err)
	}
	return nil
}

// ensureDefaultTask cria a tarefa "padrao" (se não existe) com as labels das
// caixas migradas.
func (m *MongoRepo) ensureDefaultTask(ctx context.Context) error {
	var labels []string
	if err := m.sets().Distinct(ctx, "boxes.label", bson.D{{Key: "task", Value: DefaultTask}}).Decode(&labels); err != nil {
		return fmt.Errorf("labels das caixas migradas: %w", err)
	}
	if labels == nil {
		labels = []string{}
	}
	now := time.Now().UTC()
	err := m.CreateTask(ctx, &Task{
		Name: DefaultTask, Labels: labels, CreatedBy: "migração", CreatedAt: now, UpdatedAt: now,
		Description: "Caixas importadas antes de existirem tarefas.",
	})
	if errors.Is(err, platform.ErrDuplicate) {
		return m.AddTaskLabels(ctx, DefaultTask, labels, now)
	}
	return err
}

// ---------- tarefas ----------

func (m *MongoRepo) CreateTask(ctx context.Context, t *Task) error {
	_, err := m.tasks().InsertOne(ctx, t)
	if mongo.IsDuplicateKeyError(err) {
		return platform.ErrDuplicate
	}
	if err != nil {
		return fmt.Errorf("criar tarefa %s: %w", t.Name, err)
	}
	return nil
}

func (m *MongoRepo) FindTask(ctx context.Context, name string) (*Task, error) {
	var t Task
	err := m.tasks().FindOne(ctx, bson.D{{Key: "name", Value: name}}).Decode(&t)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, platform.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("buscar tarefa %s: %w", name, err)
	}
	return &t, nil
}

// ListTasks: todas, por nome (são poucas).
func (m *MongoRepo) ListTasks(ctx context.Context) ([]Task, error) {
	cur, err := m.tasks().Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("listar tarefas: %w", err)
	}
	out := []Task{}
	if err := cur.All(ctx, &out); err != nil {
		return nil, fmt.Errorf("ler tarefas: %w", err)
	}
	return out, nil
}

func (m *MongoRepo) ReplaceTask(ctx context.Context, oldName string, t *Task) error {
	res, err := m.tasks().ReplaceOne(ctx, bson.D{{Key: "name", Value: oldName}}, t)
	if mongo.IsDuplicateKeyError(err) {
		return platform.ErrDuplicate
	}
	if err != nil {
		return fmt.Errorf("atualizar tarefa %s: %w", oldName, err)
	}
	if res.MatchedCount == 0 {
		return platform.ErrNotFound
	}
	return nil
}

// AddTaskLabels acrescenta labels novas no fim da lista (as que já existem
// ficam onde estão).
func (m *MongoRepo) AddTaskLabels(ctx context.Context, name string, labels []string, now time.Time) error {
	if len(labels) == 0 {
		return nil
	}
	res, err := m.tasks().UpdateOne(ctx, bson.D{{Key: "name", Value: name}}, bson.D{
		{Key: "$addToSet", Value: bson.D{{Key: "labels", Value: bson.D{{Key: "$each", Value: labels}}}}},
		{Key: "$set", Value: bson.D{{Key: "updated_at", Value: now}}},
	})
	if err != nil {
		return fmt.Errorf("acrescentar labels na tarefa %s: %w", name, err)
	}
	if res.MatchedCount == 0 {
		return platform.ErrNotFound
	}
	return nil
}

func (m *MongoRepo) DeleteTask(ctx context.Context, name string) error {
	res, err := m.tasks().DeleteOne(ctx, bson.D{{Key: "name", Value: name}})
	if err != nil {
		return fmt.Errorf("apagar tarefa %s: %w", name, err)
	}
	if res.DeletedCount == 0 {
		return platform.ErrNotFound
	}
	return nil
}

// TaskImages: os sha1 da tarefa por pacote (só o _id e o pacote vêm do banco),
// quantas sem caixa, e as caixas por label (uma agregação).
func (m *MongoRepo) TaskImages(ctx context.Context, task string) (*TaskImages, error) {
	filter := bson.D{{Key: "task", Value: task}}
	cur, err := m.sets().Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: filter}},
		{{Key: "$project", Value: bson.D{
			{Key: "_id", Value: 0}, {Key: "image", Value: 1}, {Key: "package", Value: 1},
			{Key: "n", Value: bson.D{{Key: "$size", Value: bson.D{{Key: "$ifNull", Value: bson.A{"$boxes", bson.A{}}}}}}},
		}}},
	})
	if err != nil {
		return nil, fmt.Errorf("imagens da tarefa %s: %w", task, err)
	}
	var rows []struct {
		Image   string `bson:"image"`
		Package string `bson:"package"`
		N       int    `bson:"n"`
	}
	if err := cur.All(ctx, &rows); err != nil {
		return nil, fmt.Errorf("ler imagens da tarefa %s: %w", task, err)
	}
	out := &TaskImages{IDs: map[string][]string{}, Labels: map[string]int64{}}
	for _, r := range rows {
		out.IDs[r.Package] = append(out.IDs[r.Package], r.Image)
		if r.N == 0 {
			out.Negatives++
		}
	}

	cur, err = m.sets().Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: filter}},
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
		out.Labels[l.Label] = l.N
	}
	return out, nil
}

// ---------- caixas ----------

// ReplaceSets grava (ou substitui) as caixas de várias imagens, cada uma na
// sua tarefa.
func (m *MongoRepo) ReplaceSets(ctx context.Context, sets []Set) error {
	const batch = 500
	for start := 0; start < len(sets); start += batch {
		end := min(start+batch, len(sets))
		models := make([]mongo.WriteModel, 0, end-start)
		for i := start; i < end; i++ {
			set := sets[i]
			set.ID = nil // o replace mantém o _id do documento existente
			if set.Boxes == nil {
				set.Boxes = []Box{} // negativa: [] e não null
			}
			models = append(models, mongo.NewReplaceOneModel().
				SetFilter(bson.D{{Key: "task", Value: set.Task}, {Key: "image", Value: set.ImageID}}).
				SetReplacement(set).
				SetUpsert(true))
		}
		if _, err := m.sets().BulkWrite(ctx, models, options.BulkWrite().SetOrdered(false)); err != nil {
			return fmt.Errorf("gravar caixas: %w", err)
		}
	}
	return nil
}

// UpdateSet grava as caixas editadas de um Set, só se ele ainda está na
// versão prev (a última edição ou, sem edição, o import).
func (m *MongoRepo) UpdateSet(ctx context.Context, set *Set, prev time.Time) error {
	doc := *set
	doc.ID = nil
	filter := bson.D{
		{Key: "task", Value: set.Task}, {Key: "image", Value: set.ImageID},
		{Key: "$or", Value: bson.A{
			bson.D{{Key: "updated_at", Value: prev}},
			bson.D{{Key: "updated_at", Value: bson.D{{Key: "$exists", Value: false}}}, {Key: "imported_at", Value: prev}},
		}},
	}
	res, err := m.sets().ReplaceOne(ctx, filter, doc)
	if err != nil {
		return fmt.Errorf("gravar caixas de %s: %w", set.ImageID, err)
	}
	if res.MatchedCount == 0 {
		n, err := m.sets().CountDocuments(ctx, bson.D{{Key: "task", Value: set.Task}, {Key: "image", Value: set.ImageID}})
		if err != nil {
			return fmt.Errorf("conferir caixas de %s: %w", set.ImageID, err)
		}
		if n == 0 {
			return platform.ErrNotFound
		}
		return ErrConflict
	}
	return nil
}

// FindImageSets: as caixas de uma imagem em todas as tarefas.
func (m *MongoRepo) FindImageSets(ctx context.Context, sha1 string) ([]Set, error) {
	cur, err := m.sets().Find(ctx, bson.D{{Key: "image", Value: sha1}}, options.Find().SetSort(bson.D{{Key: "task", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("buscar caixas de %s: %w", sha1, err)
	}
	out := []Set{}
	if err := cur.All(ctx, &out); err != nil {
		return nil, fmt.Errorf("ler caixas de %s: %w", sha1, err)
	}
	return out, nil
}

// FindSets: as caixas de várias imagens numa tarefa (sha1 -> caixas).
func (m *MongoRepo) FindSets(ctx context.Context, task string, ids []string) (map[string]Set, error) {
	out := make(map[string]Set, len(ids))
	const batch = 1000
	for start := 0; start < len(ids); start += batch {
		end := min(start+batch, len(ids))
		cur, err := m.sets().Find(ctx, bson.D{
			{Key: "task", Value: task},
			{Key: "image", Value: bson.D{{Key: "$in", Value: ids[start:end]}}},
		})
		if err != nil {
			return nil, fmt.Errorf("buscar caixas: %w", err)
		}
		var sets []Set
		if err := cur.All(ctx, &sets); err != nil {
			return nil, fmt.Errorf("ler caixas: %w", err)
		}
		for _, set := range sets {
			out[set.ImageID] = set
		}
	}
	return out, nil
}

func (m *MongoRepo) DeleteImageSets(ctx context.Context, sha1 string) error {
	if _, err := m.sets().DeleteMany(ctx, bson.D{{Key: "image", Value: sha1}}); err != nil {
		return fmt.Errorf("apagar caixas de %s: %w", sha1, err)
	}
	return nil
}

// PackageCounts: por tarefa, as imagens do pacote anotadas nela.
func (m *MongoRepo) PackageCounts(ctx context.Context, pkg string) ([]PackageCount, error) {
	cur, err := m.sets().Aggregate(ctx, mongo.Pipeline{
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
	out := []PackageCount{}
	if err := cur.All(ctx, &out); err != nil {
		return nil, fmt.Errorf("ler contagem de caixas do pacote %s: %w", pkg, err)
	}
	return out, nil
}

func (m *MongoRepo) RenameTaskSets(ctx context.Context, oldName, newName string) error {
	if _, err := m.sets().UpdateMany(ctx, bson.D{{Key: "task", Value: oldName}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "task", Value: newName}}}}); err != nil {
		return fmt.Errorf("renomear a tarefa nas caixas: %w", err)
	}
	return nil
}

func (m *MongoRepo) DeleteTaskSets(ctx context.Context, task string) (int64, error) {
	res, err := m.sets().DeleteMany(ctx, bson.D{{Key: "task", Value: task}})
	if err != nil {
		return 0, fmt.Errorf("apagar caixas da tarefa %s: %w", task, err)
	}
	return res.DeletedCount, nil
}

func (m *MongoRepo) RenamePackageSets(ctx context.Context, oldName, newName string) error {
	if _, err := m.sets().UpdateMany(ctx, bson.D{{Key: "package", Value: oldName}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "package", Value: newName}}}}); err != nil {
		return fmt.Errorf("renomear o pacote nas caixas: %w", err)
	}
	return nil
}

func (m *MongoRepo) DeletePackageSets(ctx context.Context, pkg string) error {
	if _, err := m.sets().DeleteMany(ctx, bson.D{{Key: "package", Value: pkg}}); err != nil {
		return fmt.Errorf("apagar caixas do pacote %s: %w", pkg, err)
	}
	return nil
}

// LabelStats: por tarefa (imagens e caixas) e por tarefa + label (caixas e
// imagens distintas), num aggregate com $facet. Lido em structs: os
// documentos de dentro do resultado não são bson.M.
func (m *MongoRepo) LabelStats(ctx context.Context, tasks []string, ids []string) (map[string]*TaskStats, error) {
	match := bson.D{{Key: "$match", Value: bson.D{
		{Key: "task", Value: bson.D{{Key: "$in", Value: tasks}}},
		{Key: "image", Value: bson.D{{Key: "$in", Value: ids}}},
	}}}
	boxesOf := bson.D{{Key: "$ifNull", Value: bson.A{"$boxes", bson.A{}}}}
	perTask := bson.A{bson.D{{Key: "$group", Value: bson.D{
		{Key: "_id", Value: "$task"},
		{Key: "images", Value: bson.D{{Key: "$sum", Value: 1}}},
		{Key: "boxes", Value: bson.D{{Key: "$sum", Value: bson.D{{Key: "$size", Value: boxesOf}}}}},
	}}}}
	perLabel := bson.A{
		bson.D{{Key: "$unwind", Value: "$boxes"}},
		// uma linha por tarefa + label + imagem: depois, imagens = linhas
		bson.D{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: bson.D{{Key: "t", Value: "$task"}, {Key: "l", Value: "$boxes.label"}, {Key: "i", Value: "$image"}}},
			{Key: "n", Value: bson.D{{Key: "$sum", Value: 1}}},
		}}},
		bson.D{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: bson.D{{Key: "t", Value: "$_id.t"}, {Key: "l", Value: "$_id.l"}}},
			{Key: "images", Value: bson.D{{Key: "$sum", Value: 1}}},
			{Key: "boxes", Value: bson.D{{Key: "$sum", Value: "$n"}}},
		}}},
	}
	cur, err := m.sets().Aggregate(ctx, mongo.Pipeline{
		match,
		{{Key: "$facet", Value: bson.D{{Key: "per_task", Value: perTask}, {Key: "per_label", Value: perLabel}}}},
	})
	if err != nil {
		return nil, fmt.Errorf("contar as labels: %w", err)
	}
	var out []struct {
		PerTask []struct {
			Task   string `bson:"_id"`
			Images int64  `bson:"images"`
			Boxes  int64  `bson:"boxes"`
		} `bson:"per_task"`
		PerLabel []struct {
			ID struct {
				Task  string `bson:"t"`
				Label string `bson:"l"`
			} `bson:"_id"`
			Images int64 `bson:"images"`
			Boxes  int64 `bson:"boxes"`
		} `bson:"per_label"`
	}
	if err := cur.All(ctx, &out); err != nil {
		return nil, fmt.Errorf("ler a contagem das labels: %w", err)
	}
	res := map[string]*TaskStats{}
	if len(out) == 0 {
		return res, nil
	}
	for _, t := range out[0].PerTask {
		res[t.Task] = &TaskStats{Task: t.Task, Images: t.Images, Boxes: t.Boxes, Labels: map[string]LabelCount{}}
	}
	for _, l := range out[0].PerLabel {
		if ts := res[l.ID.Task]; ts != nil {
			ts.Labels[l.ID.Label] = LabelCount{Images: l.Images, Boxes: l.Boxes}
		}
	}
	return res, nil
}
