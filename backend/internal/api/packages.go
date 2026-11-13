package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"SIDM/internal/filename"
	"SIDM/internal/store"
)

// POST /packages {"name": "Noturnas Setembro"}
// O nome é sanitizado ("Noturnas-Setembro") porque vira nome de pasta.
func (s *Server) handleCreatePackage(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<10)

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, `JSON inválido: esperado {"name": "..."}`)
		return
	}

	name, err := filename.SanitizePackage(req.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.store.CreatePackage(r.Context(), name, userFrom(r)); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			writeError(w, http.StatusConflict, fmt.Sprintf("pacote %q já existe", name))
			return
		}
		s.serverError(w, err)
		return
	}

	// A pasta é criada já na criação, para um pacote vazio também aparecer no disco.
	if err := s.files.EnsurePackageDir(name); err != nil {
		s.serverError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"name": name})
}

// GET /packages?q=noturnas&limit=20&skip=0
// Pacotes (mais novos primeiro) com o progresso de cada um.
func (s *Server) handleListPackages(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := strings.TrimSpace(q.Get("q"))
	if len(query) > 64 {
		writeError(w, http.StatusBadRequest, "busca longa demais (máx. 64 caracteres)")
		return
	}

	pkgs, total, err := s.store.ListPackages(r.Context(), query,
		intParam(q, "limit", 20, 1, 100),
		intParam(q, "skip", 0, 0, 1<<31))
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page[store.PackageSummary]{Total: total, Items: pkgs})
}
