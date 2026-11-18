package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"

	"SIDM/internal/filename"
	"SIDM/internal/files"
	"SIDM/internal/store"
)

var sha1Re = regexp.MustCompile(`^[0-9a-f]{40}$`)

// imageFromPath lê o {sha1} da URL e busca a imagem. Se der errado, já
// escreve a resposta de erro e retorna ok=false.
func (s *Server) imageFromPath(w http.ResponseWriter, r *http.Request) (*store.Image, bool) {
	id := r.PathValue("sha1")
	if !sha1Re.MatchString(id) {
		writeError(w, http.StatusBadRequest, "sha1 inválido")
		return nil, false
	}
	img, err := s.store.FindImage(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "imagem não encontrada")
		return nil, false
	}
	if err != nil {
		s.serverError(w, err)
		return nil, false
	}
	return img, true
}

// GET /images?package=...&status=pending&equipment=...&camera=...&day_time=...&weather=...
//
//	&direction=...&vehicle_type=...&vehicle_color=...&plate=...&limit=50&skip=0
func (s *Server) handleListImages(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.ImageFilter{
		Package:      q.Get("package"),
		Status:       q.Get("status"),
		Equipment:    q.Get("equipment"),
		Camera:       q.Get("camera"),
		DayTime:      q.Get("day_time"),
		Weather:      q.Get("weather"),
		Direction:    q.Get("direction"),
		VehicleType:  q.Get("vehicle_type"),
		VehicleColor: q.Get("vehicle_color"),
		Plate:        strings.ToUpper(q.Get("plate")),
		Limit:        intParam(q, "limit", 50, 1, 500),
		Skip:         intParam(q, "skip", 0, 0, 1<<31),
	}

	items, total, err := s.store.ListImages(r.Context(), f)
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page[store.Image]{Total: total, Items: items})
}

// GET /images/{sha1}
func (s *Server) handleGetImage(w http.ResponseWriter, r *http.Request) {
	img, ok := s.imageFromPath(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, img)
}

// GET /images/{sha1}/file — o arquivo da imagem, para o front exibir.
// http.ServeContent cuida de Content-Type, cache (If-Modified-Since) e Range.
func (s *Server) handleImageFile(w http.ResponseWriter, r *http.Request) {
	img, ok := s.imageFromPath(w, r)
	if !ok {
		return
	}
	f, err := os.Open(s.files.Abs(img.Path))
	if err != nil {
		s.serverError(w, fmt.Errorf("abrir %s: %w", img.Path, err))
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		s.serverError(w, err)
		return
	}
	http.ServeContent(w, r, filepath.Base(img.Path), info.ModTime(), f)
}

// annotationRequest é o corpo do PUT /images/{sha1}.
// Substitui TODOS os campos de anotação (não é patch campo a campo).
type annotationRequest struct {
	Equipment    string          `json:"equipment"`
	Camera       string          `json:"camera"`
	Direction    string          `json:"direction"`
	Resolution   string          `json:"resolution"`
	CaptureAngle string          `json:"capture_angle"`
	DayTime      string          `json:"day_time"`
	ImageRange   string          `json:"image_range"`
	Weather      string          `json:"weather"`
	Conditions   []string        `json:"conditions"`
	CapturedAt   *time.Time      `json:"captured_at"`
	Vehicles     []store.Vehicle `json:"vehicles"`

	// false: só salva o rascunho (continua pending).
	// true: valida tudo, renomeia para o padrão v2 (na pasta do pacote) e marca verified.
	Complete bool `json:"complete"`
}

func (req annotationRequest) applyTo(img *store.Image) {
	img.Equipment = req.Equipment
	img.Camera = req.Camera
	img.Direction = req.Direction
	img.Resolution = req.Resolution
	img.CaptureAngle = req.CaptureAngle
	img.DayTime = req.DayTime
	img.ImageRange = req.ImageRange
	img.Weather = req.Weather
	img.Conditions = req.Conditions
	img.Vehicles = make([]store.Vehicle, 0, len(req.Vehicles))
	for _, v := range req.Vehicles {
		v.Plate = strings.ToUpper(strings.TrimSpace(v.Plate)) // placa sempre maiúscula: igual no nome e no banco
		img.Vehicles = append(img.Vehicles, v)
	}
	img.CapturedAt = nil
	if req.CapturedAt != nil {
		t := req.CapturedAt.UTC()
		img.CapturedAt = &t
	}
}

// DELETE /images/{sha1}
// Apaga a imagem: o registro, o histórico, as caixas e o arquivo. Sem volta.
// Se outra pessoa está anotando, recusa (409).
func (s *Server) handleDeleteImage(w http.ResponseWriter, r *http.Request) {
	img, ok := s.imageFromPath(w, r)
	if !ok {
		return
	}
	if img.ClaimedByOther(userFrom(r), time.Now().UTC()) {
		writeError(w, http.StatusConflict, fmt.Sprintf("imagem está sendo anotada por %s", img.ClaimedBy))
		return
	}

	// Mesma ordem do pacote: banco primeiro. Se o disco falhar, sobra um
	// arquivo órfão, mas nunca um registro apontando para o nada.
	ctx := r.Context()
	if err := s.store.DeleteImage(ctx, img.SHA1); err != nil {
		s.serverError(w, err)
		return
	}
	warnings := []string{}
	if err := s.store.DeleteImageHistory(ctx, img.SHA1); err != nil {
		s.logError(err)
		warnings = append(warnings, "a imagem foi apagada, mas o histórico dela não")
	}
	if err := s.boxes.DeleteImage(ctx, img.SHA1); err != nil {
		s.logError(err)
		warnings = append(warnings, "a imagem foi apagada, mas as caixas dela não")
	}
	if err := s.files.Remove(img.Path); err != nil {
		s.logError(err)
		warnings = append(warnings, "a imagem foi apagada do banco, mas o arquivo não: apague "+img.Path+" manualmente")
	}
	writeJSON(w, http.StatusOK, map[string]any{"sha1": img.SHA1, "deleted": true, "warnings": warnings})
}

// PUT /images/{sha1}
// Header opcional X-User: quem está anotando (vai para o histórico).
func (s *Server) handleAnnotate(w http.ResponseWriter, r *http.Request) {
	img, ok := s.imageFromPath(w, r)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var req annotationRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields() // campo com nome errado vira erro, não é ignorado em silêncio
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "JSON inválido: "+err.Error())
		return
	}

	user := userFrom(r)
	now := time.Now().UTC()

	// Fila de trabalho: se outra pessoa está com a imagem, ninguém mais edita.
	if img.ClaimedByOther(user, now) {
		writeError(w, http.StatusConflict, fmt.Sprintf("imagem está sendo anotada por %s", img.ClaimedBy))
		return
	}

	// Uma imagem verified tem o nome do arquivo amarrado aos metadados.
	// Salvar rascunho nela deixaria nome e banco divergentes.
	if img.Status == store.StatusVerified && !req.Complete {
		writeError(w, http.StatusConflict, "imagem já verificada: para editar, envie a anotação completa com \"complete\": true")
		return
	}

	before := *img // cópia para o histórico, para desfazer e para o optimistic locking
	req.applyTo(img)
	img.UpdatedAt = now

	moved := false
	if req.Complete {
		name, msg := s.finalName(r, img)
		if msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
		newPath := files.VerifiedPath(img.Package, name)
		if newPath != img.Path {
			if err := s.files.Move(img.Path, newPath); err != nil {
				s.serverError(w, err)
				return
			}
			moved = true
		}
		img.Path = newPath
		img.FileName = name
		img.Status = store.StatusVerified
		img.PatternVersion = filename.CurrentVersion
		img.ClaimedBy, img.ClaimedUntil = "", nil // verificada sai da fila
	} else if img.ClaimedBy == user {
		until := now.Add(claimLease) // salvar rascunho renova o prazo
		img.ClaimedUntil = &until
	}

	if err := s.store.ReplaceImage(r.Context(), img, before.UpdatedAt); err != nil {
		if moved { // desfaz o move para disco e banco não divergirem
			_ = s.files.Move(img.Path, before.Path)
		}
		if errors.Is(err, store.ErrConflict) {
			writeError(w, http.StatusConflict, "a imagem foi alterada por outra pessoa enquanto você editava: recarregue")
			return
		}
		s.serverError(w, err)
		return
	}

	if err := s.store.InsertHistory(r.Context(), diffHistory(&before, img, user)); err != nil {
		// a anotação já foi salva; falha no histórico só é logada
		s.logError(err)
	}

	writeJSON(w, http.StatusOK, img)
}

// finalName valida a anotação completa e devolve o nome v2 (com extensão).
// Se algo estiver errado, devolve a mensagem para o usuário.
//
// Nenhum campo é obrigatório: o que não deu para saber (sem data, placa
// ilegível, câmera desconhecida...) fica vazio, e no nome vira uma posição
// vazia. O que VEM preenchido tem que ser válido.
func (s *Server) finalName(r *http.Request, img *store.Image) (string, string) {
	meta := metaFromImage(img)

	// O Format já valida todos os códigos e formatos.
	base, err := filename.Format(meta)
	if err != nil {
		return "", err.Error()
	}

	// O Format só valida o FORMATO do equipamento/câmera; aqui conferimos o cadastro.
	ctx := r.Context()
	if img.Equipment != "" {
		if ok, err := s.catalog.EquipmentExists(ctx, img.Equipment); err != nil || !ok {
			return "", fmt.Sprintf("equipamento %q não cadastrado", img.Equipment)
		}
	}
	if img.Camera != "" {
		if ok, err := s.catalog.CameraExists(ctx, img.Camera); err != nil || !ok {
			return "", fmt.Sprintf("câmera %q não cadastrada", img.Camera)
		}
	}

	return base + img.Ext, ""
}

// metaFromImage converte o documento da imagem nos metadados do padrão v2.
// Usado ao verificar e ao renomear em cascata (equipamento/câmera), para o
// nome do arquivo ser montado sempre do mesmo jeito.
func metaFromImage(img *store.Image) filename.ImageMeta {
	meta := filename.ImageMeta{
		SHA1:         img.SHA1,
		Version:      filename.CurrentVersion,
		Equipment:    img.Equipment,
		Camera:       img.Camera,
		Direction:    img.Direction,
		Resolution:   img.Resolution,
		CaptureAngle: img.CaptureAngle,
		DayTime:      img.DayTime,
		ImageRange:   img.ImageRange,
		Weather:      img.Weather,
		Conditions:   img.Conditions,
	}
	if img.CapturedAt != nil {
		meta.CapturedAt = *img.CapturedAt
	}
	for _, v := range img.Vehicles {
		meta.Vehicles = append(meta.Vehicles, filename.Vehicle{
			Plate: v.Plate, PlateType: v.PlateType,
			PlateCategory: v.PlateCategory, Type: v.Type, Color: v.Color,
		})
	}
	return meta
}

// diffHistory gera uma entrada para cada campo que mudou.
func diffHistory(before, after *store.Image, user string) []store.HistoryEntry {
	old, cur := trackedFields(before), trackedFields(after)
	now := time.Now().UTC()

	var entries []store.HistoryEntry
	for field, newVal := range cur {
		if !reflect.DeepEqual(old[field], newVal) {
			entries = append(entries, store.HistoryEntry{
				ImageID:   after.SHA1,
				Field:     field,
				Old:       old[field],
				New:       newVal,
				ChangedBy: user,
				ChangedAt: now,
			})
		}
	}
	return entries
}

func trackedFields(img *store.Image) map[string]any {
	var captured any
	if img.CapturedAt != nil {
		captured = *img.CapturedAt
	}
	return map[string]any{
		"status":        img.Status,
		"file_name":     img.FileName,
		"equipment":     img.Equipment,
		"camera":        img.Camera,
		"direction":     img.Direction,
		"resolution":    img.Resolution,
		"capture_angle": img.CaptureAngle,
		"day_time":      img.DayTime,
		"image_range":   img.ImageRange,
		"weather":       img.Weather,
		"conditions":    img.Conditions,
		"captured_at":   captured,
		"vehicles":      img.Vehicles,
	}
}
