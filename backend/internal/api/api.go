package api

import (
	"encoding/json"
	"log"
	"net/http"

	"SIDM/internal/filename"
	"SIDM/internal/store"
)

// Rotas planejadas:
//   GET  /health          ok se o Mongo responde
//   GET  /codes           tabelas para os dropdowns
//   GET  /images          busca com filtros (equipment, camera, day_time, weather...)
//   GET  /images/{sha1}   detalhe de uma imagem
//   POST /images          upload de imagens anotadas (valida, renomeia, salva)
//   POST /images/exists   checa hashes antes do upload (evita duplicata)
//   POST /export          gera zip em streaming das imagens selecionadas

type Server struct {
	store *store.Store
}

func New(s *store.Store) *Server {
	return &Server{store: s}
}

// Routes registra todas as rotas e devolve o handler pronto para o http.ListenAndServe.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /codes", s.handleCodes)
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

// serverError loga o erro real e devolve uma mensagem genérica:
// detalhes internos (como erro do Mongo) não devem vazar para o cliente.
func (s *Server) serverError(w http.ResponseWriter, err error) {
	log.Printf("erro interno: %v", err)
	http.Error(w, "erro interno", http.StatusInternalServerError)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("erro ao escrever JSON: %v", err)
	}
}
