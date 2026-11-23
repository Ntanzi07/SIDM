package boxes

import (
	"context"
	"slices"
	"sort"
	"sync"
	"time"

	"SIDM/internal/platform"
)

// Memory é o módulo em memória, para os testes (deste e dos outros
// módulos). Os campos são exportados para os testes montarem o cenário
// direto (use Key para a chave de Sets).
type Memory struct {
	mu    sync.Mutex
	Tasks map[string]Task
	Sets  map[string]Set // chave: Key(tarefa, sha1)
}

func NewMemory() *Memory {
	return &Memory{Tasks: map[string]Task{}, Sets: map[string]Set{}}
}

// Key é a chave de Memory.Sets.
func Key(task, sha1 string) string { return task + "|" + sha1 }

// Put grava as caixas de uma imagem numa tarefa (atalho dos testes).
func (m *Memory) Put(set Set) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if set.Boxes == nil {
		set.Boxes = []Box{}
	}
	m.Sets[Key(set.Task, set.ImageID)] = set
}

// Get: as caixas de uma imagem numa tarefa (atalho dos testes).
func (m *Memory) Get(task, sha1 string) (Set, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	set, ok := m.Sets[Key(task, sha1)]
	return set, ok
}

func (m *Memory) CreateTask(ctx context.Context, t *Task) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.Tasks[t.Name]; ok {
		return platform.ErrDuplicate
	}
	m.Tasks[t.Name] = *t
	return nil
}

func (m *Memory) FindTask(ctx context.Context, name string) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.Tasks[name]
	if !ok {
		return nil, platform.ErrNotFound
	}
	t.Labels = append([]string{}, t.Labels...)
	return &t, nil
}

func (m *Memory) ListTasks(ctx context.Context) ([]Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Task{}
	for _, t := range m.Tasks {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (m *Memory) ReplaceTask(ctx context.Context, oldName string, t *Task) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.Tasks[oldName]; !ok {
		return platform.ErrNotFound
	}
	if _, ok := m.Tasks[t.Name]; ok && t.Name != oldName {
		return platform.ErrDuplicate
	}
	delete(m.Tasks, oldName)
	m.Tasks[t.Name] = *t
	return nil
}

func (m *Memory) AddTaskLabels(ctx context.Context, name string, labels []string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.Tasks[name]
	if !ok {
		return platform.ErrNotFound
	}
	for _, l := range labels {
		if !slices.Contains(t.Labels, l) {
			t.Labels = append(t.Labels, l)
		}
	}
	t.UpdatedAt = now
	m.Tasks[name] = t
	return nil
}

func (m *Memory) DeleteTask(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.Tasks[name]; !ok {
		return platform.ErrNotFound
	}
	delete(m.Tasks, name)
	return nil
}

func (m *Memory) TaskImages(ctx context.Context, task string) (*TaskImages, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := &TaskImages{IDs: map[string][]string{}, Labels: map[string]int64{}}
	for _, set := range m.Sets {
		if set.Task != task {
			continue
		}
		out.IDs[set.Package] = append(out.IDs[set.Package], set.ImageID)
		if len(set.Boxes) == 0 {
			out.Negatives++
		}
		for _, b := range set.Boxes {
			out.Labels[b.Label]++
		}
	}
	return out, nil
}

func (m *Memory) ReplaceSets(ctx context.Context, sets []Set) error {
	for _, set := range sets {
		m.Put(set)
	}
	return nil
}

func (m *Memory) UpdateSet(ctx context.Context, set *Set, prev time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.Sets[Key(set.Task, set.ImageID)]
	if !ok {
		return platform.ErrNotFound
	}
	if !cur.Version().Equal(prev) {
		return ErrConflict
	}
	m.Sets[Key(set.Task, set.ImageID)] = *set
	return nil
}

func (m *Memory) FindImageSets(ctx context.Context, sha1 string) ([]Set, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Set{}
	for _, set := range m.Sets {
		if set.ImageID == sha1 {
			out = append(out, set)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Task < out[j].Task })
	return out, nil
}

func (m *Memory) FindSets(ctx context.Context, task string, ids []string) (map[string]Set, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]Set{}
	for _, id := range ids {
		if set, ok := m.Sets[Key(task, id)]; ok {
			out[id] = set
		}
	}
	return out, nil
}

func (m *Memory) DeleteImageSets(ctx context.Context, sha1 string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, set := range m.Sets {
		if set.ImageID == sha1 {
			delete(m.Sets, k)
		}
	}
	return nil
}

func (m *Memory) PackageCounts(ctx context.Context, pkg string) ([]PackageCount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	by := map[string]*PackageCount{}
	for _, set := range m.Sets {
		if set.Package != pkg {
			continue
		}
		c := by[set.Task]
		if c == nil {
			c = &PackageCount{Task: set.Task}
			by[set.Task] = c
		}
		c.Images++
		c.Boxes += int64(len(set.Boxes))
	}
	out := []PackageCount{}
	for _, c := range by {
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Task < out[j].Task })
	return out, nil
}

func (m *Memory) RenameTaskSets(ctx context.Context, oldName, newName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, set := range m.Sets {
		if set.Task == oldName {
			delete(m.Sets, k)
			set.Task = newName
			m.Sets[Key(newName, set.ImageID)] = set
		}
	}
	return nil
}

func (m *Memory) DeleteTaskSets(ctx context.Context, task string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for k, set := range m.Sets {
		if set.Task == task {
			delete(m.Sets, k)
			n++
		}
	}
	return n, nil
}

func (m *Memory) RenamePackageSets(ctx context.Context, oldName, newName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, set := range m.Sets {
		if set.Package == oldName {
			set.Package = newName
			m.Sets[k] = set
		}
	}
	return nil
}

func (m *Memory) DeletePackageSets(ctx context.Context, pkg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, set := range m.Sets {
		if set.Package == pkg {
			delete(m.Sets, k)
		}
	}
	return nil
}

func (m *Memory) LabelStats(ctx context.Context, tasks []string, ids []string) (map[string]*TaskStats, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var sets []*Set
	for _, set := range m.Sets {
		if slices.Contains(tasks, set.Task) && slices.Contains(ids, set.ImageID) {
			sets = append(sets, &set)
		}
	}
	return CountLabels(sets), nil
}

func (m *Memory) PackageTasks(ctx context.Context) (map[string][]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	seen := map[string]map[string]bool{}
	for _, set := range m.Sets {
		if seen[set.Package] == nil {
			seen[set.Package] = map[string]bool{}
		}
		seen[set.Package][set.Task] = true
	}
	out := map[string][]string{}
	for pkg, tasks := range seen {
		for t := range tasks {
			out[pkg] = append(out[pkg], t)
		}
		sort.Strings(out[pkg])
	}
	return out, nil
}
