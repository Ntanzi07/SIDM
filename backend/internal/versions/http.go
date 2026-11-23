package versions

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"SIDM/internal/platform"
)

// Routes são as rotas do módulo.
//
//	GET    /datasets                               os datasets, com a versão mais nova
//	POST   /datasets                               cria (X-User)
//	GET    /datasets/{name}                        um dataset
//	PATCH  /datasets/{name}                        troca a descrição, o perfil e a query
//	DELETE /datasets/{name}                        apaga (só sem versões)
//	GET    /datasets/{name}/versions               as versões, da mais nova para a mais velha
//	POST   /datasets/{name}/versions               congela uma versão nova (X-User, {"message"})
//	GET    /datasets/{name}/versions/{number}      uma versão, com o resumo
//	GET    /datasets/{name}/versions/{number}/items  as imagens congeladas, paginadas
//	GET    /datasets/{name}/versions/{number}/stats  a pesquisa dentro da versão (?package=&fields.x=&task=)
//	GET    /datasets/{name}/diff?from=&to=         o que mudou entre duas versões (&summary=1: só as contas)
//	GET    /datasets/{name}/diff/{sha1}?from=&to=  o que mudou numa imagem
func (s *Service) Routes() []platform.Route {
	return []platform.Route{
		{Method: "GET", Path: "/datasets", Handler: s.handleListDatasets},
		{Method: "POST", Path: "/datasets", Handler: s.handleCreateDataset},
		{Method: "GET", Path: "/datasets/{name}", Handler: s.handleGetDataset},
		{Method: "PATCH", Path: "/datasets/{name}", Handler: s.handleUpdateDataset},
		{Method: "DELETE", Path: "/datasets/{name}", Handler: s.handleDeleteDataset},
		{Method: "GET", Path: "/datasets/{name}/versions", Handler: s.handleListVersions},
		{Method: "POST", Path: "/datasets/{name}/versions", Handler: s.handleCreateVersion},
		{Method: "GET", Path: "/datasets/{name}/versions/{number}", Handler: s.handleGetVersion},
		{Method: "GET", Path: "/datasets/{name}/versions/{number}/items", Handler: s.handleItems},
		{Method: "GET", Path: "/datasets/{name}/versions/{number}/stats", Handler: s.handleVersionStats},
		{Method: "GET", Path: "/datasets/{name}/diff", Handler: s.handleDiff},
		{Method: "GET", Path: "/datasets/{name}/diff/{sha1}", Handler: s.handleDiffItem},
	}
}

// writeErr: 400 para pedido inválido (deste módulo ou da pesquisa do
// images), 404, 409; o resto é 500.
func writeErr(w http.ResponseWriter, err error) {
	var conflict *ConflictError
	switch {
	case errors.Is(err, platform.ErrInvalid):
		platform.WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, platform.ErrNotFound):
		platform.WriteError(w, http.StatusNotFound, "não encontrado")
	case errors.Is(err, platform.ErrDuplicate):
		platform.WriteError(w, http.StatusConflict, "já existe um dataset com esse nome")
	case errors.As(err, &conflict):
		platform.WriteError(w, http.StatusConflict, conflict.Msg)
	default:
		platform.ServerError(w, err)
	}
}

// user: quem está mexendo (X-User obrigatório para criar).
func user(w http.ResponseWriter, r *http.Request) (string, bool) {
	u := r.Header.Get("X-User")
	if u == "" {
		platform.WriteError(w, http.StatusBadRequest, "informe quem está criando no header X-User")
		return "", false
	}
	return u, true
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v); err != nil {
		platform.WriteError(w, http.StatusBadRequest, "JSON inválido: "+err.Error())
		return false
	}
	return true
}

func (s *Service) handleListDatasets(w http.ResponseWriter, r *http.Request) {
	list, err := s.ListDatasets(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, platform.Page[Dataset]{Total: int64(len(list)), Items: list})
}

func (s *Service) handleCreateDataset(w http.ResponseWriter, r *http.Request) {
	u, ok := user(w, r)
	if !ok {
		return
	}
	var in DatasetInput
	if !decode(w, r, &in) {
		return
	}
	d, err := s.CreateDataset(r.Context(), in, u)
	if err != nil {
		writeErr(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusCreated, d)
}

func (s *Service) handleGetDataset(w http.ResponseWriter, r *http.Request) {
	d, err := s.FindDataset(r.Context(), r.PathValue("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, d)
}

func (s *Service) handleUpdateDataset(w http.ResponseWriter, r *http.Request) {
	var in DatasetInput
	if !decode(w, r, &in) {
		return
	}
	d, err := s.UpdateDataset(r.Context(), r.PathValue("name"), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, d)
}

func (s *Service) handleDeleteDataset(w http.ResponseWriter, r *http.Request) {
	if err := s.DeleteDataset(r.Context(), r.PathValue("name")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) handleListVersions(w http.ResponseWriter, r *http.Request) {
	list, err := s.ListVersions(r.Context(), r.PathValue("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, platform.Page[Version]{Total: int64(len(list)), Items: list})
}

func (s *Service) handleCreateVersion(w http.ResponseWriter, r *http.Request) {
	u, ok := user(w, r)
	if !ok {
		return
	}
	var in struct {
		Message string `json:"message"`
	}
	if !decode(w, r, &in) {
		return
	}
	v, err := s.CreateVersion(r.Context(), r.PathValue("name"), in.Message, u)
	if err != nil {
		writeErr(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusCreated, v)
}

// parseNumber: "3" ou "v3".
func parseNumber(raw string) (int, bool) {
	if raw == "atual" || raw == "agora" {
		return Now, true
	}
	if len(raw) > 1 && (raw[0] == 'v' || raw[0] == 'V') {
		raw = raw[1:]
	}
	n, err := strconv.Atoi(raw)
	return n, err == nil && n >= 1
}

// number: o {number} da rota.
func number(w http.ResponseWriter, r *http.Request) (int, bool) {
	n, ok := parseNumber(r.PathValue("number"))
	if !ok {
		platform.WriteError(w, http.StatusBadRequest, "número de versão inválido: "+r.PathValue("number"))
	}
	return n, ok
}

// fromTo: o ?from= e o ?to= do diff (0 = não informado: to é a mais nova e
// from a anterior a ela).
func fromTo(w http.ResponseWriter, r *http.Request) (from, to int, ok bool) {
	q := r.URL.Query()
	for _, p := range []struct {
		key string
		dst *int
	}{{"from", &from}, {"to", &to}} {
		raw := q.Get(p.key)
		if raw == "" {
			continue
		}
		n, valid := parseNumber(raw)
		if !valid {
			platform.WriteError(w, http.StatusBadRequest, "número de versão inválido em "+p.key+": "+raw)
			return 0, 0, false
		}
		*p.dst = n
	}
	return from, to, true
}

func (s *Service) handleGetVersion(w http.ResponseWriter, r *http.Request) {
	n, ok := number(w, r)
	if !ok {
		return
	}
	var v *Version
	var err error
	if n == Now {
		v, err = s.Current(r.Context(), r.PathValue("name"))
	} else {
		v, err = s.FindVersion(r.Context(), r.PathValue("name"), n)
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, v)
}

func (s *Service) handleItems(w http.ResponseWriter, r *http.Request) {
	n, ok := number(w, r)
	if !ok {
		return
	}
	if n == Now {
		platform.WriteError(w, http.StatusBadRequest, "o agora não tem itens gravados: crie uma versão")
		return
	}
	q := r.URL.Query()
	items, total, err := s.Items(r.Context(), r.PathValue("name"), n,
		platform.IntParam(q, "limit", 50, 1, 500), platform.IntParam(q, "skip", 0, 0, 1<<31))
	if err != nil {
		writeErr(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, platform.Page[Item]{Total: total, Items: items})
}

func (s *Service) handleDiff(w http.ResponseWriter, r *http.Request) {
	from, to, ok := fromTo(w, r)
	if !ok {
		return
	}
	d, err := s.Diff(r.Context(), r.PathValue("name"), from, to)
	if err != nil {
		writeErr(w, err)
		return
	}
	if r.URL.Query().Get("summary") == "1" {
		d.Added, d.Removed, d.Changed = []ItemRef{}, []ItemRef{}, []ItemRef{}
	}
	platform.WriteJSON(w, http.StatusOK, d)
}

func (s *Service) handleDiffItem(w http.ResponseWriter, r *http.Request) {
	from, to, ok := fromTo(w, r)
	if !ok {
		return
	}
	d, err := s.DiffItem(r.Context(), r.PathValue("name"), from, to, r.PathValue("sha1"))
	if err != nil {
		writeErr(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, d)
}

func (s *Service) handleVersionStats(w http.ResponseWriter, r *http.Request) {
	n, ok := number(w, r)
	if !ok {
		return
	}
	st, err := s.VersionStats(r.Context(), r.PathValue("name"), n, r.URL.Query())
	if err != nil {
		writeErr(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, st)
}
