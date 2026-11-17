package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ErrDuplicate indica que o nome já existe no cadastro (índice único).
var ErrDuplicate = errors.New("já cadastrado")

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
	s := &Store{
		client: client,
		db:     client.Database(dbName),
	}
	for _, ensure := range []func(context.Context) error{s.ensureTaskIndexes, s.ensureBoxIndexes, s.ensureExportIndexes} {
		if err := ensure(ctx); err != nil {
			_ = client.Disconnect(context.Background())
			return nil, err
		}
	}
	return s, nil
}

func (s *Store) Ping(ctx context.Context) error {
	return s.client.Ping(ctx, nil)
}

func (s *Store) Close(ctx context.Context) error {
	return s.client.Disconnect(ctx)
}

// SearchEquipments busca equipamentos ativos cujo nome CONTÉM q (sem
// diferenciar maiúscula), em ordem alfabética, paginado. q vazio = todos.
// Feito para autocomplete: "mf47" encontra "8003-MF4739".
func (s *Store) SearchEquipments(ctx context.Context, q string, limit, skip int64) ([]string, int64, error) {
	return s.searchActiveNames(ctx, "equipments", q, limit, skip)
}

// SearchCameras faz o mesmo para câmeras.
func (s *Store) SearchCameras(ctx context.Context, q string, limit, skip int64) ([]string, int64, error) {
	return s.searchActiveNames(ctx, "cameras", q, limit, skip)
}

// nameContains monta o filtro "name contém q, ignorando maiúscula".
//
// regexp.QuoteMeta é OBRIGATÓRIO: o texto vem do usuário, e sem escapar,
// alguém digitando ".*" ou "(a+)+$" estaria mandando uma regex para o
// Mongo (resultado errado, ou uma regex lenta de propósito travando o banco).
func nameContains(q string) bson.E {
	return bson.E{Key: "name", Value: bson.Regex{Pattern: regexp.QuoteMeta(q), Options: "i"}}
}

func (s *Store) searchActiveNames(ctx context.Context, collection, q string, limit, skip int64) ([]string, int64, error) {
	coll := s.db.Collection(collection)
	filter := bson.D{{Key: "active", Value: true}}
	if q != "" {
		filter = append(filter, nameContains(q))
	}

	total, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("contar %s: %w", collection, err)
	}

	opts := options.Find().
		SetProjection(bson.D{{Key: "name", Value: 1}}). // só traz o campo name
		SetSort(bson.D{{Key: "name", Value: 1}}).
		SetLimit(limit).
		SetSkip(skip)

	cur, err := coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, fmt.Errorf("listar %s: %w", collection, err)
	}

	var docs []struct {
		Name string `bson:"name"`
	}
	if err := cur.All(ctx, &docs); err != nil { // All já fecha o cursor
		return nil, 0, fmt.Errorf("ler %s: %w", collection, err)
	}

	// make com len 0 (e não nil) para o JSON sair [] em vez de null
	names := make([]string, 0, len(docs))
	for _, d := range docs {
		names = append(names, d.Name)
	}
	return names, total, nil
}

// CreateEquipment cadastra um equipamento (nome já sanitizado).
func (s *Store) CreateEquipment(ctx context.Context, name string) error {
	return s.createNamed(ctx, "equipments", name)
}

// CreateCamera cadastra uma câmera (nome já sanitizado).
func (s *Store) CreateCamera(ctx context.Context, name string) error {
	return s.createNamed(ctx, "cameras", name)
}

func (s *Store) createNamed(ctx context.Context, collection, name string) error {
	_, err := s.db.Collection(collection).InsertOne(ctx, bson.D{
		{Key: "name", Value: name},
		{Key: "active", Value: true},
		{Key: "created_at", Value: time.Now().UTC()},
	})
	if mongo.IsDuplicateKeyError(err) {
		return ErrDuplicate // quem garante isso é o índice único do 01-init.js
	}
	if err != nil {
		return fmt.Errorf("inserir em %s: %w", collection, err)
	}
	return nil
}
