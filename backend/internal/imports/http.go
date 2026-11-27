package imports

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"SIDM/internal/importer"
	"SIDM/internal/platform"
)

// Limites de tamanho do que chega por HTTP.
const (
	maxImportFileBytes = 20 << 20  // 20 MB de CSV
	maxProfileBytes    = 256 << 10 // 256 KB de JSON de perfil
)

// Routes são as rotas do módulo.
func (s *Service) Routes() []platform.Route {
	return []platform.Route{
		{Method: "POST", Path: "/packages/{name}/import", Handler: s.handleImport},
		{Method: "POST", Path: "/imports/inspect", Handler: s.handleInspect},
		{Method: "GET", Path: "/import-profiles", Handler: s.handleListProfiles},
		{Method: "POST", Path: "/import-profiles", Handler: s.handleCreateProfile},
		{Method: "GET", Path: "/import-profiles/{name}", Handler: s.handleGetProfile},
		{Method: "PUT", Path: "/import-profiles/{name}", Handler: s.handleReplaceProfile},
		{Method: "PATCH", Path: "/import-profiles/{name}", Handler: s.handleRenameProfile},
		{Method: "DELETE", Path: "/import-profiles/{name}", Handler: s.handleDeleteProfile},
	}
}

// ---------- perfis ----------

// POST /import-profiles  (corpo: o perfil em JSON)
func (s *Service) handleCreateProfile(w http.ResponseWriter, r *http.Request) {
	p, ok := s.decodeProfile(w, r)
	if !ok {
		return
	}
	if err := s.CreateProfile(r.Context(), p, platform.UserFrom(r)); err != nil {
		if errors.Is(err, platform.ErrDuplicate) {
			platform.WriteError(w, http.StatusConflict, fmt.Sprintf("perfil %q já existe: para alterar, use PUT /import-profiles/%s", p.Name, p.Name))
			return
		}
		platform.ServerError(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusCreated, p)
}

// PUT /import-profiles/{name}
// O fluxo típico: o ensaio mostrou "Garoa" sem tradução, você adiciona no
// map e manda o perfil de novo.
func (s *Service) handleReplaceProfile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	p, ok := s.decodeProfile(w, r)
	if !ok {
		return
	}
	if p.Name != name {
		platform.WriteError(w, http.StatusBadRequest, fmt.Sprintf("o nome no corpo (%q) é diferente do nome na URL (%q)", p.Name, name))
		return
	}
	if err := s.ReplaceProfile(r.Context(), p); err != nil {
		if errors.Is(err, platform.ErrNotFound) {
			platform.WriteError(w, http.StatusNotFound, fmt.Sprintf("perfil %q não existe: crie com POST /import-profiles", name))
			return
		}
		platform.ServerError(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, p)
}

// PATCH /import-profiles/{name}   {"name": "novo-nome"}
// Só renomeia. Para mudar o conteúdo, use o PUT com o JSON inteiro.
func (s *Service) handleRenameProfile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
	var body struct {
		Name string `json:"name"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		platform.WriteError(w, http.StatusBadRequest, `JSON inválido: esperado {"name": "..."} (para mudar o conteúdo, use o PUT)`)
		return
	}

	p, err := s.RenameProfile(r.Context(), name, body.Name)
	var invalid *InvalidError
	switch {
	case errors.As(err, &invalid):
		platform.WriteError(w, http.StatusBadRequest, invalid.Msg)
	case errors.Is(err, platform.ErrNotFound):
		platform.WriteError(w, http.StatusNotFound, fmt.Sprintf("perfil %q não existe", name))
	case errors.Is(err, platform.ErrDuplicate):
		platform.WriteError(w, http.StatusConflict, fmt.Sprintf("já existe um perfil %q", strings.TrimSpace(body.Name)))
	case err != nil:
		platform.ServerError(w, err)
	default:
		platform.WriteJSON(w, http.StatusOK, p)
	}
}

// DELETE /import-profiles/{name}
// Nenhuma imagem depende do perfil: os rascunhos que ele já preencheu ficam.
func (s *Service) handleDeleteProfile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	err := s.DeleteProfile(r.Context(), name)
	var invalid *InvalidError
	if errors.As(err, &invalid) {
		platform.WriteError(w, http.StatusBadRequest, invalid.Msg)
		return
	}
	if errors.Is(err, platform.ErrNotFound) {
		platform.WriteError(w, http.StatusNotFound, fmt.Sprintf("perfil %q não existe", name))
		return
	}
	if err != nil {
		platform.ServerError(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, map[string]any{"name": name, "deleted": true})
}

// decodeProfile lê e VALIDA o perfil (contra o perfil de anotação dele). Os
// erros de validação vão todos juntos, para a pessoa corrigir o JSON de uma
// vez.
func (s *Service) decodeProfile(w http.ResponseWriter, r *http.Request) (*importer.Profile, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxProfileBytes)
	var p importer.Profile
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields() // "colum" em vez de "column" vira erro, não é ignorado
	if err := dec.Decode(&p); err != nil {
		platform.WriteError(w, http.StatusBadRequest, "JSON inválido: "+err.Error())
		return nil, false
	}
	p.Builtin = false // só o código marca perfil padrão
	var invalid *InvalidError
	if err := s.Validate(r.Context(), &p); errors.As(err, &invalid) {
		platform.WriteError(w, http.StatusBadRequest, invalid.Msg)
		return nil, false
	} else if err != nil {
		platform.ServerError(w, err)
		return nil, false
	}
	return &p, true
}

// GET /import-profiles?q=&limit=&skip=
func (s *Service) handleListProfiles(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	profiles, total, err := s.ListProfiles(r.Context(), strings.TrimSpace(q.Get("q")),
		platform.IntParam(q, "limit", 20, 1, 100),
		platform.IntParam(q, "skip", 0, 0, 1<<31))
	if err != nil {
		platform.ServerError(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, platform.Page[importer.Profile]{Total: total, Items: profiles})
}

// GET /import-profiles/{name}
func (s *Service) handleGetProfile(w http.ResponseWriter, r *http.Request) {
	p, ok := s.profileByName(w, r, r.PathValue("name"))
	if !ok {
		return
	}
	platform.WriteJSON(w, http.StatusOK, p)
}

func (s *Service) profileByName(w http.ResponseWriter, r *http.Request, name string) (*importer.Profile, bool) {
	p, err := s.FindProfile(r.Context(), name)
	if errors.Is(err, platform.ErrNotFound) {
		platform.WriteError(w, http.StatusNotFound, fmt.Sprintf("perfil %q não existe", name))
		return nil, false
	}
	if err != nil {
		platform.ServerError(w, err)
		return nil, false
	}
	return p, true
}

// ---------- import ----------

// POST /packages/{name}/import?profile=X&dry_run=true&overwrite=false
// (multipart, campo "file"; com perfil do tipo jpeg-ini, SEM arquivo: usa o
// INI já guardado de cada imagem pendente do pacote)
//
//	dry_run=true (PADRÃO): só o relatório, nada é gravado. Rode primeiro assim.
//	overwrite=false (padrão): só preenche campos vazios; o que alguém já
//	                          anotou é mantido.
//	overwrite=true: o valor do CSV substitui o do rascunho.
func (s *Service) handleImport(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	run := Run{
		Package:   r.PathValue("name"),
		User:      platform.UserFrom(r),
		DryRun:    q.Get("dry_run") != "false", // por segurança, só grava se pedir explicitamente
		Overwrite: q.Get("overwrite") == "true",
	}

	if q.Get("profile") == "" {
		platform.WriteError(w, http.StatusBadRequest, "informe o perfil: ?profile=<nome>")
		return
	}
	if _, err := s.catalog.FindPackage(r.Context(), run.Package); err != nil {
		if errors.Is(err, platform.ErrNotFound) {
			platform.WriteError(w, http.StatusNotFound, fmt.Sprintf("pacote %q não existe", run.Package))
			return
		}
		platform.ServerError(w, err)
		return
	}
	var ok bool
	if run.Profile, ok = s.profileByName(w, r, q.Get("profile")); !ok {
		return
	}

	// Perfil de INI: sem arquivo (usa o INI já guardado de cada imagem).
	var table *importer.Table
	if run.Profile.Reader.Type != importer.ReaderJPEGINI {
		if table, ok = readImportFile(w, r, run.Profile.Reader); !ok {
			return
		}
	}

	// A tela pede o andamento: a resposta vem aos poucos, uma linha JSON por
	// passo ({"progress": ...}) e, no fim, {"report": ...} ou {"error": ...}.
	if strings.Contains(r.Header.Get("Accept"), ndjson) {
		s.streamImport(w, r, run, table)
		return
	}

	rep, err := s.Import(r.Context(), run, table)
	var invalid *InvalidError
	switch {
	case errors.As(err, &invalid):
		platform.WriteError(w, http.StatusBadRequest, invalid.Msg)
	case err != nil:
		platform.ServerError(w, err)
	default:
		platform.WriteJSON(w, http.StatusOK, rep)
	}
}
