package boxes

// Labels na pesquisa: quantas caixas de cada label há nas imagens que passam
// nos filtros da pesquisa (GET /images/stats). Quem filtra as imagens é o
// módulo images (Images.SearchIDs); aqui só se contam as caixas delas.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"SIDM/internal/platform"
)

// LabelCount: caixas de uma label e em quantas imagens elas estão.
type LabelCount struct {
	Images int64 `json:"images" bson:"images"`
	Boxes  int64 `json:"boxes" bson:"boxes"`
}

// TaskStats: uma tarefa. Images = imagens anotadas nela (com ou sem caixas);
// Labels traz também as labels da tarefa que não aparecem (zero).
type TaskStats struct {
	Task   string                `json:"task"`
	Images int64                 `json:"images"`
	Boxes  int64                 `json:"boxes"`
	Labels map[string]LabelCount `json:"labels"`
}

// LabelStats é a resposta do GET /boxes/stats. Images = imagens que passam
// na pesquisa (a base para "não anotadas na tarefa": Images − tarefa.Images).
type LabelStats struct {
	Images int64       `json:"images"`
	Tasks  []TaskStats `json:"tasks"`
}

// statsChunk: quantos sha1 por consulta (um $in muito grande pesa no banco).
const statsChunk = 5000

// LabelStats conta as labels das tarefas (vazio = todas) nas imagens da
// pesquisa q. Tarefa que não existe: platform.ErrNotFound.
func (s *Service) LabelStats(ctx context.Context, q map[string][]string, tasks []string) (*LabelStats, error) {
	all, err := s.repo.ListTasks(ctx)
	if err != nil {
		return nil, err
	}
	byName := map[string]Task{}
	for _, t := range all {
		byName[t.Name] = t
	}
	if len(tasks) == 0 {
		for _, t := range all {
			tasks = append(tasks, t.Name)
		}
		slices.Sort(tasks)
	}
	for _, name := range tasks {
		if _, ok := byName[name]; !ok {
			return nil, fmt.Errorf("tarefa %q: %w", name, platform.ErrNotFound)
		}
	}

	ids, err := s.images.SearchIDs(ctx, q)
	if err != nil {
		return nil, err
	}
	counted := map[string]*TaskStats{}
	if len(tasks) > 0 {
		for start := 0; start < len(ids); start += statsChunk {
			part, err := s.repo.LabelStats(ctx, tasks, ids[start:min(start+statsChunk, len(ids))])
			if err != nil {
				return nil, err
			}
			// os pedaços não se cruzam (cada imagem está em um só): é só somar
			for name, ts := range part {
				acc := counted[name]
				if acc == nil {
					acc = &TaskStats{Task: name, Labels: map[string]LabelCount{}}
					counted[name] = acc
				}
				acc.Images += ts.Images
				acc.Boxes += ts.Boxes
				for label, c := range ts.Labels {
					l := acc.Labels[label]
					l.Images += c.Images
					l.Boxes += c.Boxes
					acc.Labels[label] = l
				}
			}
		}
	}

	out := &LabelStats{Images: int64(len(ids)), Tasks: []TaskStats{}}
	for _, name := range tasks {
		ts := TaskStats{Task: name, Labels: map[string]LabelCount{}}
		if c := counted[name]; c != nil {
			ts = *c
		}
		for _, label := range byName[name].Labels {
			if _, ok := ts.Labels[label]; !ok {
				ts.Labels[label] = LabelCount{}
			}
		}
		out.Tasks = append(out.Tasks, ts)
	}
	return out, nil
}

// GET /boxes/stats?task=a&task=b&profile=veiculos&status=...&package=...&fields.<campo>=...
//
// task: repetido para várias (sem nenhuma: todas). O resto são os filtros do
// GET /images/stats.
func (s *Service) handleLabelStats(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var tasks []string
	for _, t := range q["task"] {
		if t = strings.TrimSpace(t); t != "" && !slices.Contains(tasks, t) {
			tasks = append(tasks, t)
		}
	}
	st, err := s.LabelStats(r.Context(), q, tasks)
	switch {
	case errors.Is(err, platform.ErrInvalid):
		platform.WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, platform.ErrNotFound):
		platform.WriteError(w, http.StatusNotFound, err.Error())
	case err != nil:
		platform.ServerError(w, err)
	default:
		platform.WriteJSON(w, http.StatusOK, st)
	}
}
