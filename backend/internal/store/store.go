package store

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"SIDM/internal/platform"
)

// Os erros comuns ficam no platform (os módulos e os handlers comparam com
// errors.Is); estes nomes ficam por compatibilidade.
var (
	ErrNotFound  = platform.ErrNotFound
	ErrDuplicate = platform.ErrDuplicate
)

// Acesso ao MongoDB das partes que ainda não viraram módulo (imagens,
// histórico, fila, perfis de import e de export).
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
	for _, ensure := range []func(context.Context) error{s.ensureExportIndexes} {
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

// DB é o banco, para os módulos que já têm o seu próprio repositório (ex.:
// o catálogo). Some quando o store deixar de existir.
func (s *Store) DB() *mongo.Database { return s.db }

// nameContains: ver platform.NameContains.
func nameContains(q string) bson.E { return platform.NameContains(q) }
