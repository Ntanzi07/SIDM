package annotation

import (
	"context"
	"sort"
	"sync"

	"SIDM/internal/platform"
)

// Memory é o Repo em memória, para os testes (deste e dos outros módulos).
type Memory struct {
	mu       sync.Mutex
	Profiles map[string]Profile
}

func NewMemory() *Memory { return &Memory{Profiles: map[string]Profile{}} }

func (m *Memory) CreateProfile(ctx context.Context, p *Profile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.Profiles[p.Name]; ok {
		return platform.ErrDuplicate
	}
	m.Profiles[p.Name] = *p
	return nil
}

func (m *Memory) FindProfile(ctx context.Context, name string) (*Profile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.Profiles[name]
	if !ok {
		return nil, platform.ErrNotFound
	}
	return &p, nil
}

func (m *Memory) ReplaceProfile(ctx context.Context, p *Profile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.Profiles[p.Name]; !ok {
		return platform.ErrNotFound
	}
	m.Profiles[p.Name] = *p
	return nil
}

func (m *Memory) DeleteProfile(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.Profiles[name]; !ok {
		return platform.ErrNotFound
	}
	delete(m.Profiles, name)
	return nil
}

func (m *Memory) ListProfiles(ctx context.Context) ([]Profile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Profile{}
	for _, p := range m.Profiles {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
