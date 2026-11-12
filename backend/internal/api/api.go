package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	"SIDM/internal/filename"
	"SIDM/internal/files"
	"SIDM/internal/store"
)

// Rotas planejadas:
//   GET  /health          ok se o Mongo responde
//   GET  /codes           tabelas para os dropdowns
//   GET  /images          busca com filtros + paginação      [feito]
//   GET  /images/{sha1}   detalhe de uma imagem              [feito]
//   GET  /images/{sha1}/file  o arquivo da imagem            [feito]
//   PUT  /images/{sha1}   salva anotação / conclui (reviewed) [feito]
//   POST /images          upload para staging (pending)      [feito]
//   POST /images/exists   checa hashes antes do upload (evita duplicata)
//   POST /export          gera zip em streaming das imagens selecionadas

type Server struct {
	store *store.Store
	files *files.Storage
}

func New(s *store.Store, fs *files.Storage) *Server {
	return &Server{store: s, files: fs}
}

// Routes registra todas as rotas e devolve o handler pronto para o http.ListenAndServe.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /codes", s.handleCodes)
	mux.HandleFunc("POST /equipments", s.handleCreateNamed(filename.SanitizeEquipment, s.store.CreateEquipment))
	mux.HandleFunc("POST /cameras", s.handleCreateNamed(filename.SanitizeCamera, s.store.CreateCamera))
	mux.HandleFunc("POST /images", s.handleUpload)
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

// handleCodes junta as tabelas fixas do padrão (Go) com os cadastros (Mongo).
func (s *Server) handleCodes(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{}
	for field, table := range filename.Codes() {
		resp[field] = table
	}

	equipments, err := s.store.ListEquipments(r.Context())
	if err != nil {
		s.serverError(w, err)
		return
	}
	cameras, err := s.store.ListCameras(r.Context())
	if err != nil {
		s.serverError(w, err)
		return
	}
	resp["equipments"] = equipments
	resp["cameras"] = cameras

	writeJSON(w, http.StatusOK, resp)
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
