package versions

import (
	"context"
	"slices"
	"sort"
	"strings"
	"sync"

	"SIDM/internal/platform"
)

// Memory é o Repo em memória, para os testes.
type Memory struct {
	mu       sync.Mutex
	Datasets map[string]Dataset
	Versions []Version
	Items    []Item
}

func NewMemory() *Memory { return &Memory{Datasets: map[string]Dataset{}} }

func (m *Memory) CreateDataset(ctx context.Context, d *Dataset) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.Datasets[d.Name]; ok {
		return platform.ErrDuplicate
	}
	m.Datasets[d.Name] = *d
	return nil
}

func (m *Memory) FindDataset(ctx context.Context, name string) (*Dataset, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.Datasets[name]
	if !ok {
		return nil, platform.ErrNotFound
	}
	return &d, nil
}

func (m *Memory) ListDatasets(ctx context.Context) ([]Dataset, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := []Dataset{}
	for _, d := range m.Datasets {
		list = append(list, d)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return list, nil
}

func (m *Memory) ReplaceDataset(ctx context.Context, d *Dataset) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.Datasets[d.Name]; !ok {
		return platform.ErrNotFound
	}
	m.Datasets[d.Name] = *d
	return nil
}

func (m *Memory) DeleteDataset(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.Datasets[name]; !ok {
		return platform.ErrNotFound
	}
	delete(m.Datasets, name)
	return nil
}

func (m *Memory) LastVersion(ctx context.Context, dataset string) (*Version, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var last *Version
	for i := range m.Versions {
		if v := &m.Versions[i]; v.Dataset == dataset && (last == nil || v.Number > last.Number) {
			last = v
		}
	}
	if last == nil {
		return nil, nil
	}
	v := *last
	return &v, nil
}

func (m *Memory) InsertVersion(ctx context.Context, v *Version) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, o := range m.Versions {
		if o.Dataset == v.Dataset && o.Number == v.Number {
			return platform.ErrDuplicate
		}
	}
	m.Versions = append(m.Versions, *v)
	return nil
}

func (m *Memory) FindVersion(ctx context.Context, dataset string, number int) (*Version, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, v := range m.Versions {
		if v.Dataset == dataset && v.Number == number {
			return &v, nil
		}
	}
	return nil, platform.ErrNotFound
}

func (m *Memory) ListVersions(ctx context.Context, dataset string) ([]Version, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := []Version{}
	for _, v := range m.Versions {
		if v.Dataset == dataset {
			list = append(list, v)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Number > list[j].Number })
	return list, nil
}

func (m *Memory) InsertItems(ctx context.Context, items []Item) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Items = append(m.Items, items...)
	return nil
}

func (m *Memory) DeleteItems(ctx context.Context, version string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Items = slices.DeleteFunc(m.Items, func(it Item) bool { return it.Version == version })
	return nil
}

func (m *Memory) ListItems(ctx context.Context, version string, limit, skip int64) ([]Item, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []Item
	for _, it := range m.Items {
		if it.Version == version {
			list = append(list, it)
		}
	}
	slices.SortFunc(list, func(a, b Item) int { return strings.Compare(a.SHA1, b.SHA1) })
	total := int64(len(list))
	start := min(skip, total)
	end := min(start+limit, total)
	return append([]Item{}, list[start:end]...), total, nil
}
