package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Quantos hashes podem ser consultados por request.
const maxExistsBatch = 1000

type existsRequest struct {
	SHA1s []string `json:"sha1s"`
}

type existingImage struct {
	SHA1   string `json:"sha1"`
	Status string `json:"status"`
}

type existsResponse struct {
	Existing []existingImage `json:"existing"` // já estão no sistema: não precisa subir
	Missing  []string        `json:"missing"`  // não existem: o front sobe só esses
}

// POST /images/exists
// O front calcula o SHA-1 de cada arquivo no navegador (crypto.subtle.digest)
// e pergunta antes de subir. Assim um lote repetido de GBs nem sai do PC.
// O upload continua conferindo de novo: nunca se confia só no cliente.
func (s *Server) handleExists(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10) // 1000 hashes * ~43 bytes cabe folgado

	var req existsRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, `JSON inválido: esperado {"sha1s": ["...", ...]}`)
		return
	}
	if len(req.SHA1s) == 0 {
		writeError(w, http.StatusBadRequest, "lista sha1s vazia")
		return
	}
	if len(req.SHA1s) > maxExistsBatch {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("máximo de %d hashes por request; divida em lotes", maxExistsBatch))
		return
	}

	// Normaliza (minúsculo), valida e remove repetidos, mantendo a ordem.
	seen := make(map[string]bool, len(req.SHA1s))
	ids := make([]string, 0, len(req.SHA1s))
	for _, h := range req.SHA1s {
		h = strings.ToLower(strings.TrimSpace(h))
		if !sha1Re.MatchString(h) {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("sha1 inválido: %q", h))
			return
		}
		if !seen[h] {
			seen[h] = true
			ids = append(ids, h)
		}
	}

	found, err := s.store.ExistingImages(r.Context(), ids)
	if err != nil {
		s.serverError(w, err)
		return
	}

	resp := existsResponse{Existing: []existingImage{}, Missing: []string{}}
	for _, id := range ids {
		if status, ok := found[id]; ok {
			resp.Existing = append(resp.Existing, existingImage{SHA1: id, Status: status})
		} else {
			resp.Missing = append(resp.Missing, id)
		}
	}
	writeJSON(w, http.StatusOK, resp)
}
