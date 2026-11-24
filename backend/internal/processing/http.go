package processing

import (
	"encoding/json"
	"errors"
	"net/http"

	"SIDM/internal/boxes"
	"SIDM/internal/platform"
)

// Routes são as rotas do módulo. Iniciar e cancelar são só de admin (ficam
// fora do userWrites do users).
//
//	GET  /processing/models            os modelos ligados, com o estado (disponível, ocupado)
//	GET  /processing/runs              os últimos processamentos, mais novos primeiro
//	POST /processing/runs              inicia um (202): {model, packages, scope, task?, min_score?}
//	GET  /processing/runs/{id}         um processamento, com o progresso
//	POST /processing/runs/{id}/cancel  pede para parar
func (s *Service) Routes() []platform.Route {
	return []platform.Route{
		{Method: "GET", Path: "/processing/models", Handler: s.handleModels},
		{Method: "GET", Path: "/processing/runs", Handler: s.handleListRuns},
		{Method: "POST", Path: "/processing/runs", Handler: s.handleStart},
		{Method: "GET", Path: "/processing/runs/{id}", Handler: s.handleGetRun},
		{Method: "POST", Path: "/processing/runs/{id}/cancel", Handler: s.handleCancel},
	}
}

func writeErr(w http.ResponseWriter, err error) {
	var invalid *InvalidError
	var boxInvalid *boxes.InvalidError
	var conflict *ConflictError
	var unavailable *UnavailableError
	switch {
	case errors.As(err, &invalid):
		platform.WriteError(w, http.StatusBadRequest, invalid.Msg)
	case errors.As(err, &boxInvalid):
		platform.WriteError(w, http.StatusBadRequest, "tarefa: "+boxInvalid.Msg)
	case errors.As(err, &conflict):
		platform.WriteError(w, http.StatusConflict, conflict.Msg)
	case errors.As(err, &unavailable):
		platform.WriteError(w, http.StatusServiceUnavailable, unavailable.Msg)
	case errors.Is(err, platform.ErrNotFound):
		platform.WriteError(w, http.StatusNotFound, "processamento não encontrado")
	default:
		platform.ServerError(w, err)
	}
}

func (s *Service) handleModels(w http.ResponseWriter, r *http.Request) {
	platform.WriteJSON(w, http.StatusOK, s.Models(r.Context()))
}

func (s *Service) handleListRuns(w http.ResponseWriter, r *http.Request) {
	limit := platform.IntParam(r.URL.Query(), "limit", 50, 1, 200)
	runs, err := s.ListRuns(r.Context(), limit)
	if err != nil {
		writeErr(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, runs)
}

func (s *Service) handleStart(w http.ResponseWriter, r *http.Request) {
	var in RunInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		platform.WriteError(w, http.StatusBadRequest, "JSON inválido: "+err.Error())
		return
	}
	run, err := s.Start(r.Context(), in, platform.UserFrom(r))
	if err != nil {
		writeErr(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusAccepted, run)
}

func (s *Service) handleGetRun(w http.ResponseWriter, r *http.Request) {
	run, err := s.FindRun(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, run)
}

func (s *Service) handleCancel(w http.ResponseWriter, r *http.Request) {
	run, err := s.Cancel(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, run)
}
