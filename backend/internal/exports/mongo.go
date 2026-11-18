package exports

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"SIDM/internal/export"
	"SIDM/internal/platform"
)

// MongoRepo é a coleção export_profiles (só os perfis salvos: os padrões
// ficam no código).
type MongoRepo struct{ db *mongo.Database }

func NewMongoRepo(db *mongo.Database) *MongoRepo { return &MongoRepo{db: db} }

func (m *MongoRepo) coll() *mongo.Collection { return m.db.Collection("export_profiles") }

// EnsureIndexes: nome único. Idempotente (roda a cada subida).
func (m *MongoRepo) EnsureIndexes(ctx context.Context) error {
	_, err := m.coll().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "name", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("índice de export_profiles: %w", err)
	}
	return nil
}

func (m *MongoRepo) CreateExportProfile(ctx context.Context, p *export.Profile) error {
	_, err := m.coll().InsertOne(ctx, p)
	if mongo.IsDuplicateKeyError(err) {
		return platform.ErrDuplicate
	}
	if err != nil {
		return fmt.Errorf("criar perfil de export %s: %w", p.Name, err)
	}
	return nil
}

func (m *MongoRepo) FindExportProfile(ctx context.Context, name string) (*export.Profile, error) {
	var p export.Profile
	err := m.coll().FindOne(ctx, bson.D{{Key: "name", Value: name}}).Decode(&p)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, platform.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("buscar perfil de export %s: %w", name, err)
	}
	return &p, nil
}

func (m *MongoRepo) ReplaceExportProfile(ctx context.Context, p *export.Profile) error {
	res, err := m.coll().ReplaceOne(ctx, bson.D{{Key: "name", Value: p.Name}}, p)
	if err != nil {
		return fmt.Errorf("atualizar perfil de export %s: %w", p.Name, err)
	}
	if res.MatchedCount == 0 {
		return platform.ErrNotFound
	}
	return nil
}

func (m *MongoRepo) DeleteExportProfile(ctx context.Context, name string) error {
	res, err := m.coll().DeleteOne(ctx, bson.D{{Key: "name", Value: name}})
	if err != nil {
		return fmt.Errorf("apagar perfil de export %s: %w", name, err)
	}
	if res.DeletedCount == 0 {
		return platform.ErrNotFound
	}
	return nil
}

// ListExportProfiles: todos os salvos, por nome (são poucos).
func (m *MongoRepo) ListExportProfiles(ctx context.Context) ([]export.Profile, error) {
	cur, err := m.coll().Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("listar perfis de export: %w", err)
	}
	out := []export.Profile{}
	if err := cur.All(ctx, &out); err != nil {
		return nil, fmt.Errorf("ler perfis de export: %w", err)
	}
	return out, nil
}

// TaskRenamed: os perfis de export guardam a tarefa (boxes.TaskListener).
func (m *MongoRepo) TaskRenamed(ctx context.Context, oldName, newName string) error {
	if _, err := m.coll().UpdateMany(ctx, bson.D{{Key: "task", Value: oldName}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "task", Value: newName}}}}); err != nil {
		return fmt.Errorf("renomear a tarefa nos perfis de export: %w", err)
	}
	return nil
}
