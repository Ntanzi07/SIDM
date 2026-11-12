package store

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Acesso ao MongoDB: collections images, image_history, cameras e equipments.
type Store struct {
	client *mongo.Client
	db     *mongo.Database
}

func New(ctx context.Context, uri, dbName string) (*Store, error) {
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("mongo connect: %w", err)
	}

	// Ping: confirma que o banco está de pé de verdade
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("mongo ping: %w", err)
	}

	// Deu certo: monta o Store
	return &Store{
		client: client,
		db:     client.Database(dbName),
	}, nil
}

func (s *Store) Ping(ctx context.Context) error {
	return s.client.Ping(ctx, nil)
}

func (s *Store) Close(ctx context.Context) error {
	return s.client.Disconnect(ctx)
}

// ListEquipments retorna os nomes dos equipamentos ativos, em ordem alfabética.
func (s *Store) ListEquipments(ctx context.Context) ([]string, error) {
	return s.listActiveNames(ctx, "equipments")
}

// ListCameras retorna os nomes das câmeras ativas, em ordem alfabética.
func (s *Store) ListCameras(ctx context.Context) ([]string, error) {
	return s.listActiveNames(ctx, "cameras")
}

func (s *Store) listActiveNames(ctx context.Context, collection string) ([]string, error) {
	filter := bson.D{{Key: "active", Value: true}}
	opts := options.Find().
		SetProjection(bson.D{{Key: "name", Value: 1}}). // só traz o campo name
		SetSort(bson.D{{Key: "name", Value: 1}})

	cur, err := s.db.Collection(collection).Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("listar %s: %w", collection, err)
	}

	var docs []struct {
		Name string `bson:"name"`
	}
	if err := cur.All(ctx, &docs); err != nil { // All já fecha o cursor
		return nil, fmt.Errorf("ler %s: %w", collection, err)
	}

	// make com len 0 (e não nil) para o JSON sair [] em vez de null
	names := make([]string, 0, len(docs))
	for _, d := range docs {
		names = append(names, d.Name)
	}
	return names, nil
}
