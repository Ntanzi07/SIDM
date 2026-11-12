package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"SIDM/internal/store"
)

// Quanto tempo uma pessoa "segura" uma imagem sem mexer nela.
// Salvar rascunho renova o prazo; depois disso, a imagem volta para a fila.
const claimLease = 15 * time.Minute

// POST /packages/{name}/next
// Header X-User obrigatório. Corpo opcional: {"skip": "<sha1>"} para pular
// a imagem atual (ela vai para o fim da fila).
//
// 200: a imagem, já reservada para o usuário (claimed_by / claimed_until).
// 204: fila vazia, não há mais imagens pendentes livres neste pacote.
func (s *Server) handleNext(w http.ResponseWriter, r *http.Request) {
	user := r.Header.Get("X-User")
	if user == "" {
		writeError(w, http.StatusBadRequest, "informe quem está anotando no header X-User")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
	var req struct {
		Skip string `json:"skip"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil && !errors.Is(err, io.EOF) { // corpo vazio é ok
		writeError(w, http.StatusBadRequest, `JSON inválido: esperado {"skip": "<sha1>"} ou corpo vazio`)
		return
	}
	if req.Skip != "" && !sha1Re.MatchString(req.Skip) {
		writeError(w, http.StatusBadRequest, "skip: sha1 inválido")
		return
	}

	pkg := r.PathValue("name")
	if _, err := s.store.FindPackage(r.Context(), pkg); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, fmt.Sprintf("pacote %q não existe", pkg))
			return
		}
		s.serverError(w, err)
		return
	}

	img, err := s.store.ClaimNext(r.Context(), pkg, user, req.Skip, claimLease, time.Now().UTC())
	if errors.Is(err, store.ErrNotFound) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, img)
}

// POST /images/{sha1}/release: devolve a imagem para a fila.
// O front chama ao fechar a aba ou sair da anotação. Se não chamar, tudo
// bem: o prazo vence sozinho em claimLease.
func (s *Server) handleRelease(w http.ResponseWriter, r *http.Request) {
	user := r.Header.Get("X-User")
	if user == "" {
		writeError(w, http.StatusBadRequest, "informe quem está anotando no header X-User")
		return
	}
	img, ok := s.imageFromPath(w, r)
	if !ok {
		return
	}
	if err := s.store.ReleaseClaim(r.Context(), img.SHA1, user); err != nil {
		s.serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
