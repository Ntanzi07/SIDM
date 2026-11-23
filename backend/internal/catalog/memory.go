package catalog

import (
	"context"
	"sort"
	"strings"
	"sync"

	"SIDM/internal/platform"
)

// Memory é o catálogo em memória, para os testes (deste e dos outros
// módulos). Os campos são exportados para os testes montarem o cenário
// direto; Err simula falha do banco nas buscas.
type Memory struct {
	mu sync.Mutex

	Packages   map[string]Package
	Equipments map[string]bool // nome -> ativo (a chave existir = está cadastrado)
	Cameras    map[string]bool
	Err        error

	// LastSearch: os parâmetros da última busca (para conferir a paginação).
	LastSearch struct {
		Q           string
		Limit, Skip int64
	}
}

func NewMemory() *Memory {
	return &Memory{Packages: map[string]Package{}, Equipments: map[string]bool{}, Cameras: map[string]bool{}}
}

func (m *Memory) set(reg Registry) map[string]bool {
	if reg == Cameras {
		return m.Cameras
	}
	return m.Equipments
}

func paginate[T any](all []T, limit, skip int64) []T {
	if skip >= int64(len(all)) {
		return []T{}
	}
	return all[skip:min(skip+limit, int64(len(all)))]
}

func (m *Memory) CreatePackage(ctx context.Context, p Package) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.Packages[p.Name]; ok {
		return platform.ErrDuplicate
	}
	m.Packages[p.Name] = p
	return nil
}

func (m *Memory) FindPackage(ctx context.Context, name string) (*Package, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.Packages[name]
	if !ok {
		return nil, platform.ErrNotFound
	}
	return &p, nil
}

// ListPackages: em ordem de nome (os testes não dependem da data).
func (m *Memory) ListPackages(ctx context.Context, q string, limit, skip int64) ([]Package, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LastSearch.Q, m.LastSearch.Limit, m.LastSearch.Skip = q, limit, skip
	if m.Err != nil {
		return nil, 0, m.Err
	}
	all := []Package{}
	for _, p := range m.Packages {
		if strings.Contains(strings.ToLower(p.Name), strings.ToLower(q)) {
			all = append(all, p)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })
	return paginate(all, limit, skip), int64(len(all)), nil
}

func (m *Memory) RenamePackage(ctx context.Context, oldName, newName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.Packages[oldName]
	if !ok {
		return platform.ErrNotFound
	}
	if _, exists := m.Packages[newName]; exists {
		return platform.ErrDuplicate
	}
	delete(m.Packages, oldName)
	p.Name = newName
	m.Packages[newName] = p
	return nil
}

func (m *Memory) PackagesWithProfile(ctx context.Context, profile string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []string{}
	for _, p := range m.Packages {
		if p.Profile == profile {
			out = append(out, p.Name)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (m *Memory) SetPackageProfile(ctx context.Context, name, profile string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.Packages[name]
	if !ok {
		return platform.ErrNotFound
	}
	p.Profile = profile
	m.Packages[name] = p
	return nil
}

func (m *Memory) DeletePackage(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.Packages[name]; !ok {
		return platform.ErrNotFound
	}
	delete(m.Packages, name)
	return nil
}

// Search imita o Mongo: contém, sem diferenciar maiúscula, só os ativos,
// em ordem alfabética, paginado.
func (m *Memory) Search(ctx context.Context, reg Registry, q string, active bool, limit, skip int64) ([]string, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LastSearch.Q, m.LastSearch.Limit, m.LastSearch.Skip = q, limit, skip
	if m.Err != nil {
		return nil, 0, m.Err
	}
	all := []string{}
	for n, on := range m.set(reg) {
		if on == active && strings.Contains(strings.ToLower(n), strings.ToLower(q)) {
			all = append(all, n)
		}
	}
	sort.Strings(all)
	return paginate(all, limit, skip), int64(len(all)), nil
}

func (m *Memory) Create(ctx context.Context, reg Registry, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	set := m.set(reg)
	if _, ok := set[name]; ok {
		return platform.ErrDuplicate
	}
	set[name] = true
	return nil
}

func (m *Memory) Find(ctx context.Context, reg Registry, name string) (*RegistryEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	active, ok := m.set(reg)[name]
	if !ok {
		return nil, platform.ErrNotFound
	}
	return &RegistryEntry{Name: name, Active: active}, nil
}

func (m *Memory) ActiveExists(ctx context.Context, reg Registry, name string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.set(reg)[name], nil
}

func (m *Memory) SetActive(ctx context.Context, reg Registry, name string, active bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	set := m.set(reg)
	if _, ok := set[name]; !ok {
		return platform.ErrNotFound
	}
	set[name] = active
	return nil
}

func (m *Memory) Delete(ctx context.Context, reg Registry, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	set := m.set(reg)
	if _, ok := set[name]; !ok {
		return platform.ErrNotFound
	}
	delete(set, name)
	return nil
}
