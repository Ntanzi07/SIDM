// Package mongotest é a conexão dos testes de INTEGRAÇÃO: rodam contra o
// Mongo de verdade (docker compose up -d mongo), com o usuário root do .env
// e um banco descartável (sidm_test_<timestamp>) apagado no fim do teste.
// O banco "sidm" de verdade nunca é tocado.
//
// Sem Mongo disponível, os testes são pulados (t.Skip): assim o `go test ./...`
// não quebra numa máquina sem Docker. Para forçar outra URI: MONGO_TEST_URI.
package mongotest

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// URI devolve a URI do Mongo de teste, ou pula o teste se não houver.
func URI(t *testing.T) string {
	t.Helper()
	// O teste roda na pasta do pacote (backend/internal/<modulo>): o .env da
	// raiz do projeto fica três pastas acima.
	_ = godotenv.Load("../../../.env")
	if uri := os.Getenv("MONGO_TEST_URI"); uri != "" {
		return uri
	}
	user := os.Getenv("MONGO_ROOT_USER")
	if user == "" {
		t.Skip("sem MONGO_TEST_URI nem MONGO_ROOT_USER no .env: pulando integração")
	}
	port := os.Getenv("MONGO_PORT")
	if port == "" {
		port = "27017"
	}
	u := url.URL{
		Scheme:   "mongodb",
		User:     url.UserPassword(user, os.Getenv("MONGO_ROOT_PASSWORD")),
		Host:     "localhost:" + port,
		Path:     "/",
		RawQuery: "authSource=admin",
	}
	return u.String()
}

// DBName: um nome de banco descartável.
func DBName() string { return fmt.Sprintf("sidm_test_%d", time.Now().UnixNano()) }

// DB conecta num banco descartável, com os índices únicos de nome do
// mongo/init/01-init.js, e apaga tudo no fim do teste.
func DB(t *testing.T) *mongo.Database {
	t.Helper()
	uri := URI(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Skipf("Mongo indisponível (%v): pulando integração", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		t.Skipf("Mongo indisponível (%v): pulando integração", err)
	}
	db := client.Database(DBName())
	t.Cleanup(func() {
		bg := context.Background()
		_ = db.Drop(bg)
		_ = client.Disconnect(bg)
	})
	if err := UniqueNames(ctx, db); err != nil {
		t.Fatal(err)
	}
	return db
}

// UniqueNames cria os mesmos índices únicos de nome do 01-init.js.
func UniqueNames(ctx context.Context, db *mongo.Database) error {
	for _, coll := range []string{"equipments", "cameras", "packages", "import_profiles"} {
		_, err := db.Collection(coll).Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys:    bson.D{{Key: "name", Value: 1}},
			Options: options.Index().SetUnique(true),
		})
		if err != nil {
			return fmt.Errorf("criar índice em %s: %w", coll, err)
		}
	}
	return nil
}
