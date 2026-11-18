package main

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/joho/godotenv"

	"SIDM/internal/api"
	"SIDM/internal/boxes"
	"SIDM/internal/catalog"
	"SIDM/internal/files"
	"SIDM/internal/images"
	"SIDM/internal/store"
)

// Ponto de entrada do servidor SIDM.
func main() {
	loadDotEnv()

	addr := getenv("HTTP_ADDR", ":8080")
	mongoDB := getenv("MONGO_DB", "sidm")
	mongoURI := mongoURIFromEnv(mongoDB)

	// Conexão com timeout: se o Mongo não responder em 10s, o servidor nem sobe.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	s, err := store.New(ctx, mongoURI, mongoDB)
	if err != nil {
		log.Fatalf("falha ao conectar no Mongo: %v", err)
	}
	defer s.Close(context.Background())
	log.Printf("conectado ao Mongo (%s)", mongoDB)

	// Local (go run em backend/): usa a pasta data/ da raiz do projeto.
	// No Docker, o compose define IMAGES_DIR=/data/images.
	imagesDir := getenv("IMAGES_DIR", "../data/images")
	fs, err := files.NewStorage(imagesDir)
	if err != nil {
		log.Fatalf("falha ao preparar %s: %v", imagesDir, err)
	}
	log.Printf("imagens em %s", imagesDir)

	// Os módulos. Cada um tem o seu repositório; o store ainda guarda os
	// perfis de import e de export (que viram módulos nos próximos passos).
	db := s.DB()
	img := images.New(images.NewMongoRepo(db), fs, nil, s) // s: o perfil jpeg-ini do upload
	cat := catalog.New(catalog.NewMongoRepo(db), fs, img)
	img.SetCatalog(cat)
	boxRepo := boxes.NewMongoRepo(db)
	if err := boxRepo.EnsureIndexes(ctx); err != nil {
		log.Fatalf("falha ao preparar as caixas no Mongo: %v", err)
	}
	bx := boxes.New(boxRepo, img, fs, cat, s) // s: os perfis de export guardam a tarefa
	cat.Listen(bx)                            // as caixas guardam o pacote de cada imagem
	img.Listen(bx)                            // e são apagadas junto com a imagem
	srv := api.New(s, fs, cat, bx, img)

	log.Printf("SIDM ouvindo em %s", addr)
	log.Fatal(http.ListenAndServe(addr, srv.Routes()))
}

// loadDotEnv carrega o .env para rodar local com `go run`.
// Procura em backend/.env e na raiz do projeto (../.env).
// Não sobrescreve variáveis que já existem: no Docker, o compose ganha;
// no terminal, um $env:VAR setado também ganha do arquivo.
func loadDotEnv() {
	for _, path := range []string{".env", "../.env"} {
		if err := godotenv.Load(path); err == nil {
			log.Printf("variáveis carregadas de %s", path)
			return
		}
	}
}

// mongoURIFromEnv usa MONGO_URI se existir (Docker). Senão, monta a URI
// local a partir do usuário da aplicação do .env.
func mongoURIFromEnv(dbName string) string {
	if uri := os.Getenv("MONGO_URI"); uri != "" {
		return uri
	}

	port := getenv("MONGO_PORT", "27017")
	user := os.Getenv("MONGO_APP_USER")
	if user == "" {
		return "mongodb://localhost:" + port
	}

	// url.UserPassword escapa caracteres especiais da senha automaticamente.
	u := url.URL{
		Scheme:   "mongodb",
		User:     url.UserPassword(user, os.Getenv("MONGO_APP_PASSWORD")),
		Host:     "localhost:" + port,
		Path:     "/" + dbName,
		RawQuery: "authSource=" + dbName,
	}
	return u.String()
}

// getenv retorna a variável de ambiente ou o valor padrão se estiver vazia.
func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
