package processing

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

// MongoRepo é a coleção processing_runs.
type MongoRepo struct{ db *mongo.Database }

func NewMongoRepo(db *mongo.Database) *MongoRepo { return &MongoRepo{db: db} }

func (m *MongoRepo) coll() *mongo.Collection { return m.db.Collection("processing_runs") }

// EnsureIndexes: a lista sai pela data (mais novos primeiro) e a subida
// procura os que estavam rodando. Idempotente.
func (m *MongoRepo) EnsureIndexes(ctx context.Context) error {
	if _, err := m.coll().Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "started_at", Value: -1}}},
		{Keys: bson.D{{Key: "status", Value: 1}}},
	}); err != nil {
		return fmt.Errorf("índices de processing_runs: %w", err)
	}
	return nil
}

func (m *MongoRepo) CreateRun(ctx context.Context, r *Run) error {
	_, err := m.coll().InsertOne(ctx, r)
	if mongo.IsDuplicateKeyError(err) {
		return platform.ErrDuplicate
	}
	if err != nil {
		return fmt.Errorf("criar processamento %s: %w", r.ID, err)
	}
	return nil
}

func (m *MongoRepo) SaveRun(ctx context.Context, r *Run) error {
	res, err := m.coll().ReplaceOne(ctx, bson.D{{Key: "_id", Value: r.ID}}, r)
	if err != nil {
		return fmt.Errorf("gravar processamento %s: %w", r.ID, err)
	}
	if res.MatchedCount == 0 {
		return platform.ErrNotFound
	}
	return nil
}

func (m *MongoRepo) FindRun(ctx context.Context, id string) (*Run, error) {
	var r Run
	err := m.coll().FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&r)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, platform.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("buscar processamento %s: %w", id, err)
	}
	return &r, nil
}

func (m *MongoRepo) ListRuns(ctx context.Context, limit int64) ([]Run, error) {
	cur, err := m.coll().Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "started_at", Value: -1}}).SetLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("listar processamentos: %w", err)
	}
	out := []Run{}
	if err := cur.All(ctx, &out); err != nil {
		return nil, fmt.Errorf("ler processamentos: %w", err)
	}
	return out, nil
}

func (m *MongoRepo) InterruptRunning(ctx context.Context, now time.Time) (int64, error) {
	res, err := m.coll().UpdateMany(ctx, bson.D{{Key: "status", Value: StatusRunning}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "status", Value: StatusInterrupted}, {Key: "finished_at", Value: now}}}})
	if err != nil {
		return 0, fmt.Errorf("marcar processamentos interrompidos: %w", err)
	}
	return res.ModifiedCount, nil
}
