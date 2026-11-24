package main

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/joho/godotenv"

	"SIDM/internal/annotation"
	"SIDM/internal/api"
	"SIDM/internal/boxes"
	"SIDM/internal/catalog"
	"SIDM/internal/exports"
	"SIDM/internal/files"
	"SIDM/internal/images"
	"SIDM/internal/imports"
	"SIDM/internal/platform"
	"SIDM/internal/versions"
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

	client, err := platform.ConnectMongo(ctx, mongoURI)
	if err != nil {
		log.Fatalf("falha ao conectar no Mongo: %v", err)
	}
	defer client.Disconnect(context.Background())
	log.Printf("conectado ao Mongo (%s)", mongoDB)

	// Local (go run em backend/): usa a pasta data/ da raiz do projeto.
	// No Docker, o compose define IMAGES_DIR=/data/images.
	imagesDir := getenv("IMAGES_DIR", "../data/images")
	fs, err := files.NewStorage(imagesDir)
	if err != nil {
		log.Fatalf("falha ao preparar %s: %v", imagesDir, err)
	}
	log.Printf("imagens em %s", imagesDir)

	// Os módulos. Cada um tem o seu repositório (as coleções dele) e conversa
	// com os outros pelos Services. Os Listen são as cascatas: quem guarda o
	// nome de um pacote, imagem ou tarefa é avisado quando ele muda.
	db := client.Database(mongoDB)

	// Os perfis de anotação vêm primeiro: o catálogo (cada pacote tem um) e
	// as imagens (cada anotação passa por ele) dependem deles.
	annRepo := annotation.NewMongoRepo(db)
	if err := annRepo.EnsureIndexes(ctx); err != nil {
		log.Fatalf("falha ao preparar os perfis de anotação no Mongo: %v", err)
	}
	ann := annotation.New(annRepo)

	// As imagens de antes dos perfis vão para fields/objects (uma vez só:
	// nas próximas subidas não há mais nada para migrar). Sem o prazo de 10s
	// da conexão: num banco grande, a migração pode demorar mais.
	imgRepo := images.NewMongoRepo(db)
	if n, err := imgRepo.Migrate(context.Background()); err != nil {
		log.Fatalf("falha ao migrar as imagens: %v", err)
	} else if n > 0 {
		log.Printf("%d imagens migradas para os campos do perfil de anotação", n)
	}
	if err := imgRepo.EnsureIndexes(ctx); err != nil {
		log.Fatalf("falha ao preparar as imagens no Mongo: %v", err)
	}

	impRepo := imports.NewMongoRepo(db)
	img := images.New(imgRepo, fs, nil, imports.Profiles{Repo: impRepo}, ann) // o perfil jpeg-ini do upload (padrão ou do banco)
	cat := catalog.New(catalog.NewMongoRepo(db), fs, img, ann)
	img.SetCatalog(cat)
	// No disco toda imagem é <sha1>.ext: as verificadas de antes, com o nome
	// v2 no arquivo, voltam para o sha1 (uma vez só).
	if n, failed, err := img.MigrateFiles(context.Background()); err != nil {
		log.Fatalf("falha ao migrar os arquivos das imagens: %v", err)
	} else {
		if n > 0 {
			log.Printf("%d arquivos de imagens verificadas voltaram para <sha1>.ext", n)
		}
		for _, f := range failed {
			log.Printf("AVISO: o arquivo da imagem %s não foi movido; tenta de novo na próxima subida", f)
		}
	}
	ann.SetPackages(cat) // um perfil em uso não pode ser apagado

	boxRepo := boxes.NewMongoRepo(db)
	if err := boxRepo.EnsureIndexes(ctx); err != nil {
		log.Fatalf("falha ao preparar as caixas no Mongo: %v", err)
	}
	bx := boxes.New(boxRepo, img, fs, cat, img)
	cat.Listen(bx) // as caixas guardam o pacote de cada imagem
	img.Listen(bx) // e são apagadas junto com a imagem

	imp := imports.New(impRepo, img, cat, ann)

	expRepo := exports.NewMongoRepo(db)
	if err := expRepo.EnsureIndexes(ctx); err != nil {
		log.Fatalf("falha ao preparar os perfis de export no Mongo: %v", err)
	}
	exp := exports.New(expRepo, img, bx, cat, fs, ann)
	bx.Listen(exp) // os perfis de export guardam a tarefa

	verRepo := versions.NewMongoRepo(db)
	if err := verRepo.EnsureIndexes(ctx); err != nil {
		log.Fatalf("falha ao preparar as versões no Mongo: %v", err)
	}
	ver := versions.New(verRepo, img, bx, ann)
	exp.UseVersions(ver) // baixar uma versão de um dataset

	ping := func(ctx context.Context) error { return client.Ping(ctx, nil) }
	srv := api.New(ping, cat, img, bx, imp, exp, ann, ver)

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
