package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

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

// GET /packages: todos os pacotes com o progresso (total, verified, pending).
func (s *Server) handleListPackages(w http.ResponseWriter, r *http.Request) {
	pkgs, err := s.store.ListPackages(r.Context())
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pkgs)
}
