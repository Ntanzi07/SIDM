package platform

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ConnectMongo abre a conexão e confirma, com um Ping, que o banco está de
// pé de verdade (o Connect sozinho não fala com o servidor). Cada módulo
// recebe o *mongo.Database e cuida das coleções dele.
func ConnectMongo(ctx context.Context, uri string) (*mongo.Client, error) {
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("mongo connect: %w", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("mongo ping: %w", err)
	}
	return client, nil
}
