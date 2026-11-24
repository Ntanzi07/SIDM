package exports

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"SIDM/internal/annotation"
	"SIDM/internal/export"
	"SIDM/internal/platform"
	"SIDM/internal/versions"
)

const maxExportProfileBytes = 64 << 10

// Routes são as rotas do módulo.
func (s *Service) Routes() []platform.Route {
	return []platform.Route{
		{Method: "GET", Path: "/export-folder-fields", Handler: s.handleFolderFields},
		{Method: "GET", Path: "/export-profiles", Handler: s.handleListProfiles},
		{Method: "POST", Path: "/export-profiles", Handler: s.handleCreateProfile},
		{Method: "GET", Path: "/export-profiles/{name}", Handler: s.handleGetProfile},
		{Method: "PUT", Path: "/export-profiles/{name}", Handler: s.handleReplaceProfile},
		{Method: "DELETE", Path: "/export-profiles/{name}", Handler: s.handleDeleteProfile},
		{Method: "POST", Path: "/exports/preview", Handler: s.handlePreview},
		{Method: "POST", Path: "/exports/download", Handler: s.handleDownload},
	}
}

// writeErr traduz os erros do Service: InvalidError vira 400, ErrNotFound
// vira 404 (com a mensagem dada), o resto 500.
func writeErr(w http.ResponseWriter, err error, notFound string) {
	var invalid *InvalidError
	switch {
	case errors.As(err, &invalid):
		platform.WriteError(w, http.StatusBadRequest, invalid.Msg)
	case errors.Is(err, platform.ErrNotFound) && notFound != "":
		platform.WriteError(w, http.StatusNotFound, notFound)
	default:
		platform.ServerError(w, err)
	}
}

// ---------- perfis ----------

// decodeProfile lê e valida um perfil (corpo JSON).
func (s *Service) decodeProfile(w http.ResponseWriter, r *http.Request) (*export.Profile, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxExportProfileBytes)
	var p export.Profile
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		platform.WriteError(w, http.StatusBadRequest, "JSON inválido: "+err.Error())
		return nil, false
	}
	if err := s.Validate(r.Context(), &p); err != nil {
		writeErr(w, err, "")
		return nil, false
	}
	return &p, true
}

func notFoundMsg(name string) string { return fmt.Sprintf("perfil de export %q não existe", name) }

// GET /export-profiles  (perfis padrão + os salvos)
func (s *Service) handleListProfiles(w http.ResponseWriter, r *http.Request) {
	items, err := s.ListProfiles(r.Context())
	if err != nil {
		platform.ServerError(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, platform.Page[export.Profile]{Total: int64(len(items)), Items: items})
}

// GET /export-profiles/{name}  (padrão ou salvo)
func (s *Service) handleGetProfile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	p, err := s.FindProfile(r.Context(), name)
	if err != nil {
		writeErr(w, err, notFoundMsg(name))
		return
	}
	platform.WriteJSON(w, http.StatusOK, p)
}

// POST /export-profiles
func (s *Service) handleCreateProfile(w http.ResponseWriter, r *http.Request) {
	p, ok := s.decodeProfile(w, r)
	if !ok {
		return
	}
	if err := s.CreateProfile(r.Context(), p, platform.UserFrom(r)); err != nil {
		if errors.Is(err, platform.ErrDuplicate) {
			platform.WriteError(w, http.StatusConflict, fmt.Sprintf("já existe um perfil de export %q", p.Name))
			return
		}
		platform.ServerError(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusCreated, p)
}

// PUT /export-profiles/{name}  (os padrões não)
func (s *Service) handleReplaceProfile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := Editable(name, "alterado"); err != nil {
		writeErr(w, err, "")
		return
	}
	p, ok := s.decodeProfile(w, r)
	if !ok {
		return
	}
	if p.Name != name {
		platform.WriteError(w, http.StatusBadRequest, fmt.Sprintf("o nome no corpo (%q) é diferente do nome na URL (%q)", p.Name, name))
		return
	}
	if err := s.ReplaceProfile(r.Context(), p); err != nil {
		writeErr(w, err, notFoundMsg(name))
		return
	}
	platform.WriteJSON(w, http.StatusOK, p)
}

// DELETE /export-profiles/{name}  (os padrões não)
func (s *Service) handleDeleteProfile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.DeleteProfile(r.Context(), name); err != nil {
		writeErr(w, err, notFoundMsg(name))
		return
	}
	platform.WriteJSON(w, http.StatusOK, map[string]any{"name": name, "deleted": true})
}

// GET /export-folder-fields?annotation_profile=veiculos
// Os metadados que um perfil de anotação (vazio = o padrão) dá às imagens,
// para as pastas (layout por pastas) e o modelo de nome dos arquivos:
//
//	fields: todos, com o nome na tela, o grupo (file, image ou o tipo do
//	        objeto), se servem de pasta e de nome, os valores possíveis com o
//	        nome padrão da pasta de cada um (null = campo aberto, como
//	        equipamento: a pasta é o próprio valor) e os nomes na tela dos
//	        valores. alias = nome antigo/atalho (aceito, mas a tela não oferece).
//	items: os que servem de pasta (field, values), como antes;
//	name_fields: as chaves que servem de nome;
//	legacy_template: o padrão antigo (só no perfil veiculos).
func (s *Service) handleFolderFields(w http.ResponseWriter, r *http.Request) {
	ap, err := s.AnnotationProfile(r.Context(), strings.TrimSpace(r.URL.Query().Get("annotation_profile")))
	if err != nil {
		writeErr(w, err, "")
		return
	}
	type item struct {
		Field  string            `json:"field"`
		Values map[string]string `json:"values"`
	}
	fields := export.MetaFields(ap)
	items, names := []item{}, []string{}
	for _, f := range fields {
		if f.Alias {
			continue
		}
		if f.Folder {
			items = append(items, item{Field: f.Key, Values: f.Values})
		}
		if f.Name {
			names = append(names, f.Key)
		}
	}
	legacy := ""
	if ap.Is(annotation.DefaultProfile) {
		legacy = export.TemplateLegacy
	}
	platform.WriteJSON(w, http.StatusOK, map[string]any{
		"annotation_profile": ap.Name, "fields": fields,
		"items": items, "unknown": export.DefaultUnknown,
		"name_fields": names, "legacy_template": legacy,
	})
}

// ---------- dataset ----------

// request: {"profile": "rf-detr", "tasks": ["parabrisa", "placa"]} (um
// perfil salvo/padrão) ou {"profile": {...}, "tasks": [...]} (um perfil
// montado na tela, sem salvar). As tarefas do pedido valem mais que as do
// perfil; se nenhum dos dois tem tarefa, é erro. "task" (uma só) é o formato
// de antes e ainda vale.
//
// Com "version": {"dataset": "motos", "number": 3}, as imagens e a anotação
// são as congeladas na versão (0 = o estado atual do dataset).
type request struct {
	Profile json.RawMessage `json:"profile"`
	Tasks   []string        `json:"tasks"`
	Task    string          `json:"task"`
	Version *VersionRef     `json:"version"`
}

// tasks: as tarefas do pedido (vazio = as do perfil).
func (r request) tasks() []string {
	if len(r.Tasks) > 0 {
		return r.Tasks
	}
	if strings.TrimSpace(r.Task) != "" {
		return []string{r.Task}
	}
	return nil
}

// profileFromRequest resolve e confere o perfil do pedido. O download também
// aceita o pedido num campo de formulário "request" (assim o navegador baixa
// o zip direto para o disco, sem segurar tudo na memória).
// Com versão, volta também a versão congelada.
func (s *Service) profileFromRequest(w http.ResponseWriter, r *http.Request) (*export.Profile, *versions.Frozen, bool) {
	p, tasks, ref, ok := s.parseRequest(w, r)
	if !ok {
		return nil, nil, false
	}
	if ref != nil {
		fz, err := s.FindVersion(r.Context(), *ref)
		if err != nil {
			writeVersionErr(w, err, *ref)
			return nil, nil, false
		}
		if err := s.PrepareVersion(r.Context(), p, tasks, fz); err != nil {
			writeErr(w, err, "")
			return nil, nil, false
		}
		return p, fz, true
	}
	if err := s.Prepare(r.Context(), p, tasks); err != nil {
		writeErr(w, err, "")
		return nil, nil, false
	}
	return p, nil, true
}

// writeVersionErr: a versão que não existe vira 404; um pedido que o
// versions recusa, 400.
func writeVersionErr(w http.ResponseWriter, err error, ref VersionRef) {
	if errors.Is(err, platform.ErrInvalid) {
		platform.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	msg := fmt.Sprintf("a versão %d do dataset %q não existe", ref.Number, ref.Dataset)
	if ref.Number == 0 {
		msg = fmt.Sprintf("o dataset %q não existe", ref.Dataset)
	}
	writeErr(w, err, msg)
}

// collect: as imagens de hoje ou as da versão.
func (s *Service) collect(ctx context.Context, p *export.Profile, fz *versions.Frozen) ([]export.Item, *Preview, error) {
	if fz != nil {
		items, prev := s.CollectVersion(p, fz)
		return items, prev, nil
	}
	return s.Collect(ctx, p)
}

func (s *Service) parseRequest(w http.ResponseWriter, r *http.Request) (*export.Profile, []string, *VersionRef, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxExportProfileBytes)
	var raw []byte
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		if err := r.ParseForm(); err != nil {
			platform.WriteError(w, http.StatusBadRequest, "formulário inválido: "+err.Error())
			return nil, nil, nil, false
		}
		raw = []byte(r.PostForm.Get("request"))
	} else {
		var err error
		if raw, err = io.ReadAll(r.Body); err != nil {
			platform.WriteError(w, http.StatusBadRequest, "erro lendo o pedido: "+err.Error())
			return nil, nil, nil, false
		}
	}
	var req request
	if err := json.Unmarshal(raw, &req); err != nil || len(req.Profile) == 0 {
		platform.WriteError(w, http.StatusBadRequest, `esperado {"profile": "nome", "task": "tarefa"} ou {"profile": {...}, "task": "tarefa"}`)
		return nil, nil, nil, false
	}
	var name string
	if json.Unmarshal(req.Profile, &name) == nil {
		p, err := s.FindProfile(r.Context(), name)
		if err != nil {
			writeErr(w, err, notFoundMsg(name))
			return nil, nil, nil, false
		}
		return p, req.tasks(), req.Version, true
	}
	var p export.Profile
	dec := json.NewDecoder(bytes.NewReader(req.Profile))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		platform.WriteError(w, http.StatusBadRequest, "perfil inválido: "+err.Error())
		return nil, nil, nil, false
	}
	p.Normalize() // "task" (uma tarefa, o formato antigo) vira "tasks"
	// Perfil montado na tela e ainda não salvo: o nome só vai no nome do zip.
	// Um padrão sem mudanças (só a tarefa escolhida) é o próprio padrão.
	if b, builtin := export.Builtin(p.Name); builtin && sameProfile(p, b) {
		b.Tasks, b.AllTasks = p.Tasks, p.AllTasks
		return &b, req.tasks(), req.Version, true
	} else if builtin || p.Name == "" {
		p.Name = "personalizado"
	}
	if err := s.Validate(r.Context(), &p); err != nil {
		writeErr(w, err, "")
		return nil, nil, nil, false
	}
	return &p, req.tasks(), req.Version, true
}

// sameProfile: os dois perfis geram o mesmo dataset (as tarefas não contam:
// os padrões não têm tarefa).
func sameProfile(a, b export.Profile) bool {
	for _, p := range []*export.Profile{&a, &b} {
		p.Normalize()
		p.Builtin, p.Tasks, p.AllTasks, p.CreatedBy, p.UpdatedAt = false, nil, false, "", time.Time{}
	}
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return bytes.Equal(ja, jb)
}

// POST /exports/preview  (o que vai sair: imagens por grupo, caixas por classe)
func (s *Service) handlePreview(w http.ResponseWriter, r *http.Request) {
	p, fz, ok := s.profileFromRequest(w, r)
	if !ok {
		return
	}
	_, prev, err := s.collect(r.Context(), p, fz)
	if err != nil {
		platform.ServerError(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, prev)
}

// POST /exports/download
// O zip é gerado enquanto é baixado (não ocupa disco no servidor).
func (s *Service) handleDownload(w http.ResponseWriter, r *http.Request) {
	p, fz, ok := s.profileFromRequest(w, r)
	if !ok {
		return
	}
	items, prev, err := s.collect(r.Context(), p, fz)
	if err != nil {
		platform.ServerError(w, err)
		return
	}
	if len(items) == 0 {
		msg := "nenhuma imagem verificada passa nos filtros deste perfil"
		switch {
		case fz != nil && p.NeedsTasks():
			msg = fmt.Sprintf("nenhuma imagem da %s está anotada em %s", versionText(fz.Version), tasksText(p))
		case fz != nil:
			msg = fmt.Sprintf("a %s não tem imagens", versionText(fz.Version))
		case p.NeedsTasks():
			msg = fmt.Sprintf("nenhuma imagem verificada e anotada em %s passa nos filtros deste perfil", tasksText(p))
		}
		platform.WriteError(w, http.StatusBadRequest, msg)
		return
	}

	now := s.now()
	// O nome do zip: o perfil; de uma versão, <dataset>-v<n>-<perfil>.
	zipName := fmt.Sprintf("%s-%s", p.Name, now.Format("20060102-1504"))
	if fz != nil {
		zipName = fmt.Sprintf("%s-%s-%s", fz.Version.Dataset, versionTag(fz.Version.Number, now), p.Name)
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, zipName))
	zw := zip.NewWriter(w)
	info := map[string]any{
		"generated_by": "SIDM", "generated_at": now, "user": platform.UserFrom(r),
		"profile": p, "preview": prev,
	}
	if fz != nil {
		info["version"] = prev.Version // de qual versão saiu (dataset, número, id)
	}
	if _, err := export.Write(zw, *p, items, prev.Classes, info); err != nil {
		platform.LogError(fmt.Errorf("export %s: %w", p.Name, err))
	}
	if err := zw.Close(); err != nil {
		platform.LogError(fmt.Errorf("export %s: %w", p.Name, err))
	}
}

// tasksText: "na tarefa \"placa\"" ou "nas tarefas \"a\" e \"b\"" (todas, com all_tasks).
func tasksText(p *export.Profile) string {
	if len(p.Tasks) == 1 {
		return fmt.Sprintf("tarefa %q", p.Tasks[0])
	}
	quoted := make([]string, len(p.Tasks))
	for i, t := range p.Tasks {
		quoted[i] = fmt.Sprintf("%q", t)
	}
	if p.AllTasks {
		return "todas as tarefas " + strings.Join(quoted, ", ")
	}
	return "alguma das tarefas " + strings.Join(quoted, ", ")
}

// versionTag: "v3" (o estado atual, que não é versão: "atual-<data>").
func versionTag(number int, now time.Time) string {
	if number == 0 {
		return "atual-" + now.Format("20060102-1504")
	}
	return fmt.Sprintf("v%d", number)
}
