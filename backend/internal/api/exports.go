package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"SIDM/internal/export"
	"SIDM/internal/store"
)

// Export de datasets de treino (RF-DETR / YOLO / Pascal VOC).
//
//	GET    /export-profiles            perfis padrão (rf-detr, yolo) + os salvos
//	POST   /export-profiles            cria um perfil
//	GET    /export-profiles/{name}     um perfil (padrão ou salvo)
//	PUT    /export-profiles/{name}     atualiza (os padrões não)
//	DELETE /export-profiles/{name}     apaga (os padrões não)
//	POST   /exports/preview            o que vai sair: imagens por grupo, caixas por classe
//	POST   /exports/download           o zip do dataset
//	GET    /export-folder-fields       metadados que podem virar pasta (layout por pastas)
//
// Entram as imagens VERIFICADAS e ANOTADAS NA TAREFA escolhida que passam
// nos filtros do perfil (as anotadas sem nenhuma caixa entram como negativas).
// A divisão train/valid/test é fixa por imagem.

const maxExportProfileBytes = 64 << 10

// decodeExportProfile lê e valida um perfil (corpo JSON).
func decodeExportProfile(w http.ResponseWriter, r *http.Request) (*export.Profile, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxExportProfileBytes)
	var p export.Profile
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "JSON inválido: "+err.Error())
		return nil, false
	}
	return validExportProfile(w, &p)
}

func validExportProfile(w http.ResponseWriter, p *export.Profile) (*export.Profile, bool) {
	p.Builtin = false // só o servidor marca perfil padrão
	p.Normalize()
	if err := p.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return nil, false
	}
	return p, true
}

// GET /export-profiles
func (s *Server) handleListExportProfiles(w http.ResponseWriter, r *http.Request) {
	saved, err := s.store.ListExportProfiles(r.Context())
	if err != nil {
		s.serverError(w, err)
		return
	}
	items := append(export.Builtins(), saved...)
	writeJSON(w, http.StatusOK, page[export.Profile]{Total: int64(len(items)), Items: items})
}

// GET /export-profiles/{name}
func (s *Server) handleGetExportProfile(w http.ResponseWriter, r *http.Request) {
	p, ok := s.exportProfileByName(w, r, r.PathValue("name"))
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) exportProfileByName(w http.ResponseWriter, r *http.Request, name string) (*export.Profile, bool) {
	if p, ok := export.Builtin(name); ok {
		return &p, true
	}
	p, err := s.store.FindExportProfile(r.Context(), name)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("perfil de export %q não existe", name))
		return nil, false
	}
	if err != nil {
		s.serverError(w, err)
		return nil, false
	}
	return p, true
}

// POST /export-profiles
func (s *Server) handleCreateExportProfile(w http.ResponseWriter, r *http.Request) {
	p, ok := decodeExportProfile(w, r)
	if !ok {
		return
	}
	p.CreatedBy, p.UpdatedAt = userFrom(r), time.Now().UTC()
	if err := s.store.CreateExportProfile(r.Context(), p); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			writeError(w, http.StatusConflict, fmt.Sprintf("já existe um perfil de export %q", p.Name))
			return
		}
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// PUT /export-profiles/{name}
func (s *Server) handleReplaceExportProfile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, builtin := export.Builtin(name); builtin {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("%q é um perfil padrão e não pode ser alterado: salve uma cópia com outro nome", name))
		return
	}
	p, ok := decodeExportProfile(w, r)
	if !ok {
		return
	}
	if p.Name != name {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("o nome no corpo (%q) é diferente do nome na URL (%q)", p.Name, name))
		return
	}
	old, err := s.store.FindExportProfile(r.Context(), name)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("perfil de export %q não existe", name))
		return
	}
	if err != nil {
		s.serverError(w, err)
		return
	}
	p.CreatedBy, p.UpdatedAt = old.CreatedBy, time.Now().UTC()
	if err := s.store.ReplaceExportProfile(r.Context(), p); err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// DELETE /export-profiles/{name}
func (s *Server) handleDeleteExportProfile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, builtin := export.Builtin(name); builtin {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("%q é um perfil padrão e não pode ser apagado", name))
		return
	}
	err := s.store.DeleteExportProfile(r.Context(), name)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("perfil de export %q não existe", name))
		return
	}
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "deleted": true})
}

// GET /export-folder-fields
// Os campos do modelo de nome dos arquivos (name_fields, legacy_template) e
// os metadados que podem virar nível de pasta (layout por pastas), com os
// valores possíveis e o nome padrão da pasta de cada um (null = campo aberto,
// como equipamento e câmera: a pasta é o próprio valor).
func (s *Server) handleExportFolderFields(w http.ResponseWriter, r *http.Request) {
	type field struct {
		Field  string            `json:"field"`
		Values map[string]string `json:"values"`
	}
	out := make([]field, 0, len(export.FolderFields))
	for _, f := range export.FolderFields {
		out = append(out, field{Field: f, Values: export.FolderValues(f)})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": out, "unknown": export.DefaultUnknown,
		// campos do modelo de nome dos arquivos de saída, e o padrão antigo
		"name_fields": export.NameFields(), "legacy_template": export.TemplateLegacy,
	})
}

// exportRequest: {"profile": "rf-detr", "task": "capacete"} (um perfil
// salvo/padrão) ou {"profile": {...}, "task": ...} (um perfil montado na
// tela, sem salvar). "task" vale mais que a tarefa do perfil; se nenhum dos
// dois tem tarefa, é erro.
type exportRequest struct {
	Profile json.RawMessage `json:"profile"`
	Task    string          `json:"task"`
}

// profileFromRequest resolve o perfil do pedido. O download também aceita o
// pedido num campo de formulário "request" (assim o navegador baixa o zip
// direto para o disco, sem segurar tudo na memória).
func (s *Server) profileFromRequest(w http.ResponseWriter, r *http.Request) (*export.Profile, bool) {
	p, task, ok := s.parseExportRequest(w, r)
	if !ok {
		return nil, false
	}
	if task = strings.TrimSpace(task); task != "" {
		p.Task = task
	}
	if p.Task == "" {
		writeError(w, http.StatusBadRequest, "escolha a tarefa de anotação (as caixas de qual tarefa entram no dataset)")
		return nil, false
	}
	if _, err := s.store.FindTask(r.Context(), p.Task); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("a tarefa %q não existe (foi renomeada ou apagada?)", p.Task))
		return nil, false
	} else if err != nil {
		s.serverError(w, err)
		return nil, false
	}
	return p, true
}

func (s *Server) parseExportRequest(w http.ResponseWriter, r *http.Request) (*export.Profile, string, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxExportProfileBytes)
	var raw []byte
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		if err := r.ParseForm(); err != nil {
			writeError(w, http.StatusBadRequest, "formulário inválido: "+err.Error())
			return nil, "", false
		}
		raw = []byte(r.PostForm.Get("request"))
	} else {
		var err error
		if raw, err = io.ReadAll(r.Body); err != nil {
			writeError(w, http.StatusBadRequest, "erro lendo o pedido: "+err.Error())
			return nil, "", false
		}
	}
	var req exportRequest
	if err := json.Unmarshal(raw, &req); err != nil || len(req.Profile) == 0 {
		writeError(w, http.StatusBadRequest, `esperado {"profile": "nome", "task": "tarefa"} ou {"profile": {...}, "task": "tarefa"}`)
		return nil, "", false
	}
	var name string
	if json.Unmarshal(req.Profile, &name) == nil {
		p, ok := s.exportProfileByName(w, r, name)
		return p, req.Task, ok
	}
	var p export.Profile
	dec := json.NewDecoder(bytes.NewReader(req.Profile))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "perfil inválido: "+err.Error())
		return nil, "", false
	}
	// Perfil montado na tela e ainda não salvo: o nome só vai no nome do zip.
	// Um padrão sem mudanças (só a tarefa escolhida) é o próprio padrão.
	if b, builtin := export.Builtin(p.Name); builtin && sameProfile(p, b) {
		b.Task = p.Task
		return &b, req.Task, true
	} else if builtin || p.Name == "" {
		p.Name = "personalizado"
	}
	vp, ok := validExportProfile(w, &p)
	return vp, req.Task, ok
}

// sameProfile: os dois perfis geram o mesmo dataset (a tarefa não conta: os
// padrões não têm tarefa).
func sameProfile(a, b export.Profile) bool {
	for _, p := range []*export.Profile{&a, &b} {
		p.Builtin, p.Task, p.CreatedBy, p.UpdatedAt = false, "", "", time.Time{}
		p.Normalize()
	}
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return bytes.Equal(ja, jb)
}

// exportPreview é o resumo do que vai sair.
type exportPreview struct {
	Profile   string         `json:"profile"`
	Task      string         `json:"task"`
	Format    string         `json:"format"`
	Layout    string         `json:"layout"`
	Split     export.Split   `json:"split"`
	Classes   []string       `json:"classes"`
	Total     int            `json:"total"`     // imagens no dataset
	Negatives int            `json:"negatives"` // dessas, as sem nenhuma caixa (anotadas como vazias)
	Images    map[string]int `json:"images"`    // por grupo
	Folders   map[string]int `json:"folders"`   // por pasta (layout por pastas; vazio no por divisão)
	Examples  []string       `json:"examples"`  // os primeiros nomes de arquivo, como vão sair
	Boxes     map[string]int `json:"boxes"`     // por classe
	Skipped   exportSkipped  `json:"skipped"`
	Packages  []string       `json:"packages"` // os que entraram (vazio no perfil = todos)
}

type exportSkipped struct {
	NotAnnotated  int `json:"not_annotated"`  // verificadas, mas não anotadas na tarefa
	WithoutLabels int `json:"without_labels"` // com caixas, mas nenhuma das labels escolhidas
}

// collectExport passa pelas imagens do perfil e monta os itens do dataset.
func (s *Server) collectExport(ctx context.Context, p *export.Profile) ([]export.Item, *exportPreview, error) {
	prev := &exportPreview{
		Profile: p.Name, Task: p.Task, Format: p.Format, Layout: p.Layout, Split: p.Split,
		Images: map[string]int{}, Folders: map[string]int{}, Examples: []string{}, Boxes: map[string]int{}, Packages: p.Packages,
	}
	if prev.Packages == nil {
		prev.Packages = []string{}
	}
	labels := export.NewLabels(p.Labels)
	seen := map[string]int{}
	var items []export.Item

	pkgs := p.Packages
	if len(pkgs) == 0 {
		pkgs = []string{""} // todos
	}
	f := p.Filters
	const pageSize = 500
	for _, pkg := range pkgs {
		for skip := int64(0); ; skip += pageSize {
			imgs, _, err := s.store.ListImages(ctx, store.ImageFilter{
				Package: pkg, Status: store.StatusVerified,
				Equipment: f.Equipment, Camera: f.Camera, Direction: f.Direction, DayTime: f.DayTime,
				Weather: f.Weather, VehicleType: f.VehicleType, VehicleColor: f.VehicleColor,
				Limit: pageSize, Skip: skip,
			})
			if err != nil {
				return nil, nil, err
			}
			ids := make([]string, len(imgs))
			for i, img := range imgs {
				ids[i] = img.SHA1
			}
			sets, err := s.store.FindBoxSets(ctx, p.Task, ids)
			if err != nil {
				return nil, nil, err
			}
			for _, img := range imgs {
				set, ok := sets[img.SHA1]
				if !ok {
					prev.Skipped.NotAnnotated++
					continue
				}
				stored := path.Base(img.Path)
				meta := imageMeta(&img)
				item := export.Item{
					SHA1: img.SHA1, Width: set.Width, Height: set.Height,
					FileName: p.FileNameOf(export.NameMeta(img.SHA1, stored, img.OriginalName, meta), path.Ext(stored)),
					Split:    export.SplitOf(img.SHA1, p.Split),
					Open:     s.opener(img.Path),
				}
				if p.Layout == export.LayoutFolders {
					item.Dir = p.Dir(meta, item.Split)
				}
				for _, b := range set.Boxes {
					if name, ok := labels.Map(b.Label); ok {
						b.Label = name
						item.Boxes = append(item.Boxes, b)
						seen[name]++
					}
				}
				switch {
				case len(set.Boxes) == 0: // negativa: entra sem caixas
					prev.Negatives++
				case len(item.Boxes) == 0: // tinha caixas, mas de labels que não entram
					prev.Skipped.WithoutLabels++
					continue
				}
				items = append(items, item)
				if len(prev.Examples) < 3 {
					prev.Examples = append(prev.Examples, path.Join(item.Dir, item.FileName))
				}
				prev.Images[item.Split]++
				if item.Dir != "" {
					prev.Folders[item.Dir]++
				}
				for _, b := range item.Boxes {
					prev.Boxes[b.Label]++
				}
			}
			if len(imgs) < pageSize {
				break
			}
		}
	}
	prev.Total = len(items)
	prev.Classes = labels.Classes(seen)
	return items, prev, nil
}

// imageMeta: os metadados da imagem que viram pasta ou parte do nome de
// saída (os do veículo vêm do primeiro veículo; a condição é a primeira).
func imageMeta(img *store.Image) export.Meta {
	m := export.Meta{
		"resolution": img.Resolution, "direction": img.Direction, "day_time": img.DayTime,
		"weather": img.Weather, "capture_angle": img.CaptureAngle, "image_range": img.ImageRange,
		"equipment": img.Equipment, "camera": img.Camera, "package": img.Package,
	}
	if len(img.Vehicles) > 0 {
		v := img.Vehicles[0]
		m["vehicle_type"], m["plate_type"], m["plate_category"], m["vehicle_color"] = v.Type, v.PlateType, v.PlateCategory, v.Color
		m["plate"] = v.Plate
	}
	if len(img.Conditions) > 0 {
		m["condition"] = img.Conditions[0]
	}
	if img.CapturedAt != nil {
		t := img.CapturedAt.UTC()
		m["captured_at"], m["date"] = t.Format("20060102T150405Z"), t.Format("20060102")
	}
	return m
}

func (s *Server) opener(rel string) func() (io.ReadCloser, error) {
	return func() (io.ReadCloser, error) { return os.Open(s.files.Abs(rel)) }
}

// checkPackages: um pacote do perfil que não existe é erro (provavelmente
// foi renomeado ou apagado).
func (s *Server) checkPackages(w http.ResponseWriter, r *http.Request, p *export.Profile) bool {
	for _, name := range p.Packages {
		if _, err := s.store.FindPackage(r.Context(), name); errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("o pacote %q do perfil não existe (foi renomeado ou apagado?)", name))
			return false
		} else if err != nil {
			s.serverError(w, err)
			return false
		}
	}
	return true
}

// POST /exports/preview
func (s *Server) handleExportPreview(w http.ResponseWriter, r *http.Request) {
	p, ok := s.profileFromRequest(w, r)
	if !ok || !s.checkPackages(w, r, p) {
		return
	}
	_, prev, err := s.collectExport(r.Context(), p)
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, prev)
}

// POST /exports/download
// O zip é gerado enquanto é baixado (não ocupa disco no servidor).
func (s *Server) handleExportDownload(w http.ResponseWriter, r *http.Request) {
	p, ok := s.profileFromRequest(w, r)
	if !ok || !s.checkPackages(w, r, p) {
		return
	}
	items, prev, err := s.collectExport(r.Context(), p)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if len(items) == 0 {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("nenhuma imagem verificada e anotada na tarefa %q passa nos filtros deste perfil", p.Task))
		return
	}

	now := time.Now().UTC()
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s.zip"`, p.Name, now.Format("20060102-1504")))
	zw := zip.NewWriter(w)
	info := map[string]any{
		"generated_by": "SIDM", "generated_at": now, "user": userFrom(r),
		"profile": p, "preview": prev,
	}
	if _, err := export.Write(zw, *p, items, prev.Classes, info); err != nil {
		s.logError(fmt.Errorf("export %s: %w", p.Name, err))
	}
	if err := zw.Close(); err != nil {
		s.logError(fmt.Errorf("export %s: %w", p.Name, err))
	}
}
