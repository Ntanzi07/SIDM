package processing

import (
	"context"
	"sort"
	"sync"
	"time"

	"SIDM/internal/platform"
)

// Memory é o repositório em memória, para os testes.
type Memory struct {
	mu   sync.Mutex
	Runs map[string]Run
}

func NewMemory() *Memory { return &Memory{Runs: map[string]Run{}} }

func (m *Memory) CreateRun(ctx context.Context, r *Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.Runs[r.ID]; ok {
		return platform.ErrDuplicate
	}
	m.Runs[r.ID] = *r
	return nil
}

func (m *Memory) SaveRun(ctx context.Context, r *Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.Runs[r.ID]; !ok {
		return platform.ErrNotFound
	}
	m.Runs[r.ID] = *r
	return nil
}

func (m *Memory) FindRun(ctx context.Context, id string) (*Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.Runs[id]
	if !ok {
		return nil, platform.ErrNotFound
	}
	return &r, nil
}

func (m *Memory) ListRuns(ctx context.Context, limit int64) ([]Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Run, 0, len(m.Runs))
	for _, r := range m.Runs {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	if limit > 0 && int64(len(out)) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *Memory) InterruptRunning(ctx context.Context, now time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for id, r := range m.Runs {
		if r.Status == StatusRunning {
			r.Status, r.FinishedAt = StatusInterrupted, &now
			m.Runs[id] = r
			n++
		}
	}
	return n, nil
}
