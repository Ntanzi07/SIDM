package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"SIDM/internal/api"
	"SIDM/internal/store"
)

// Ponto de entrada do servidor SIDM.
func main() {
	addr := getenv("HTTP_ADDR", ":8080")
	mongoURI := getenv("MONGO_URI", "mongodb://localhost:27017")
	mongoDB := getenv("MONGO_DB", "sidm")

	// Conexão com timeout: se o Mongo não responder em 10s, o servidor nem sobe.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	s, err := store.New(ctx, mongoURI, mongoDB)
	if err != nil {
		log.Fatalf("falha ao conectar no Mongo: %v", err)
	}
	defer s.Close(context.Background())
	log.Printf("conectado ao Mongo (%s)", mongoDB)

	srv := api.New(s)

	log.Printf("SIDM ouvindo em %s", addr)
	log.Fatal(http.ListenAndServe(addr, srv.Routes()))
}

// getenv retorna a variável de ambiente ou o valor padrão se estiver vazia.
func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
