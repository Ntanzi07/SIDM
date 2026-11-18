package imports

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"SIDM/internal/importer"
	"SIDM/internal/platform"
)

// Memory é o Repo em memória, para os testes (deste e dos outros módulos).
// Profiles é exportado para os testes montarem o cenário direto.
type Memory struct {
	mu       sync.Mutex
	Profiles map[string]importer.Profile
}

func NewMemory() *Memory { return &Memory{Profiles: map[string]importer.Profile{}} }

func paginate[T any](all []T, limit, skip int64) []T {
	if skip >= int64(len(all)) {
		return []T{}
	}
	end := skip + limit
	if end > int64(len(all)) {
		end = int64(len(all))
	}
	return all[skip:end]
}

func (m *Memory) CreateProfile(ctx context.Context, p *importer.Profile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.Profiles[p.Name]; ok {
		return platform.ErrDuplicate
	}
	m.Profiles[p.Name] = *p
	return nil
}

func (m *Memory) FindProfile(ctx context.Context, name string) (*importer.Profile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.Profiles[name]
	if !ok {
		return nil, platform.ErrNotFound
	}
	p.Normalize()
	return &p, nil
}

func (m *Memory) ReplaceProfile(ctx context.Context, p *importer.Profile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	old, ok := m.Profiles[p.Name]
	if !ok {
		return platform.ErrNotFound
	}
	p.CreatedBy = old.CreatedBy
	m.Profiles[p.Name] = *p
	return nil
}

func (m *Memory) RenameProfile(ctx context.Context, oldName, newName string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.Profiles[oldName]
	if !ok {
		return platform.ErrNotFound
	}
	if _, taken := m.Profiles[newName]; taken {
		return platform.ErrDuplicate
	}
	delete(m.Profiles, oldName)
	p.Name, p.UpdatedAt = newName, now
	m.Profiles[newName] = p
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

func (m *Memory) ListProfiles(ctx context.Context, q string, limit, skip int64) ([]importer.Profile, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []importer.Profile{}
	for _, p := range m.Profiles {
		if strings.Contains(strings.ToLower(p.Name), strings.ToLower(q)) {
			p.Normalize()
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return paginate(out, limit, skip), int64(len(out)), nil
}
