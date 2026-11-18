package annotation

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"SIDM/internal/platform"
)

// MongoRepo é a coleção annotation_profiles (só os salvos).
type MongoRepo struct{ db *mongo.Database }

func NewMongoRepo(db *mongo.Database) *MongoRepo { return &MongoRepo{db: db} }

func (m *MongoRepo) coll() *mongo.Collection { return m.db.Collection("annotation_profiles") }

// EnsureIndexes: nome único. Idempotente (roda a cada subida).
func (m *MongoRepo) EnsureIndexes(ctx context.Context) error {
	_, err := m.coll().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "name", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("índice de annotation_profiles: %w", err)
	}
	return nil
}

func (m *MongoRepo) CreateProfile(ctx context.Context, p *Profile) error {
	_, err := m.coll().InsertOne(ctx, p)
	if mongo.IsDuplicateKeyError(err) {
		return platform.ErrDuplicate
	}
	if err != nil {
		return fmt.Errorf("criar perfil de anotação %s: %w", p.Name, err)
	}
	return nil
}

func (m *MongoRepo) FindProfile(ctx context.Context, name string) (*Profile, error) {
	var p Profile
	err := m.coll().FindOne(ctx, bson.D{{Key: "name", Value: name}}).Decode(&p)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, platform.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("buscar perfil de anotação %s: %w", name, err)
	}
	return &p, nil
}

func (m *MongoRepo) ReplaceProfile(ctx context.Context, p *Profile) error {
	res, err := m.coll().ReplaceOne(ctx, bson.D{{Key: "name", Value: p.Name}}, p)
	if err != nil {
		return fmt.Errorf("atualizar perfil de anotação %s: %w", p.Name, err)
	}
	if res.MatchedCount == 0 {
		return platform.ErrNotFound
	}
	return nil
}

func (m *MongoRepo) DeleteProfile(ctx context.Context, name string) error {
	res, err := m.coll().DeleteOne(ctx, bson.D{{Key: "name", Value: name}})
	if err != nil {
		return fmt.Errorf("apagar perfil de anotação %s: %w", name, err)
	}
	if res.DeletedCount == 0 {
		return platform.ErrNotFound
	}
	return nil
}

// ListProfiles: todos os salvos, por nome (são poucos).
func (m *MongoRepo) ListProfiles(ctx context.Context) ([]Profile, error) {
	cur, err := m.coll().Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("listar perfis de anotação: %w", err)
	}
	out := []Profile{}
	if err := cur.All(ctx, &out); err != nil {
		return nil, fmt.Errorf("ler perfis de anotação: %w", err)
	}
	return out, nil
}
