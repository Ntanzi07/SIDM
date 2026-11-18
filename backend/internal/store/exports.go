package store

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"SIDM/internal/export"
)

// Perfis de export (os padrões, rf-detr e yolo, ficam no código, não aqui).

func (s *Store) exportProfiles() *mongo.Collection { return s.db.Collection("export_profiles") }

// ensureExportIndexes: nome único. Idempotente (roda a cada subida).
func (s *Store) ensureExportIndexes(ctx context.Context) error {
	_, err := s.exportProfiles().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "name", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("índice de export_profiles: %w", err)
	}
	return nil
}

func (s *Store) CreateExportProfile(ctx context.Context, p *export.Profile) error {
	_, err := s.exportProfiles().InsertOne(ctx, p)
	if mongo.IsDuplicateKeyError(err) {
		return ErrDuplicate
	}
	if err != nil {
		return fmt.Errorf("criar perfil de export %s: %w", p.Name, err)
	}
	return nil
}

func (s *Store) FindExportProfile(ctx context.Context, name string) (*export.Profile, error) {
	var p export.Profile
	err := s.exportProfiles().FindOne(ctx, bson.D{{Key: "name", Value: name}}).Decode(&p)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("buscar perfil de export %s: %w", name, err)
	}
	return &p, nil
}

func (s *Store) ReplaceExportProfile(ctx context.Context, p *export.Profile) error {
	res, err := s.exportProfiles().ReplaceOne(ctx, bson.D{{Key: "name", Value: p.Name}}, p)
	if err != nil {
		return fmt.Errorf("atualizar perfil de export %s: %w", p.Name, err)
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteExportProfile(ctx context.Context, name string) error {
	res, err := s.exportProfiles().DeleteOne(ctx, bson.D{{Key: "name", Value: name}})
	if err != nil {
		return fmt.Errorf("apagar perfil de export %s: %w", name, err)
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// ListExportProfiles: todos os salvos, por nome (são poucos).
func (s *Store) ListExportProfiles(ctx context.Context) ([]export.Profile, error) {
	cur, err := s.exportProfiles().Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
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
func (s *Store) TaskRenamed(ctx context.Context, oldName, newName string) error {
	if _, err := s.exportProfiles().UpdateMany(ctx, bson.D{{Key: "task", Value: oldName}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "task", Value: newName}}}}); err != nil {
		return fmt.Errorf("renomear a tarefa nos perfis de export: %w", err)
	}
	return nil
}
