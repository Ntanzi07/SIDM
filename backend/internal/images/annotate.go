package images

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"sort"
	"time"

	"SIDM/internal/annotation"
	"SIDM/internal/filename"
	"SIDM/internal/importer"
)

// annotationRequest é o corpo do PUT /images/{sha1}.
// Substitui TODA a anotação (não é patch campo a campo).
type annotationRequest struct {
	Fields  map[string]any      `json:"fields"`  // os campos da imagem, pelo perfil do pacote
	Objects []annotation.Object `json:"objects"` // os objetos sem caixa (ex.: os veículos)

	// false: só salva o rascunho (continua pending).
	// true: valida tudo (inclusive os obrigatórios), renomeia o arquivo e
	// marca verified.
	Complete bool `json:"complete"`
}

// finalName confere o que o perfil não confere sozinho (o cadastro de
// equipamento e câmera) e devolve o nome do arquivo verificado e a versão do
// padrão. Se algo estiver errado, devolve a mensagem para o usuário.
//
// O perfil padrão (veiculos) usa o nome v2. Os outros perfis ainda não têm
// modelo de nome: o arquivo continua <sha1>.ext, na pasta do pacote.
func (s *Service) finalName(ctx context.Context, img *Image, p *annotation.Profile) (name, version, msg string) {
	for _, ref := range []struct{ key, msg string }{
		{annotation.RefEquipment, "equipamento %q não cadastrado"}, {annotation.RefCamera, "câmera %q não cadastrada"},
	} {
		v, _ := img.Fields[ref.key].(string)
		if v == "" {
			continue
		}
		exists := s.catalog.EquipmentExists
		if ref.key == annotation.RefCamera {
			exists = s.catalog.CameraExists
		}
		if ok, err := exists(ctx, v); err != nil || !ok {
			return "", "", fmt.Sprintf(ref.msg, v)
		}
	}

	if p.Name != annotation.DefaultProfile {
		return img.SHA1 + img.Ext, "", ""
	}
	// O Format valida os códigos e os formatos do v2.
	base, err := filename.Format(MetaFromImage(img))
	if err != nil {
		return "", "", err.Error()
	}
	return base + img.Ext, filename.CurrentVersion, ""
}

// ---------- ponte com o nome v2 (perfil veiculos) ----------

func str(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

// MetaFromImage converte a anotação (perfil veiculos) nos metadados do
// padrão v2. Usado ao verificar e ao renomear em cascata (equipamento/câmera),
// para o nome do arquivo ser montado sempre do mesmo jeito.
func MetaFromImage(img *Image) filename.ImageMeta {
	f := img.Fields
	meta := filename.ImageMeta{
		SHA1:         img.SHA1,
		Version:      filename.CurrentVersion,
		Equipment:    str(f, "equipment"),
		Camera:       str(f, "camera"),
		Direction:    str(f, "direction"),
		Resolution:   str(f, "resolution"),
		CaptureAngle: str(f, "capture_angle"),
		DayTime:      str(f, "day_time"),
		ImageRange:   str(f, "image_range"),
		Weather:      str(f, "weather"),
	}
	if c, ok := f["conditions"].([]string); ok {
		meta.Conditions = c
	}
	if t, ok := f["captured_at"].(time.Time); ok {
		meta.CapturedAt = t
	}
	for _, o := range img.Objects {
		if o.Type != "vehicle" {
			continue
		}
		meta.Vehicles = append(meta.Vehicles, filename.Vehicle{
			Plate: str(o.Fields, "plate"), PlateType: str(o.Fields, "plate_type"),
			PlateCategory: str(o.Fields, "plate_category"), Type: str(o.Fields, "type"), Color: str(o.Fields, "color"),
		})
	}
	return meta
}

// setNonEmpty põe o valor no mapa só se não for vazio.
func setNonEmpty(m map[string]any, key string, v any) {
	switch x := v.(type) {
	case string:
		if x == "" {
			return
		}
	case []string:
		if len(x) == 0 {
			return
		}
	case time.Time:
		if x.IsZero() {
			return
		}
	}
	m[key] = v
}

func vehicleObject(plate, plateType, plateCategory, typ, color string) annotation.Object {
	f := map[string]any{}
	setNonEmpty(f, "plate", plate)
	setNonEmpty(f, "plate_type", plateType)
	setNonEmpty(f, "plate_category", plateCategory)
	setNonEmpty(f, "type", typ)
	setNonEmpty(f, "color", color)
	return annotation.Object{Type: "vehicle", Fields: f}
}

func vehicleFields(equipment, camera, direction, resolution, angle, dayTime, imageRange, weather string, conditions []string, captured time.Time) map[string]any {
	f := map[string]any{}
	setNonEmpty(f, "equipment", equipment)
	setNonEmpty(f, "camera", camera)
	setNonEmpty(f, "direction", direction)
	setNonEmpty(f, "resolution", resolution)
	setNonEmpty(f, "capture_angle", angle)
	setNonEmpty(f, "day_time", dayTime)
	setNonEmpty(f, "image_range", imageRange)
	setNonEmpty(f, "weather", weather)
	setNonEmpty(f, "conditions", conditions)
	setNonEmpty(f, "captured_at", captured)
	return f
}

// ValuesFromMeta: o que o nome v2 diz, nos campos do perfil veiculos.
func ValuesFromMeta(m filename.ImageMeta) annotation.Values {
	v := annotation.Values{
		Fields: vehicleFields(m.Equipment, m.Camera, m.Direction, m.Resolution, m.CaptureAngle,
			m.DayTime, m.ImageRange, m.Weather, m.Conditions, m.CapturedAt),
		Objects: []annotation.Object{},
	}
	for _, x := range m.Vehicles {
		v.Objects = append(v.Objects, vehicleObject(x.Plate, x.PlateType, x.PlateCategory, x.Type, x.Color))
	}
	return v
}

// ValuesFromAnnotation: o que o import (CSV ou INI) descobriu, nos campos do
// perfil veiculos (os perfis de import ainda escrevem nesses campos).
func ValuesFromAnnotation(a importer.Annotation) annotation.Values {
	var captured time.Time
	if a.CapturedAt != nil {
		captured = a.CapturedAt.UTC()
	}
	v := annotation.Values{
		Fields: vehicleFields(a.Equipment, a.Camera, a.Direction, a.Resolution, a.CaptureAngle,
			a.DayTime, a.ImageRange, a.Weather, a.Conditions, captured),
		Objects: []annotation.Object{},
	}
	for _, x := range a.Vehicles {
		v.Objects = append(v.Objects, vehicleObject(x.Plate, x.PlateType, x.PlateCategory, x.Type, x.Color))
	}
	return v
}

// ---------- import e histórico ----------

// MergeAnnotation aplica no rascunho da imagem uma anotação que veio de fora
// (já encaixada no perfil com o Fit). overwrite=false: só preenche o que está
// vazio (respeita quem já anotou); os objetos só entram se a imagem não tem
// nenhum. Devolve true se algo mudou.
//
// Nunca altera o mapa que a imagem já tinha (cria outro): quem chamou pode
// estar guardando a imagem de antes para o histórico.
func MergeAnnotation(img *Image, v annotation.Values, overwrite bool) bool {
	fields := maps.Clone(img.Fields)
	if fields == nil {
		fields = map[string]any{}
	}
	changed := false
	for k, val := range v.Fields {
		cur, has := fields[k]
		if (overwrite || !has) && !reflect.DeepEqual(cur, val) {
			fields[k] = val
			changed = true
		}
	}
	img.Fields = fields
	if len(v.Objects) > 0 && (overwrite || len(img.Objects) == 0) && !reflect.DeepEqual(img.Objects, v.Objects) {
		img.Objects = append([]annotation.Object(nil), v.Objects...)
		changed = true
	}
	return changed
}

// DiffHistory gera uma entrada para cada campo que mudou (o nome do campo do
// perfil; os objetos vão juntos, como "objects").
func DiffHistory(before, after *Image, user string) []HistoryEntry {
	old, cur := trackedFields(before), trackedFields(after)
	keys := map[string]bool{}
	for k := range old {
		keys[k] = true
	}
	for k := range cur {
		keys[k] = true
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)

	now := time.Now().UTC()
	var entries []HistoryEntry
	for _, field := range sorted {
		if !reflect.DeepEqual(old[field], cur[field]) {
			entries = append(entries, HistoryEntry{
				ImageID: after.SHA1, Field: field, Old: old[field], New: cur[field],
				ChangedBy: user, ChangedAt: now,
			})
		}
	}
	return entries
}

func trackedFields(img *Image) map[string]any {
	out := map[string]any{"status": img.Status, "file_name": img.FileName}
	for k, v := range img.Fields {
		out[k] = v
	}
	var objects any
	if len(img.Objects) > 0 {
		objects = img.Objects
	}
	out["objects"] = objects
	return out
}
