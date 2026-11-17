package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"SIDM/internal/boxes"
	"SIDM/internal/store"
)

// Tarefas de anotação de caixas (ex.: "capacete", "placa-letras").
//
//	GET    /tasks                  todas, com o resumo de cada uma
//	POST   /tasks                  cria {name, description, labels}
//	GET    /tasks/{name}           a tarefa e o resumo (pacotes, caixas por label)
//	PUT    /tasks/{name}           atualiza; mudar o nome leva junto caixas e perfis de export
//	DELETE /tasks/{name}           apaga a tarefa e as caixas dela
//	POST   /tasks/{name}/import    importa o COCO 1.0 do CVAT (ensaio por padrão)
//
// Os pacotes de uma tarefa não são escolhidos: são os das imagens anotadas
// nela. O import procura as imagens no banco todo.

var taskNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

const (
	maxTaskLabels   = 500
	maxTaskLabelLen = 64
	maxTaskBytes    = 64 << 10
)

// taskBody é o que o cliente manda para criar/atualizar.
type taskBody struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Labels      []string `json:"labels"`
}

// taskView é a tarefa com o resumo.
type taskView struct {
	store.Task
	Stats *store.TaskStats `json:"stats"`
}

func decodeTask(w http.ResponseWriter, r *http.Request) (*taskBody, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxTaskBytes)
	var b taskBody
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "JSON inválido: "+err.Error())
		return nil, false
	}
	b.Name = strings.TrimSpace(b.Name)
	b.Description = strings.TrimSpace(b.Description)
	if !taskNameRe.MatchString(b.Name) {
		writeError(w, http.StatusBadRequest, "nome da tarefa: de 1 a 64 caracteres, só letras minúsculas, números, - e _ (começando por letra ou número)")
		return nil, false
	}
	labels, err := normalizeTaskLabels(b.Labels)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return nil, false
	}
	b.Labels = labels
	return &b, true
}

// normalizeTaskLabels: sem espaços nas pontas, sem vazias, sem repetidas
// (a ordem é mantida: é a ordem em que aparecem na tela).
func normalizeTaskLabels(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, l := range in {
		l = strings.TrimSpace(l)
		if l == "" || seen[l] {
			continue
		}
		if len([]rune(l)) > maxTaskLabelLen {
			return nil, fmt.Errorf("label %q passa de %d caracteres", l, maxTaskLabelLen)
		}
		seen[l] = true
		out = append(out, l)
	}
	if len(out) > maxTaskLabels {
		return nil, fmt.Errorf("no máximo %d labels por tarefa", maxTaskLabels)
	}
	return out, nil
}

func (s *Server) taskByName(w http.ResponseWriter, r *http.Request, name string) (*store.Task, bool) {
	t, err := s.store.FindTask(r.Context(), name)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("tarefa %q não existe", name))
		return nil, false
	}
	if err != nil {
		s.serverError(w, err)
		return nil, false
	}
	return t, true
}

// GET /tasks
func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.store.ListTasks(r.Context())
	if err != nil {
		s.serverError(w, err)
		return
	}
	items := make([]taskView, 0, len(tasks))
	for _, t := range tasks {
		st, err := s.store.TaskStats(r.Context(), t.Name)
		if err != nil {
			s.serverError(w, err)
			return
		}
		items = append(items, taskView{Task: t, Stats: st})
	}
	writeJSON(w, http.StatusOK, page[taskView]{Total: int64(len(items)), Items: items})
}

// GET /tasks/{name}
func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	t, ok := s.taskByName(w, r, r.PathValue("name"))
	if !ok {
		return
	}
	st, err := s.store.TaskStats(r.Context(), t.Name)
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, taskView{Task: *t, Stats: st})
}

// POST /tasks
func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	b, ok := decodeTask(w, r)
	if !ok {
		return
	}
	now := time.Now().UTC()
	t := &store.Task{
		Name: b.Name, Description: b.Description, Labels: b.Labels,
		CreatedBy: userFrom(r), CreatedAt: now, UpdatedAt: now,
	}
	if err := s.store.CreateTask(r.Context(), t); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			writeError(w, http.StatusConflict, fmt.Sprintf("já existe uma tarefa %q", t.Name))
			return
		}
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

// PUT /tasks/{name}
func (s *Server) handleReplaceTask(w http.ResponseWriter, r *http.Request) {
	old, ok := s.taskByName(w, r, r.PathValue("name"))
	if !ok {
		return
	}
	b, ok := decodeTask(w, r)
	if !ok {
		return
	}
	t := &store.Task{
		Name: b.Name, Description: b.Description, Labels: b.Labels,
		CreatedBy: old.CreatedBy, CreatedAt: old.CreatedAt, UpdatedAt: time.Now().UTC(),
	}
	if err := s.store.UpdateTask(r.Context(), old.Name, t); err != nil {
		switch {
		case errors.Is(err, store.ErrDuplicate):
			writeError(w, http.StatusConflict, fmt.Sprintf("já existe uma tarefa %q", t.Name))
		case errors.Is(err, store.ErrNotFound):
			writeError(w, http.StatusNotFound, fmt.Sprintf("tarefa %q não existe", old.Name))
		default:
			s.serverError(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// DELETE /tasks/{name}
func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	n, err := s.store.DeleteTask(r.Context(), name)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("tarefa %q não existe", name))
		return
	}
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": name, "images": n})
}

// POST /tasks/{name}/import?dry_run=true&negatives=false   (multipart, campo "file")
// dry_run=true (padrão): só o relatório. dry_run=false: grava.
// negatives=true: imagens do arquivo sem nenhuma caixa entram como anotadas
// e vazias (ver matchBoxes). Labels que a tarefa ainda não tem são
// acrescentadas a ela.
func (s *Server) handleImportTask(w http.ResponseWriter, r *http.Request) {
	task, ok := s.taskByName(w, r, r.PathValue("name"))
	if !ok {
		return
	}
	q := r.URL.Query()
	dryRun := q.Get("dry_run") != "false"
	negatives := q.Get("negatives") == "true"

	fileName, data, ok := readUploadedFile(w, r, boxes.MaxFileBytes)
	if !ok {
		return
	}
	parsed, err := boxes.Read(data)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	rep, err := s.matchBoxes(r, task, parsed, negatives)
	if err != nil {
		s.serverError(w, err)
		return
	}
	rep.DryRun, rep.File = dryRun, fileName

	if !dryRun && len(rep.matched) > 0 {
		now := time.Now().UTC()
		user := userFrom(r)
		sets := make([]store.BoxSet, 0, len(rep.matched))
		for _, m := range rep.matched {
			sets = append(sets, store.BoxSet{
				Task: task.Name, ImageID: m.img.SHA1, Package: m.img.Package,
				Width: m.boxes.Width, Height: m.boxes.Height, Boxes: m.boxes.Boxes,
				Source:     store.BoxSource{Tool: "cvat", Format: "coco", File: fileName, FileName: m.boxes.FileName},
				ImportedBy: user, ImportedAt: now,
			})
			rep.AppliedBoxes += len(m.boxes.Boxes)
		}
		// Labels primeiro: se gravar as caixas falhar, sobra só label a mais.
		if err := s.store.AddTaskLabels(r.Context(), task.Name, rep.NewLabels, now); err != nil {
			s.serverError(w, err)
			return
		}
		if err := s.store.ReplaceBoxSets(r.Context(), sets); err != nil {
			s.serverError(w, err)
			return
		}
		rep.Applied = len(sets)
	}
	writeJSON(w, http.StatusOK, rep)
}
