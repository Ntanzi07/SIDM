package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
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

// GET /images?status=pending&equipment=...&camera=...&day_time=...&weather=...
//
//	&direction=...&vehicle_type=...&vehicle_color=...&plate=...&limit=50&skip=0
func (s *Server) handleListImages(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.ImageFilter{
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
	writeJSON(w, http.StatusOK, map[string]any{"total": total, "items": items})
}

// intParam lê um número da query string, com padrão e limites.
func intParam(q url.Values, key string, def, min, max int64) int64 {
	v, err := strconv.ParseInt(q.Get(key), 10, 64)
	if err != nil {
		return def
	}
	return clamp(v, min, max)
}

func clamp(v, min, max int64) int64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
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
	// true: valida tudo, gera o nome v2, move para dataset/ e marca reviewed.
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

// PUT /images/{sha1}
// Header opcional X-User: quem está anotando (vai para o histórico).
// TODO: quando tiver login, o usuário vem da sessão, não de um header.
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

	// Uma imagem reviewed tem o nome do arquivo amarrado aos metadados.
	// Salvar rascunho nela deixaria nome e banco divergentes.
	if img.Status == store.StatusReviewed && !req.Complete {
		writeError(w, http.StatusConflict, "imagem já revisada: para editar, envie a anotação completa com \"complete\": true")
		return
	}

	before := *img // cópia para o histórico e para desfazer
	req.applyTo(img)
	img.UpdatedAt = time.Now().UTC()

	moved := false
	if req.Complete {
		name, msg := s.finalName(r, img)
		if msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
		newPath := files.DatasetPath(name)
		if newPath != img.Path {
			if err := s.files.Move(img.Path, newPath); err != nil {
				s.serverError(w, err)
				return
			}
			moved = true
		}
		img.Path = newPath
		img.FileName = name
		img.Status = store.StatusReviewed
		img.PatternVersion = filename.CurrentVersion
	}

	if err := s.store.ReplaceImage(r.Context(), img); err != nil {
		if moved { // desfaz o move para disco e banco não divergirem
			_ = s.files.Move(img.Path, before.Path)
		}
		s.serverError(w, err)
		return
	}

	user := r.Header.Get("X-User")
	if user == "" {
		user = "anonymous"
	}
	if err := s.store.InsertHistory(r.Context(), diffHistory(&before, img, user)); err != nil {
		// a anotação já foi salva; falha no histórico só é logada
		s.logError(err)
	}

	writeJSON(w, http.StatusOK, img)
}

// finalName valida a anotação completa e devolve o nome v2 (com extensão).
// Se algo estiver errado, devolve a mensagem para o usuário.
func (s *Server) finalName(r *http.Request, img *store.Image) (string, string) {
	if img.CapturedAt == nil {
		return "", "captured_at é obrigatório para concluir"
	}

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
		CapturedAt:   *img.CapturedAt,
	}
	for _, v := range img.Vehicles {
		meta.Vehicles = append(meta.Vehicles, filename.Vehicle{
			Plate: v.Plate, PlateType: v.PlateType,
			PlateCategory: v.PlateCategory, Type: v.Type, Color: v.Color,
		})
	}

	// O Format já valida todos os códigos e formatos.
	base, err := filename.Format(meta)
	if err != nil {
		return "", err.Error()
	}

	// O Format só valida o FORMATO do equipamento/câmera; aqui conferimos o cadastro.
	ctx := r.Context()
	if ok, err := s.store.EquipmentExists(ctx, img.Equipment); err != nil || !ok {
		return "", fmt.Sprintf("equipamento %q não cadastrado", img.Equipment)
	}
	if ok, err := s.store.CameraExists(ctx, img.Camera); err != nil || !ok {
		return "", fmt.Sprintf("câmera %q não cadastrada", img.Camera)
	}

	return base + img.Ext, ""
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
