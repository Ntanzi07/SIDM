package exports

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"SIDM/internal/export"
	"SIDM/internal/platform"
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
func decodeProfile(w http.ResponseWriter, r *http.Request) (*export.Profile, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxExportProfileBytes)
	var p export.Profile
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		platform.WriteError(w, http.StatusBadRequest, "JSON inválido: "+err.Error())
		return nil, false
	}
	if err := Validate(&p); err != nil {
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
	p, ok := decodeProfile(w, r)
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
	p, ok := decodeProfile(w, r)
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

// GET /export-folder-fields
// Os campos do modelo de nome dos arquivos (name_fields, legacy_template) e
// os metadados que podem virar nível de pasta (layout por pastas), com os
// valores possíveis e o nome padrão da pasta de cada um (null = campo aberto,
// como equipamento e câmera: a pasta é o próprio valor).
func (s *Service) handleFolderFields(w http.ResponseWriter, r *http.Request) {
	type field struct {
		Field  string            `json:"field"`
		Values map[string]string `json:"values"`
	}
	out := make([]field, 0, len(export.FolderFields))
	for _, f := range export.FolderFields {
		out = append(out, field{Field: f, Values: export.FolderValues(f)})
	}
	platform.WriteJSON(w, http.StatusOK, map[string]any{
		"items": out, "unknown": export.DefaultUnknown,
		// campos do modelo de nome dos arquivos de saída, e o padrão antigo
		"name_fields": export.NameFields(), "legacy_template": export.TemplateLegacy,
	})
}

// ---------- dataset ----------

// request: {"profile": "rf-detr", "task": "capacete"} (um perfil
// salvo/padrão) ou {"profile": {...}, "task": ...} (um perfil montado na
// tela, sem salvar). "task" vale mais que a tarefa do perfil; se nenhum dos
// dois tem tarefa, é erro.
type request struct {
	Profile json.RawMessage `json:"profile"`
	Task    string          `json:"task"`
}

// profileFromRequest resolve e confere o perfil do pedido. O download também
// aceita o pedido num campo de formulário "request" (assim o navegador baixa
// o zip direto para o disco, sem segurar tudo na memória).
func (s *Service) profileFromRequest(w http.ResponseWriter, r *http.Request) (*export.Profile, bool) {
	p, task, ok := s.parseRequest(w, r)
	if !ok {
		return nil, false
	}
	if err := s.Prepare(r.Context(), p, task); err != nil {
		writeErr(w, err, "")
		return nil, false
	}
	return p, true
}

func (s *Service) parseRequest(w http.ResponseWriter, r *http.Request) (*export.Profile, string, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxExportProfileBytes)
	var raw []byte
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		if err := r.ParseForm(); err != nil {
			platform.WriteError(w, http.StatusBadRequest, "formulário inválido: "+err.Error())
			return nil, "", false
		}
		raw = []byte(r.PostForm.Get("request"))
	} else {
		var err error
		if raw, err = io.ReadAll(r.Body); err != nil {
			platform.WriteError(w, http.StatusBadRequest, "erro lendo o pedido: "+err.Error())
			return nil, "", false
		}
	}
	var req request
	if err := json.Unmarshal(raw, &req); err != nil || len(req.Profile) == 0 {
		platform.WriteError(w, http.StatusBadRequest, `esperado {"profile": "nome", "task": "tarefa"} ou {"profile": {...}, "task": "tarefa"}`)
		return nil, "", false
	}
	var name string
	if json.Unmarshal(req.Profile, &name) == nil {
		p, err := s.FindProfile(r.Context(), name)
		if err != nil {
			writeErr(w, err, notFoundMsg(name))
			return nil, "", false
		}
		return p, req.Task, true
	}
	var p export.Profile
	dec := json.NewDecoder(bytes.NewReader(req.Profile))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		platform.WriteError(w, http.StatusBadRequest, "perfil inválido: "+err.Error())
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
	if err := Validate(&p); err != nil {
		writeErr(w, err, "")
		return nil, "", false
	}
	return &p, req.Task, true
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

// POST /exports/preview  (o que vai sair: imagens por grupo, caixas por classe)
func (s *Service) handlePreview(w http.ResponseWriter, r *http.Request) {
	p, ok := s.profileFromRequest(w, r)
	if !ok {
		return
	}
	_, prev, err := s.Collect(r.Context(), p)
	if err != nil {
		platform.ServerError(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, prev)
}

// POST /exports/download
// O zip é gerado enquanto é baixado (não ocupa disco no servidor).
func (s *Service) handleDownload(w http.ResponseWriter, r *http.Request) {
	p, ok := s.profileFromRequest(w, r)
	if !ok {
		return
	}
	items, prev, err := s.Collect(r.Context(), p)
	if err != nil {
		platform.ServerError(w, err)
		return
	}
	if len(items) == 0 {
		platform.WriteError(w, http.StatusBadRequest, fmt.Sprintf("nenhuma imagem verificada e anotada na tarefa %q passa nos filtros deste perfil", p.Task))
		return
	}

	now := s.now()
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s.zip"`, p.Name, now.Format("20060102-1504")))
	zw := zip.NewWriter(w)
	info := map[string]any{
		"generated_by": "SIDM", "generated_at": now, "user": platform.UserFrom(r),
		"profile": p, "preview": prev,
	}
	if _, err := export.Write(zw, *p, items, prev.Classes, info); err != nil {
		platform.LogError(fmt.Errorf("export %s: %w", p.Name, err))
	}
	if err := zw.Close(); err != nil {
		platform.LogError(fmt.Errorf("export %s: %w", p.Name, err))
	}
}
