package annotation

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"SIDM/internal/platform"
)

const maxProfileBytes = 256 << 10

// Routes são as rotas do módulo.
func (s *Service) Routes() []platform.Route {
	return []platform.Route{
		{Method: "GET", Path: "/annotation-profiles", Handler: s.handleList},
		{Method: "POST", Path: "/annotation-profiles", Handler: s.handleCreate},
		{Method: "GET", Path: "/annotation-profiles/{name}", Handler: s.handleGet},
		{Method: "PUT", Path: "/annotation-profiles/{name}", Handler: s.handleReplace},
		{Method: "DELETE", Path: "/annotation-profiles/{name}", Handler: s.handleDelete},
		{Method: "POST", Path: "/annotation-profiles/{name}/check", Handler: s.handleCheck},
	}
}

// writeErr: InvalidError vira 400 (com a lista de problemas), ErrNotFound
// 404, ErrDuplicate 409, o resto 500.
func writeErr(w http.ResponseWriter, err error, name string) {
	var invalid *InvalidError
	switch {
	case errors.As(err, &invalid):
		platform.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": invalid.Msg, "problems": nonNil(invalid.Problems)})
	case errors.Is(err, platform.ErrNotFound):
		platform.WriteError(w, http.StatusNotFound, fmt.Sprintf("perfil de anotação %q não existe", name))
	case errors.Is(err, platform.ErrDuplicate):
		platform.WriteError(w, http.StatusConflict, fmt.Sprintf("já existe um perfil de anotação %q", name))
	default:
		platform.ServerError(w, err)
	}
}

func nonNil(ps []Problem) []Problem {
	if ps == nil {
		return []Problem{}
	}
	return ps
}

// decode lê o corpo JSON (campos desconhecidos são erro: "optons" em vez de
// "options" não pode passar calado).
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxProfileBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		platform.WriteError(w, http.StatusBadRequest, "JSON inválido: "+err.Error())
		return false
	}
	return true
}

// GET /annotation-profiles  (os padrões + os salvos)
func (s *Service) handleList(w http.ResponseWriter, r *http.Request) {
	items, err := s.ListProfiles(r.Context())
	if err != nil {
		platform.ServerError(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, platform.Page[Profile]{Total: int64(len(items)), Items: items})
}

// GET /annotation-profiles/{name}
func (s *Service) handleGet(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	p, err := s.FindProfile(r.Context(), name)
	if err != nil {
		writeErr(w, err, name)
		return
	}
	platform.WriteJSON(w, http.StatusOK, p)
}

// POST /annotation-profiles
func (s *Service) handleCreate(w http.ResponseWriter, r *http.Request) {
	var p Profile
	if !decode(w, r, &p) {
		return
	}
	if err := s.CreateProfile(r.Context(), &p, platform.UserFrom(r)); err != nil {
		writeErr(w, err, p.Name)
		return
	}
	platform.WriteJSON(w, http.StatusCreated, p)
}

// PUT /annotation-profiles/{name}  (os padrões não)
func (s *Service) handleReplace(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := editable(name); err != nil {
		writeErr(w, err, name)
		return
	}
	var p Profile
	if !decode(w, r, &p) {
		return
	}
	if p.Name != name {
		platform.WriteError(w, http.StatusBadRequest, fmt.Sprintf("o nome no corpo (%q) é diferente do nome na URL (%q)", p.Name, name))
		return
	}
	if err := s.ReplaceProfile(r.Context(), &p); err != nil {
		writeErr(w, err, name)
		return
	}
	platform.WriteJSON(w, http.StatusOK, p)
}

// DELETE /annotation-profiles/{name}  (os padrões não)
func (s *Service) handleDelete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.DeleteProfile(r.Context(), name); err != nil {
		writeErr(w, err, name)
		return
	}
	platform.WriteJSON(w, http.StatusOK, map[string]any{"name": name, "deleted": true})
}

// POST /annotation-profiles/{name}/check  {"fields": {...}, "objects": [...], "complete": true}
// Confere uma anotação contra o perfil, sem gravar nada: devolve os valores
// limpos ou a lista de problemas. Serve para testar um perfil novo e para a
// tela avisar antes de salvar.
func (s *Service) handleCheck(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	p, err := s.FindProfile(r.Context(), name)
	if err != nil {
		writeErr(w, err, name)
		return
	}
	var body struct {
		Values
		Complete bool `json:"complete"`
	}
	if !decode(w, r, &body) {
		return
	}
	clean, err := p.Check(body.Values, body.Complete)
	if err != nil {
		writeErr(w, err, name)
		return
	}
	platform.WriteJSON(w, http.StatusOK, clean)
}
