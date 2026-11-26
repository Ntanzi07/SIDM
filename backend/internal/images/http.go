package images

import (
	"SIDM/internal/annotation"
	"SIDM/internal/filename"
	"SIDM/internal/files"
	"SIDM/internal/platform"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Routes: as rotas do módulo.
//
//	GET    /codes                    tabelas fixas do padrão v2 (para os dropdowns)
//	GET    /images                   busca com filtros, paginado
//	POST   /images?package=X         upload para o pacote X (pending)
//	GET    /images/stats             contagens de um perfil (pesquisa/dashboard)
//	GET    /images/search            as imagens da pesquisa, paginadas
//	POST   /images/exists            checa hashes antes do upload
//	GET    /images/{sha1}            detalhe
//	GET    /images/{sha1}/file       o arquivo
//	PUT    /images/{sha1}            salva rascunho / verifica (renomeia para v2)
//	DELETE /images/{sha1}            apaga (registro, histórico, caixas e arquivo)
//	POST   /images/{sha1}/reopen     reabre para edição e reserva
//	POST   /images/{sha1}/release    devolve para a fila
//	POST   /packages/{name}/next     a próxima imagem da fila
func (s *Service) Routes() []platform.Route {
	return []platform.Route{
		{Method: "GET", Path: "/codes", Handler: handleCodes},
		{Method: "GET", Path: "/images", Handler: s.handleListImages},
		{Method: "GET", Path: "/images/stats", Handler: s.handleStats},
		{Method: "GET", Path: "/images/search", Handler: s.handleSearch},
		{Method: "POST", Path: "/images", Handler: s.handleUpload},
		{Method: "POST", Path: "/images/exists", Handler: s.handleExists},
		{Method: "GET", Path: "/images/{sha1}", Handler: s.handleGetImage},
		{Method: "GET", Path: "/images/{sha1}/file", Handler: s.handleImageFile},
		{Method: "PUT", Path: "/images/{sha1}", Handler: s.handleAnnotate},
		{Method: "DELETE", Path: "/images/{sha1}", Handler: s.handleDeleteImage},
		{Method: "POST", Path: "/images/{sha1}/reopen", Handler: s.handleReopen},
		{Method: "POST", Path: "/images/{sha1}/release", Handler: s.handleRelease},
		{Method: "POST", Path: "/packages/{name}/next", Handler: s.handleNext},
		{Method: "GET", Path: "/system/storage", Handler: s.handleStorage},
		{Method: "POST", Path: "/system/trash/purge", Handler: s.handlePurgeTrash},
		{Method: "POST", Path: "/packages/{name}/zip-uploads", Handler: s.handleStartZip},
		{Method: "GET", Path: "/packages/{name}/zip-uploads", Handler: s.handleListZips},
		{Method: "GET", Path: "/zip-uploads/{id}", Handler: s.handleGetZip},
		{Method: "PUT", Path: "/zip-uploads/{id}/chunk", Handler: s.handleZipChunk},
		{Method: "POST", Path: "/zip-uploads/{id}/finish", Handler: s.handleFinishZip},
		{Method: "DELETE", Path: "/zip-uploads/{id}", Handler: s.handleCancelZip},
	}
}

// handleCodes devolve as tabelas fixas do padrão v2 (direção, clima, cor...).
// Equipamentos e câmeras NÃO vêm aqui: podem ser milhares, então têm busca
// própria e paginada (GET /equipments e GET /cameras).
func handleCodes(w http.ResponseWriter, r *http.Request) {
	platform.WriteJSON(w, http.StatusOK, filename.Codes())
}

var sha1Re = regexp.MustCompile(`^[0-9a-f]{40}$`)

// Os filtros de antes dos perfis (perfil veiculos).
var (
	legacyFields = map[string]bool{"equipment": true, "camera": true, "day_time": true, "weather": true, "direction": true}
	legacyObject = map[string]string{"vehicle_type": "type", "vehicle_color": "color", "plate": "plate"}
)

// imageFromPath lê o {sha1} da URL e busca a imagem. Se der errado, já
// escreve a resposta de erro e retorna ok=false.
func (s *Service) imageFromPath(w http.ResponseWriter, r *http.Request) (*Image, bool) {
	id := r.PathValue("sha1")
	if !sha1Re.MatchString(id) {
		platform.WriteError(w, http.StatusBadRequest, "sha1 inválido")
		return nil, false
	}
	img, err := s.repo.FindImage(r.Context(), id)
	if errors.Is(err, platform.ErrNotFound) {
		platform.WriteError(w, http.StatusNotFound, "imagem não encontrada")
		return nil, false
	}
	if err != nil {
		platform.ServerError(w, err)
		return nil, false
	}
	return img, true
}

// GET /images?package=...&status=pending&fields.<campo>=...&objects.<campo>=...&limit=50&skip=0
//
// fields.<campo>: um campo da imagem (numa lista, basta conter o valor).
// objects.<campo>: todos valendo para o MESMO objeto (ex.: objects.type=truck
// e objects.color=white é um caminhão branco).
//
// Os nomes de antes dos perfis continuam valendo: equipment, camera,
// day_time, weather, direction (= fields.*) e vehicle_type, vehicle_color,
// plate (= objects.type, objects.color, objects.plate).
func (s *Service) handleListImages(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := ImageFilter{
		Package: q.Get("package"),
		Status:  q.Get("status"),
		Fields:  map[string]any{},
		Object:  map[string]any{},
		Limit:   platform.IntParam(q, "limit", 50, 1, 500),
		Skip:    platform.IntParam(q, "skip", 0, 0, 1<<31),
	}
	for key, vals := range q {
		v := strings.TrimSpace(vals[0])
		if v == "" {
			continue
		}
		switch {
		case strings.HasPrefix(key, "fields."):
			f.Fields[strings.TrimPrefix(key, "fields.")] = v
		case strings.HasPrefix(key, "objects."):
			f.Object[strings.TrimPrefix(key, "objects.")] = v
		case legacyFields[key]:
			f.Fields[key] = v
		case legacyObject[key] != "":
			f.Object[legacyObject[key]] = v
		}
	}
	if p, ok := f.Object["plate"].(string); ok {
		f.Object["plate"] = strings.ToUpper(p) // placa é guardada em maiúsculas
	}

	items, total, err := s.repo.ListImages(r.Context(), f)
	if err != nil {
		platform.ServerError(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, platform.Page[Image]{Total: total, Items: items})
}

// GET /images/{sha1}
func (s *Service) handleGetImage(w http.ResponseWriter, r *http.Request) {
	img, ok := s.imageFromPath(w, r)
	if !ok {
		return
	}
	platform.WriteJSON(w, http.StatusOK, img)
}

// GET /images/{sha1}/file — o arquivo da imagem, para o front exibir.
// http.ServeContent cuida de Content-Type, cache (If-Modified-Since) e Range.
func (s *Service) handleImageFile(w http.ResponseWriter, r *http.Request) {
	img, ok := s.imageFromPath(w, r)
	if !ok {
		return
	}
	f, err := os.Open(s.files.Abs(img.Path))
	if err != nil {
		platform.ServerError(w, fmt.Errorf("abrir %s: %w", img.Path, err))
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		platform.ServerError(w, err)
		return
	}
	http.ServeContent(w, r, filepath.Base(img.Path), info.ModTime(), f)
}

// DELETE /images/{sha1}
// Apaga a imagem do pacote, mas ela vai para a LIXEIRA: o registro inteiro
// (images_trash) e o arquivo (lixeira/<sha1>.ext). Assim ela volta por uma
// versão (POST /datasets/{name}/versions/{number}/items/{sha1}/restore). As
// caixas saem (a versão tem as delas); o histórico fica.
// Se outra pessoa está anotando, recusa (409).
func (s *Service) handleDeleteImage(w http.ResponseWriter, r *http.Request) {
	img, ok := s.imageFromPath(w, r)
	if !ok {
		return
	}
	user := platform.UserFrom(r)
	if img.ClaimedByOther(user, time.Now().UTC()) {
		platform.WriteError(w, http.StatusConflict, fmt.Sprintf("imagem está sendo anotada por %s", img.ClaimedBy))
		return
	}

	// Primeiro a lixeira, depois o banco, depois o disco: se algo cair no
	// meio, o pior caso é um arquivo fora do lugar, nunca uma imagem perdida.
	ctx := r.Context()
	if err := s.repo.PutTrash(ctx, &Trash{Image: *img, DeletedBy: user, DeletedAt: time.Now().UTC()}); err != nil {
		platform.ServerError(w, err)
		return
	}
	if err := s.repo.DeleteImage(ctx, img.SHA1); err != nil {
		platform.ServerError(w, err)
		return
	}
	warnings := []string{}
	if err := s.imageDeleted(ctx, img.SHA1); err != nil {
		platform.LogError(err)
		warnings = append(warnings, "a imagem foi apagada, mas as caixas dela não")
	}
	if err := s.files.MoveTo(img.Path, files.TrashPath(img.SHA1, img.Ext)); err != nil {
		platform.LogError(err)
		warnings = append(warnings, "a imagem foi apagada, mas o arquivo não foi para a lixeira: ficou em "+img.Path)
	}
	platform.WriteJSON(w, http.StatusOK, map[string]any{"sha1": img.SHA1, "deleted": true, "trash": true, "warnings": warnings})
}

// PUT /images/{sha1}
// Header opcional X-User: quem está anotando (vai para o histórico).
func (s *Service) handleAnnotate(w http.ResponseWriter, r *http.Request) {
	img, ok := s.imageFromPath(w, r)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var req annotationRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields() // campo com nome errado vira erro, não é ignorado em silêncio
	if err := dec.Decode(&req); err != nil {
		platform.WriteError(w, http.StatusBadRequest, "JSON inválido: "+err.Error())
		return
	}

	user := platform.UserFrom(r)
	now := time.Now().UTC()

	// Fila de trabalho: se outra pessoa está com a imagem, ninguém mais edita.
	if img.ClaimedByOther(user, now) {
		platform.WriteError(w, http.StatusConflict, fmt.Sprintf("imagem está sendo anotada por %s", img.ClaimedBy))
		return
	}

	// Uma imagem verified tem o nome do arquivo amarrado aos metadados.
	// Salvar rascunho nela deixaria nome e banco divergentes.
	if img.Status == StatusVerified && !req.Complete {
		platform.WriteError(w, http.StatusConflict, "imagem já verificada: para editar, envie a anotação completa com \"complete\": true")
		return
	}

	// A anotação passa pelo perfil do pacote: limpa (espaços, maiúsculas,
	// datas em UTC) e, ao verificar, cobra os obrigatórios.
	profile, err := s.ProfileOf(r.Context(), img.Package)
	if err != nil {
		platform.ServerError(w, err)
		return
	}
	clean, err := profile.Check(annotation.Values{Fields: req.Fields, Objects: req.Objects}, req.Complete)
	if err != nil {
		writeProblems(w, err)
		return
	}

	before := *img // cópia para o histórico, para desfazer e para o optimistic locking
	img.Fields, img.Objects = clean.Fields, clean.Objects
	img.UpdatedAt = now

	if req.Complete {
		// O arquivo NÃO muda de nome: no disco toda imagem é <sha1>.ext. O
		// nome v2 fica no banco (file_name), e o export escolhe o nome de saída.
		name, version, msg := s.finalName(r.Context(), img, profile)
		if msg != "" {
			platform.WriteError(w, http.StatusBadRequest, msg)
			return
		}
		img.FileName = name
		img.Status = StatusVerified
		img.PatternVersion = version
		img.ClaimedBy, img.ClaimedUntil = "", nil // verificada sai da fila
	} else if img.ClaimedBy == user {
		until := now.Add(claimLease) // salvar rascunho renova o prazo
		img.ClaimedUntil = &until
	}

	if err := s.repo.ReplaceImage(r.Context(), img, before.UpdatedAt); err != nil {
		if errors.Is(err, ErrConflict) {
			platform.WriteError(w, http.StatusConflict, "a imagem foi alterada por outra pessoa enquanto você editava: recarregue")
			return
		}
		platform.ServerError(w, err)
		return
	}

	if err := s.repo.InsertHistory(r.Context(), DiffHistory(&before, img, user)); err != nil {
		// a anotação já foi salva; falha no histórico só é logada
		platform.LogError(err)
	}

	platform.WriteJSON(w, http.StatusOK, img)
}

// writeProblems: o erro do Check do perfil, com a lista de problemas
// (cada um com o caminho do campo, para a tela marcar).
func writeProblems(w http.ResponseWriter, err error) {
	var invalid *annotation.InvalidError
	if errors.As(err, &invalid) {
		platform.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": invalid.Msg, "problems": invalid.Problems})
		return
	}
	platform.ServerError(w, err)
}

type existsRequest struct {
	SHA1s []string `json:"sha1s"`
}

type existingImage struct {
	SHA1    string `json:"sha1"`
	Package string `json:"package"` // em qual pacote ela já está
	Status  string `json:"status"`
}

type ExistsResponse struct {
	Existing []existingImage `json:"existing"` // já estão no sistema: o front pula essas
	Missing  []string        `json:"missing"`  // não existem: o front sobe só esses
}

// POST /images/exists
// O front calcula o SHA-1 de cada arquivo no navegador (crypto.subtle.digest)
// e pergunta antes de subir. Assim um lote repetido de GBs nem sai do PC.
// O upload continua conferindo de novo: nunca se confia só no cliente.
func (s *Service) handleExists(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10) // 1000 hashes * ~43 bytes cabe folgado

	var req existsRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		platform.WriteError(w, http.StatusBadRequest, `JSON inválido: esperado {"sha1s": ["...", ...]}`)
		return
	}
	if len(req.SHA1s) == 0 {
		platform.WriteError(w, http.StatusBadRequest, "lista sha1s vazia")
		return
	}
	if len(req.SHA1s) > MaxExistsBatch {
		platform.WriteError(w, http.StatusBadRequest, fmt.Sprintf("máximo de %d hashes por request; divida em lotes", MaxExistsBatch))
		return
	}

	// Normaliza (minúsculo), valida e remove repetidos, mantendo a ordem.
	seen := make(map[string]bool, len(req.SHA1s))
	ids := make([]string, 0, len(req.SHA1s))
	for _, h := range req.SHA1s {
		h = strings.ToLower(strings.TrimSpace(h))
		if !sha1Re.MatchString(h) {
			platform.WriteError(w, http.StatusBadRequest, fmt.Sprintf("sha1 inválido: %q", h))
			return
		}
		if !seen[h] {
			seen[h] = true
			ids = append(ids, h)
		}
	}

	found, err := s.repo.ExistingImages(r.Context(), ids)
	if err != nil {
		platform.ServerError(w, err)
		return
	}

	resp := ExistsResponse{Existing: []existingImage{}, Missing: []string{}}
	for _, id := range ids {
		if e, ok := found[id]; ok {
			resp.Existing = append(resp.Existing, existingImage{SHA1: id, Package: e.Package, Status: e.Status})
		} else {
			resp.Missing = append(resp.Missing, id)
		}
	}
	platform.WriteJSON(w, http.StatusOK, resp)
}

// POST /packages/{name}/next
// Header X-User obrigatório. Corpo opcional: {"skip": "<sha1>"} para pular
// a imagem atual (ela vai para o fim da fila).
//
// 200: a imagem, já reservada para o usuário (claimed_by / claimed_until).
// 204: fila vazia, não há mais imagens pendentes livres neste pacote.
func (s *Service) handleNext(w http.ResponseWriter, r *http.Request) {
	user := r.Header.Get("X-User")
	if user == "" {
		platform.WriteError(w, http.StatusBadRequest, "informe quem está anotando no header X-User")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
	var req struct {
		Skip string `json:"skip"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil && !errors.Is(err, io.EOF) { // corpo vazio é ok
		platform.WriteError(w, http.StatusBadRequest, `JSON inválido: esperado {"skip": "<sha1>"} ou corpo vazio`)
		return
	}
	if req.Skip != "" && !sha1Re.MatchString(req.Skip) {
		platform.WriteError(w, http.StatusBadRequest, "skip: sha1 inválido")
		return
	}

	pkg := r.PathValue("name")
	if _, err := s.catalog.FindPackage(r.Context(), pkg); err != nil {
		if errors.Is(err, platform.ErrNotFound) {
			platform.WriteError(w, http.StatusNotFound, fmt.Sprintf("pacote %q não existe", pkg))
			return
		}
		platform.ServerError(w, err)
		return
	}

	img, err := s.repo.ClaimNext(r.Context(), pkg, user, req.Skip, claimLease, time.Now().UTC())
	if errors.Is(err, platform.ErrNotFound) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		platform.ServerError(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, img)
}

// POST /images/{sha1}/release: devolve a imagem para a fila.
// O front chama ao fechar a aba ou sair da anotação. Se não chamar, tudo
// bem: o prazo vence sozinho em claimLease.
func (s *Service) handleRelease(w http.ResponseWriter, r *http.Request) {
	user := r.Header.Get("X-User")
	if user == "" {
		platform.WriteError(w, http.StatusBadRequest, "informe quem está anotando no header X-User")
		return
	}
	img, ok := s.imageFromPath(w, r)
	if !ok {
		return
	}
	if err := s.repo.ReleaseClaim(r.Context(), img.SHA1, user); err != nil {
		platform.ServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /images/{sha1}/reopen   (X-User obrigatório)
// Reabre uma imagem para edição: uma VERIFICADA volta para pendente (o
// arquivo perde o nome v2 e volta a ser <sha1>.ext; os metadados ficam como
// rascunho) e já fica reservada para quem pediu, para ninguém pegar na fila
// no meio. Numa PENDENTE, só reserva (é o "abrir esta imagem" da tela).
// As caixas do CVAT não mudam: são guardadas pelo sha1.
func (s *Service) handleReopen(w http.ResponseWriter, r *http.Request) {
	user := r.Header.Get("X-User")
	if user == "" {
		platform.WriteError(w, http.StatusBadRequest, "informe quem está anotando no header X-User")
		return
	}
	img, ok := s.imageFromPath(w, r)
	if !ok {
		return
	}
	now := time.Now().UTC()
	if img.ClaimedByOther(user, now) {
		platform.WriteError(w, http.StatusConflict, fmt.Sprintf("imagem está sendo anotada por %s", img.ClaimedBy))
		return
	}

	before := *img
	until := now.Add(claimLease)
	img.ClaimedBy, img.ClaimedUntil = user, &until
	img.UpdatedAt = now

	moved := false
	if img.Status == StatusVerified {
		// No disco a imagem já se chama <sha1>.ext; o move só acontece numa
		// verificada de antes, que ainda tivesse o nome v2 no arquivo.
		pendingPath := files.PendingPath(img.Package, img.SHA1, img.Ext)
		if pendingPath != img.Path {
			if err := s.files.Move(img.Path, pendingPath); err != nil {
				platform.ServerError(w, err)
				return
			}
			moved = true
		}
		img.Path, img.FileName, img.Status, img.PatternVersion = pendingPath, "", StatusPending, ""
	}

	if err := s.repo.ReplaceImage(r.Context(), img, before.UpdatedAt); err != nil {
		if moved { // banco e disco não podem divergir
			_ = s.files.Move(img.Path, before.Path)
		}
		if errors.Is(err, ErrConflict) {
			platform.WriteError(w, http.StatusConflict, "a imagem foi alterada por outra pessoa agora mesmo: recarregue")
			return
		}
		platform.ServerError(w, err)
		return
	}
	if before.Status != img.Status { // reabriu uma verificada: vai para o histórico
		if err := s.repo.InsertHistory(r.Context(), DiffHistory(&before, img, user)); err != nil {
			platform.LogError(err)
		}
	}
	platform.WriteJSON(w, http.StatusOK, img)
}
