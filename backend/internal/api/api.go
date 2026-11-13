package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"SIDM/internal/filename"
	"SIDM/internal/files"
	"SIDM/internal/store"
)

// Rotas:
//   GET  /health               ok se o Mongo responde
//   GET  /codes                tabelas fixas do padrão (para os dropdowns)
//   GET  /equipments?q=&limit= busca equipamentos (autocomplete), paginado
//   GET  /cameras?q=&limit=    busca câmeras (autocomplete), paginado
//   POST /equipments           cadastra equipamento {"name":...}
//   POST /cameras              cadastra câmera {"name":...}
//   POST /packages             cria pacote {"name":...}
//   GET  /packages?q=&limit=   pacotes com progresso, paginado
//   POST /packages/{name}/next pega a próxima imagem da fila (X-User obrigatório)
//   POST /images/{sha1}/release devolve a imagem para a fila
//   POST /images/exists        checa hashes antes do upload
//   POST /images?package=X     upload para o pacote X (pending)
//   GET  /images               busca com filtros, paginado
//   GET  /images/{sha1}        detalhe de uma imagem
//   GET  /images/{sha1}/file   o arquivo da imagem
//   PUT  /images/{sha1}        salva rascunho / verifica (renomeia para v2)
//   POST /export               TODO: zip das imagens selecionadas
//
// Toda listagem responde no mesmo formato: {"total": N, "items": [...]},
// com ?limit= e ?skip= para paginar.

// Store é tudo que a API precisa do banco.
// A API depende desta interface, e não do *store.Store concreto: assim os
// testes usam um banco em memória (ver api_test.go) e não precisam de Mongo.
// O *store.Store satisfaz a interface sem declarar nada (interfaces em Go
// são implícitas): basta ter os métodos.
type Store interface {
	Ping(ctx context.Context) error

	SearchEquipments(ctx context.Context, q string, limit, skip int64) ([]string, int64, error)
	SearchCameras(ctx context.Context, q string, limit, skip int64) ([]string, int64, error)
	CreateEquipment(ctx context.Context, name string) error
	CreateCamera(ctx context.Context, name string) error
	EquipmentExists(ctx context.Context, name string) (bool, error)
	CameraExists(ctx context.Context, name string) (bool, error)

	CreatePackage(ctx context.Context, name, createdBy string) error
	FindPackage(ctx context.Context, name string) (*store.Package, error)
	ListPackages(ctx context.Context, q string, limit, skip int64) ([]store.PackageSummary, int64, error)

	InsertImage(ctx context.Context, img *store.Image) error
	DeleteImage(ctx context.Context, sha1 string) error
	FindImage(ctx context.Context, sha1 string) (*store.Image, error)
	ReplaceImage(ctx context.Context, img *store.Image, prevUpdatedAt time.Time) error
	ClaimNext(ctx context.Context, pkg, user, skip string, lease time.Duration, now time.Time) (*store.Image, error)
	ReleaseClaim(ctx context.Context, sha1, user string) error
	ListImages(ctx context.Context, f store.ImageFilter) ([]store.Image, int64, error)
	ExistingImages(ctx context.Context, ids []string) (map[string]store.ExistingImage, error)
	InsertHistory(ctx context.Context, entries []store.HistoryEntry) error
}

type Server struct {
	store Store
	files *files.Storage
}

func New(s Store, fs *files.Storage) *Server {
	return &Server{store: s, files: fs}
}

// Routes registra todas as rotas e devolve o handler pronto para o http.ListenAndServe.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /codes", s.handleCodes)
	mux.HandleFunc("GET /equipments", s.handleSearchNames(s.store.SearchEquipments))
	mux.HandleFunc("GET /cameras", s.handleSearchNames(s.store.SearchCameras))
	mux.HandleFunc("POST /equipments", s.handleCreateNamed(filename.SanitizeEquipment, s.store.CreateEquipment))
	mux.HandleFunc("POST /cameras", s.handleCreateNamed(filename.SanitizeCamera, s.store.CreateCamera))
	mux.HandleFunc("POST /packages", s.handleCreatePackage)
	mux.HandleFunc("GET /packages", s.handleListPackages)
	mux.HandleFunc("POST /packages/{name}/next", s.handleNext)
	mux.HandleFunc("POST /images/{sha1}/release", s.handleRelease)
	mux.HandleFunc("POST /images", s.handleUpload)
	mux.HandleFunc("POST /images/exists", s.handleExists)
	mux.HandleFunc("GET /images", s.handleListImages)
	mux.HandleFunc("GET /images/{sha1}", s.handleGetImage)
	mux.HandleFunc("GET /images/{sha1}/file", s.handleImageFile)
	mux.HandleFunc("PUT /images/{sha1}", s.handleAnnotate)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	// r.Context() é cancelado se o cliente desistir da requisição.
	if err := s.store.Ping(r.Context()); err != nil {
		http.Error(w, "mongo indisponível", http.StatusServiceUnavailable)
		return
	}
	w.Write([]byte("ok"))
}

// handleCodes devolve as tabelas fixas do padrão v2 (direção, clima, cor...).
// Equipamentos e câmeras NÃO vêm aqui: podem ser milhares, então têm busca
// própria e paginada (GET /equipments e GET /cameras).
func (s *Server) handleCodes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, filename.Codes())
}

// page é o formato padrão de toda listagem.
type page[T any] struct {
	Total int64 `json:"total"`
	Items []T   `json:"items"`
}

type nameItem struct {
	Name string `json:"name"`
}

// handleSearchNames: GET /equipments?q=mf47&limit=10&skip=0
// Pensado para autocomplete: o front chama a cada letra digitada (com
// debounce) e mostra as primeiras opções. Sem q, lista todos em ordem.
func (s *Server) handleSearchNames(
	search func(ctx context.Context, q string, limit, skip int64) ([]string, int64, error),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		query := strings.TrimSpace(q.Get("q"))
		if len(query) > 64 {
			writeError(w, http.StatusBadRequest, "busca longa demais (máx. 64 caracteres)")
			return
		}

		names, total, err := search(r.Context(), query,
			intParam(q, "limit", 10, 1, 50),
			intParam(q, "skip", 0, 0, 1<<31))
		if err != nil {
			s.serverError(w, err)
			return
		}

		items := make([]nameItem, len(names))
		for i, n := range names {
			items[i] = nameItem{Name: n}
		}
		writeJSON(w, http.StatusOK, page[nameItem]{Total: total, Items: items})
	}
}

// handleCreateNamed serve para equipamentos e câmeras: o fluxo é o mesmo,
// só mudam a sanitização e a collection. Por isso recebe as duas funções.
func (s *Server) handleCreateNamed(
	sanitize func(string) (string, error),
	create func(context.Context, string) error,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<10) // corpo de no máximo 1 KB

		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, `JSON inválido: esperado {"name": "..."}`)
			return
		}

		name, err := sanitize(req.Name)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		if err := create(r.Context(), name); err != nil {
			if errors.Is(err, store.ErrDuplicate) {
				writeError(w, http.StatusConflict, fmt.Sprintf("%q já está cadastrado", name))
				return
			}
			s.serverError(w, err)
			return
		}

		writeJSON(w, http.StatusCreated, map[string]string{"name": name})
	}
}

// serverError loga o erro real e devolve uma mensagem genérica:
// detalhes internos (como erro do Mongo) não devem vazar para o cliente.
func (s *Server) serverError(w http.ResponseWriter, err error) {
	log.Printf("erro interno: %v", err)
	http.Error(w, "erro interno", http.StatusInternalServerError)
}

// userFrom diz quem está fazendo a ação.
// TODO: quando tiver login, o usuário vem da sessão, não de um header.
func userFrom(r *http.Request) string {
	if u := r.Header.Get("X-User"); u != "" {
		return u
	}
	return "anonymous"
}

func (s *Server) logError(err error) {
	log.Printf("erro: %v", err)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("erro ao escrever JSON: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
