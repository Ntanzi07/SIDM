package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"SIDM/internal/filename"
	"SIDM/internal/platform"
)

// Routes: as rotas do catálogo.
//
//	GET    /packages?q=&limit=&skip=      pacotes com progresso, paginado
//	POST   /packages                      cria {"name": ...}
//	PATCH  /packages/{name}               renomeia (pasta + imagens)
//	DELETE /packages/{name}?confirm=<name> apaga pacote, imagens e arquivos
//	GET    /equipments?q=&limit=&skip=    busca equipamentos ativos (autocomplete)
//	POST   /equipments                    cadastra {"name": ...}
//	PATCH  /equipments/{name}             renomeia (cascata) e/ou ativa/desativa
//	DELETE /equipments/{name}             apaga, ou desativa se estiver em uso
//	(e o mesmo para /cameras)
func (s *Service) Routes() []platform.Route {
	return []platform.Route{
		{Method: "GET", Path: "/packages", Handler: s.handleListPackages},
		{Method: "POST", Path: "/packages", Handler: s.handleCreatePackage},
		{Method: "PATCH", Path: "/packages/{name}", Handler: s.handleUpdatePackage},
		{Method: "DELETE", Path: "/packages/{name}", Handler: s.handleDeletePackage},

		{Method: "GET", Path: "/equipments", Handler: s.handleSearch(Equipments)},
		{Method: "POST", Path: "/equipments", Handler: s.handleCreate(Equipments, filename.SanitizeEquipment)},
		{Method: "PATCH", Path: "/equipments/{name}", Handler: s.handleUpdate(Equipments, filename.SanitizeEquipment)},
		{Method: "DELETE", Path: "/equipments/{name}", Handler: s.handleRemove(Equipments)},
		{Method: "GET", Path: "/cameras", Handler: s.handleSearch(Cameras)},
		{Method: "POST", Path: "/cameras", Handler: s.handleCreate(Cameras, filename.SanitizeCamera)},
		{Method: "PATCH", Path: "/cameras/{name}", Handler: s.handleUpdate(Cameras, filename.SanitizeCamera)},
		{Method: "DELETE", Path: "/cameras/{name}", Handler: s.handleRemove(Cameras)},
	}
}

// PackageByName busca o pacote e já responde 404/500 se não der. Para os
// handlers dos outros módulos que recebem um pacote na URL.
func (s *Service) PackageByName(w http.ResponseWriter, r *http.Request, name string) (*Package, bool) {
	p, err := s.repo.FindPackage(r.Context(), name)
	if errors.Is(err, platform.ErrNotFound) {
		platform.WriteError(w, http.StatusNotFound, fmt.Sprintf("pacote %q não existe", name))
		return nil, false
	}
	if err != nil {
		platform.ServerError(w, err)
		return nil, false
	}
	return p, true
}

// decodeName lê {"name": "..."} (corpo de no máximo 1 KB).
func decodeName(w http.ResponseWriter, r *http.Request, strict bool) (string, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
	var req struct {
		Name string `json:"name"`
	}
	dec := json.NewDecoder(r.Body)
	if strict {
		dec.DisallowUnknownFields()
	}
	if err := dec.Decode(&req); err != nil {
		platform.WriteError(w, http.StatusBadRequest, `JSON inválido: esperado {"name": "..."}`)
		return "", false
	}
	return req.Name, true
}

// searchQuery lê ?q= (no máximo 64 caracteres).
func searchQuery(w http.ResponseWriter, r *http.Request) (string, bool) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) > 64 {
		platform.WriteError(w, http.StatusBadRequest, "busca longa demais (máx. 64 caracteres)")
		return "", false
	}
	return q, true
}

// packageBody é o corpo do POST e do PATCH de pacote.
type packageBody struct {
	Name    *string `json:"name"`
	Profile *string `json:"profile"` // perfil de anotação
}

func decodePackage(w http.ResponseWriter, r *http.Request) (packageBody, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
	var b packageBody
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&b); err != nil {
		platform.WriteError(w, http.StatusBadRequest, `JSON inválido: esperado {"name": "...", "profile": "..."}`)
		return b, false
	}
	return b, true
}

// writeErr: InvalidError vira 400, o resto 500.
func writeErr(w http.ResponseWriter, err error) {
	var invalid *InvalidError
	if errors.As(err, &invalid) {
		platform.WriteError(w, http.StatusBadRequest, invalid.Msg)
		return
	}
	platform.ServerError(w, err)
}

// POST /packages {"name": "Noturnas Setembro", "profile": "veiculos"}
// O nome é sanitizado ("Noturnas-Setembro") porque vira nome de pasta. Sem
// profile, o pacote usa o perfil de anotação padrão.
func (s *Service) handleCreatePackage(w http.ResponseWriter, r *http.Request) {
	b, ok := decodePackage(w, r)
	if !ok {
		return
	}
	raw, profile := "", ""
	if b.Name != nil {
		raw = *b.Name
	}
	if b.Profile != nil {
		profile = *b.Profile
	}
	name, err := filename.SanitizePackage(raw)
	if err != nil {
		platform.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.CreatePackage(r.Context(), name, profile, platform.UserFrom(r)); err != nil {
		if errors.Is(err, platform.ErrDuplicate) {
			platform.WriteError(w, http.StatusConflict, fmt.Sprintf("pacote %q já existe", name))
			return
		}
		writeErr(w, err)
		return
	}
	p, err := s.FindPackage(r.Context(), name)
	if err != nil {
		platform.ServerError(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusCreated, map[string]string{"name": name, "profile": p.Profile})
}

// GET /packages?q=noturnas&limit=20&skip=0
// Pacotes (mais novos primeiro) com o progresso de cada um.
func (s *Service) handleListPackages(w http.ResponseWriter, r *http.Request) {
	query, ok := searchQuery(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	pkgs, total, err := s.ListPackages(r.Context(), query,
		platform.IntParam(q, "limit", 20, 1, 100),
		platform.IntParam(q, "skip", 0, 0, 1<<31))
	if err != nil {
		platform.ServerError(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, platform.Page[PackageSummary]{Total: total, Items: pkgs})
}

// PATCH /packages/{name}  {"name": "novo nome"} e/ou {"profile": "outro-perfil"}
// O perfil só pode ser trocado enquanto o pacote não tem imagem verificada.
// Com os dois, o perfil é trocado primeiro (se ele for recusado, nada muda).
func (s *Service) handleUpdatePackage(w http.ResponseWriter, r *http.Request) {
	oldName := r.PathValue("name")
	if _, ok := s.PackageByName(w, r, oldName); !ok {
		return
	}
	b, ok := decodePackage(w, r)
	if !ok {
		return
	}
	if b.Name == nil && b.Profile == nil {
		platform.WriteError(w, http.StatusBadRequest, `nada para mudar: envie "name" e/ou "profile"`)
		return
	}
	if b.Profile != nil {
		if err := s.SetPackageProfile(r.Context(), oldName, *b.Profile); err != nil {
			writeErr(w, err)
			return
		}
	}
	name, n := oldName, int64(0)
	if b.Name != nil {
		newName, err := filename.SanitizePackage(*b.Name)
		if err != nil {
			platform.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		n, err = s.RenamePackage(r.Context(), oldName, newName)
		switch {
		case errors.Is(err, platform.ErrDuplicate):
			platform.WriteError(w, http.StatusConflict, fmt.Sprintf("já existe um pacote %q", newName))
			return
		case err != nil:
			platform.ServerError(w, err)
			return
		}
		name = newName
	}
	p, err := s.FindPackage(r.Context(), name)
	if err != nil {
		platform.ServerError(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, map[string]any{"name": name, "profile": p.Profile, "images": n})
}

// DELETE /packages/{name}?confirm=<name>
// IRREVERSÍVEL: por isso exige repetir o nome em ?confirm=.
func (s *Service) handleDeletePackage(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if r.URL.Query().Get("confirm") != name {
		platform.WriteError(w, http.StatusBadRequest, fmt.Sprintf(
			"isto apaga o pacote, todas as imagens e os arquivos, sem volta. Para confirmar, repita o nome: ?confirm=%s", name))
		return
	}
	if _, ok := s.PackageByName(w, r, name); !ok {
		return
	}
	res, err := s.DeletePackage(r.Context(), name)
	if err != nil {
		platform.ServerError(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, map[string]any{"name": name, "deleted_images": res.Images, "warnings": res.Warnings})
}

type nameItem struct {
	Name string `json:"name"`
}

// GET /equipments?q=mf47&limit=10&skip=0
// Pensado para autocomplete: o front chama a cada letra digitada (com
// debounce) e mostra as primeiras opções. Sem q, lista todos em ordem.
func (s *Service) handleSearch(reg Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query, ok := searchQuery(w, r)
		if !ok {
			return
		}
		q := r.URL.Query()
		// ?inactive=true: os desativados (para reativar em Configurar).
		names, total, err := s.repo.Search(r.Context(), reg, query, q.Get("inactive") != "true",
			platform.IntParam(q, "limit", 10, 1, 50),
			platform.IntParam(q, "skip", 0, 0, 1<<31))
		if err != nil {
			platform.ServerError(w, err)
			return
		}
		items := make([]nameItem, len(names))
		for i, n := range names {
			items[i] = nameItem{Name: n}
		}
		platform.WriteJSON(w, http.StatusOK, platform.Page[nameItem]{Total: total, Items: items})
	}
}

// POST /equipments {"name": ...} (e câmeras)
func (s *Service) handleCreate(reg Registry, sanitize func(string) (string, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, ok := decodeName(w, r, false)
		if !ok {
			return
		}
		name, err := sanitize(raw)
		if err != nil {
			platform.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.repo.Create(r.Context(), reg, name); err != nil {
			if errors.Is(err, platform.ErrDuplicate) {
				platform.WriteError(w, http.StatusConflict, fmt.Sprintf("%q já está cadastrado", name))
				return
			}
			platform.ServerError(w, err)
			return
		}
		platform.WriteJSON(w, http.StatusCreated, map[string]string{"name": name})
	}
}

func (s *Service) entryByName(w http.ResponseWriter, r *http.Request, reg Registry, name string) (*RegistryEntry, bool) {
	e, err := s.repo.Find(r.Context(), reg, name)
	if errors.Is(err, platform.ErrNotFound) {
		platform.WriteError(w, http.StatusNotFound, fmt.Sprintf("%s %q não existe", reg.ImageField(), name))
		return nil, false
	}
	if err != nil {
		platform.ServerError(w, err)
		return nil, false
	}
	return e, true
}

// PATCH /equipments/{name}  e  PATCH /cameras/{name}
//
//	{"name": "8071-MFR2"}  renomeia em CASCATA: todas as imagens que usam o
//	                       nome antigo passam a usar o novo; nas verificadas,
//	                       o arquivo é renomeado para o novo nome v2.
//	                       Se o novo nome já existir, os dois são FUNDIDOS
//	                       (útil para limpar duplicatas criadas pelo import).
//	{"active": false}      desativa (some do autocomplete e não pode ser
//	                       usado em novas verificações).
func (s *Service) handleUpdate(reg Registry, sanitize func(string) (string, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		oldName := r.PathValue("name")
		if _, ok := s.entryByName(w, r, reg, oldName); !ok {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
		var req struct {
			Name   *string `json:"name"`
			Active *bool   `json:"active"`
		}
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			platform.WriteError(w, http.StatusBadRequest, `JSON inválido: esperado {"name": "..."} e/ou {"active": true|false}`)
			return
		}
		if req.Name == nil && req.Active == nil {
			platform.WriteError(w, http.StatusBadRequest, `nada para alterar: informe "name" e/ou "active"`)
			return
		}
		if req.Name != nil {
			clean, err := sanitize(*req.Name)
			if err != nil {
				platform.WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			req.Name = &clean
		}
		rep, err := s.Update(r.Context(), reg, oldName, req.Name, req.Active, platform.UserFrom(r))
		if err != nil {
			platform.ServerError(w, err)
			return
		}
		platform.WriteJSON(w, http.StatusOK, rep)
	}
}

// DELETE /equipments/{name}  e  DELETE /cameras/{name}
// Em uso por alguma imagem: DESATIVA (as imagens continuam intactas).
// Sem uso: apaga de vez.
func (s *Service) handleRemove(reg Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if _, ok := s.entryByName(w, r, reg, name); !ok {
			return
		}
		res, err := s.Remove(r.Context(), reg, name)
		if err != nil {
			platform.ServerError(w, err)
			return
		}
		platform.WriteJSON(w, http.StatusOK, res)
	}
}
