package exports

import (
	"context"
	"sort"
	"sync"

	"SIDM/internal/export"
	"SIDM/internal/platform"
)

// Memory é o Repo em memória, para os testes (deste e dos outros módulos).
// Profiles é exportado para os testes montarem o cenário direto.
type Memory struct {
	mu       sync.Mutex
	Profiles map[string]export.Profile
}

func NewMemory() *Memory { return &Memory{Profiles: map[string]export.Profile{}} }

// TaskRenamed: os perfis de export guardam a tarefa (boxes.TaskListener).
func (m *Memory) TaskRenamed(ctx context.Context, oldName, newName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, p := range m.Profiles {
		if p.Task == oldName {
			p.Task = newName
			m.Profiles[name] = p
		}
	}
	return nil
}

func (m *Memory) CreateExportProfile(ctx context.Context, p *export.Profile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.Profiles[p.Name]; ok {
		return platform.ErrDuplicate
	}
	m.Profiles[p.Name] = *p
	return nil
}

func (m *Memory) FindExportProfile(ctx context.Context, name string) (*export.Profile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.Profiles[name]
	if !ok {
		return nil, platform.ErrNotFound
	}
	return &p, nil
}

func (m *Memory) ReplaceExportProfile(ctx context.Context, p *export.Profile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.Profiles[p.Name]; !ok {
		return platform.ErrNotFound
	}
	m.Profiles[p.Name] = *p
	return nil
}

func (m *Memory) DeleteExportProfile(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.Profiles[name]; !ok {
		return platform.ErrNotFound
	}
	delete(m.Profiles, name)
	return nil
}

func (m *Memory) ListExportProfiles(ctx context.Context) ([]export.Profile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []export.Profile{}
	for _, p := range m.Profiles {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
