package images

import (
	"SIDM/internal/filename"
	"SIDM/internal/importer"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"
)

// annotationRequest é o corpo do PUT /images/{sha1}.
// Substitui TODOS os campos de anotação (não é patch campo a campo).
type annotationRequest struct {
	Equipment    string     `json:"equipment"`
	Camera       string     `json:"camera"`
	Direction    string     `json:"direction"`
	Resolution   string     `json:"resolution"`
	CaptureAngle string     `json:"capture_angle"`
	DayTime      string     `json:"day_time"`
	ImageRange   string     `json:"image_range"`
	Weather      string     `json:"weather"`
	Conditions   []string   `json:"conditions"`
	CapturedAt   *time.Time `json:"captured_at"`
	Vehicles     []Vehicle  `json:"vehicles"`

	// false: só salva o rascunho (continua pending).
	// true: valida tudo, renomeia para o padrão v2 (na pasta do pacote) e marca verified.
	Complete bool `json:"complete"`
}

func (req annotationRequest) applyTo(img *Image) {
	img.Equipment = req.Equipment
	img.Camera = req.Camera
	img.Direction = req.Direction
	img.Resolution = req.Resolution
	img.CaptureAngle = req.CaptureAngle
	img.DayTime = req.DayTime
	img.ImageRange = req.ImageRange
	img.Weather = req.Weather
	img.Conditions = req.Conditions
	img.Vehicles = make([]Vehicle, 0, len(req.Vehicles))
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

// finalName valida a anotação completa e devolve o nome v2 (com extensão).
// Se algo estiver errado, devolve a mensagem para o usuário.
//
// Nenhum campo é obrigatório: o que não deu para saber (sem data, placa
// ilegível, câmera desconhecida...) fica vazio, e no nome vira uma posição
// vazia. O que VEM preenchido tem que ser válido.
func (s *Service) finalName(r *http.Request, img *Image) (string, string) {
	meta := MetaFromImage(img)

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

// MetaFromImage converte o documento da imagem nos metadados do padrão v2.
// Usado ao verificar e ao renomear em cascata (equipamento/câmera), para o
// nome do arquivo ser montado sempre do mesmo jeito.
func MetaFromImage(img *Image) filename.ImageMeta {
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

// DiffHistory gera uma entrada para cada campo que mudou.
func DiffHistory(before, after *Image, user string) []HistoryEntry {
	old, cur := trackedFields(before), trackedFields(after)
	now := time.Now().UTC()

	var entries []HistoryEntry
	for field, newVal := range cur {
		if !reflect.DeepEqual(old[field], newVal) {
			entries = append(entries, HistoryEntry{
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

func trackedFields(img *Image) map[string]any {
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

func applyMeta(img *Image, m filename.ImageMeta) {
	img.PatternVersion = m.Version
	img.Equipment = m.Equipment
	img.Camera = m.Camera
	img.Direction = m.Direction
	img.Resolution = m.Resolution
	img.CaptureAngle = m.CaptureAngle
	img.DayTime = m.DayTime
	img.ImageRange = m.ImageRange
	img.Weather = m.Weather
	img.Conditions = m.Conditions
	img.CapturedAt = nil
	if !m.CapturedAt.IsZero() { // posição vazia no nome = data desconhecida
		t := m.CapturedAt
		img.CapturedAt = &t
	}
	img.Vehicles = make([]Vehicle, 0, len(m.Vehicles))
	for _, v := range m.Vehicles {
		img.Vehicles = append(img.Vehicles, Vehicle{
			Plate: v.Plate, PlateType: v.PlateType, PlateCategory: v.PlateCategory,
			Type: v.Type, Color: v.Color,
		})
	}
}

// MergeAnnotation aplica o que o import descobriu no rascunho da imagem.
// overwrite=false: só preenche o que está vazio (respeita quem já anotou).
// Devolve true se algo mudou.
func MergeAnnotation(img *Image, a importer.Annotation, overwrite bool) bool {
	changed := false
	set := func(dst *string, v string) {
		if v != "" && *dst != v && (overwrite || *dst == "") {
			*dst = v
			changed = true
		}
	}
	set(&img.Equipment, a.Equipment)
	set(&img.Camera, a.Camera)
	set(&img.Direction, a.Direction)
	set(&img.Resolution, a.Resolution)
	set(&img.CaptureAngle, a.CaptureAngle)
	set(&img.DayTime, a.DayTime)
	set(&img.ImageRange, a.ImageRange)
	set(&img.Weather, a.Weather)

	if len(a.Conditions) > 0 && (overwrite || len(img.Conditions) == 0) && !equalStrings(img.Conditions, a.Conditions) {
		img.Conditions = append([]string(nil), a.Conditions...)
		changed = true
	}
	if a.CapturedAt != nil && (overwrite || img.CapturedAt == nil) &&
		(img.CapturedAt == nil || !img.CapturedAt.Equal(*a.CapturedAt)) {
		t := *a.CapturedAt
		img.CapturedAt = &t
		changed = true
	}
	if len(a.Vehicles) > 0 && (overwrite || len(img.Vehicles) == 0) {
		vehicles := make([]Vehicle, len(a.Vehicles))
		for i, v := range a.Vehicles {
			vehicles[i] = Vehicle{
				Plate: v.Plate, PlateType: v.PlateType, PlateCategory: v.PlateCategory,
				Type: v.Type, Color: v.Color,
			}
		}
		if !equalVehicles(img.Vehicles, vehicles) {
			img.Vehicles = vehicles
			changed = true
		}
	}
	return changed
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalVehicles(a, b []Vehicle) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
