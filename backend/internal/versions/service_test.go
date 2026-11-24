package versions

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"testing"

	"SIDM/internal/annotation"
	"SIDM/internal/boxes"
	"SIDM/internal/images"
	"SIDM/internal/platform"
)

// fakeImages: as imagens em memória; a "pesquisa" devolve as verificadas do
// pacote pedido (ou todas), e falha com ErrInvalid num filtro "fields.x".
type fakeImages struct{ imgs map[string]images.Image }

func (f *fakeImages) SearchIDs(ctx context.Context, q url.Values) ([]string, error) {
	if q.Get("status") != images.StatusVerified {
		return nil, errors.New("a versão tem que pedir só as verificadas")
	}
	if q.Has("fields.x") {
		return nil, &InvalidError{"fields.x: o perfil não tem esse campo"}
	}
	ids := []string{}
	for id, img := range f.imgs {
		if img.Status == images.StatusVerified && (len(q["package"]) == 0 || slices.Contains(q["package"], img.Package)) {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids, nil
}

func (f *fakeImages) List(ctx context.Context, flt images.ImageFilter) ([]images.Image, int64, error) {
	var out []images.Image
	for _, id := range flt.IDs {
		if img, ok := f.imgs[id]; ok && (flt.Status == "" || img.Status == flt.Status) {
			out = append(out, img)
		}
	}
	return out, int64(len(out)), nil
}

// Restore: a volta, só na memória (o caminho de verdade é testado no api).
func (f *fakeImages) Restore(ctx context.Context, sha1 string, in images.RestoreInput, changedBy string) (*images.Image, bool, error) {
	img, ok := f.imgs[sha1]
	if !ok {
		return nil, false, platform.ErrNotFound
	}
	img.Fields, img.Objects = in.Fields, in.Objects
	f.imgs[sha1] = img
	return &img, false, nil
}

type fakeBoxes struct {
	tasks []boxes.Task
	sets  []boxes.Set
}

func (f *fakeBoxes) ListTasks(ctx context.Context) ([]boxes.Task, error) { return f.tasks, nil }

func (f *fakeBoxes) FindSets(ctx context.Context, task string, shas []string) (map[string]boxes.Set, error) {
	out := map[string]boxes.Set{}
	for _, s := range f.sets {
		if s.Task == task && slices.Contains(shas, s.ImageID) {
			out[s.ImageID] = s
		}
	}
	return out, nil
}

func (f *fakeBoxes) RestoreSets(ctx context.Context, sha1, pkg string, tasks []string, frozen map[string]boxes.Set, user string) error {
	f.sets = slices.DeleteFunc(f.sets, func(s boxes.Set) bool { return s.ImageID == sha1 && slices.Contains(tasks, s.Task) })
	for task, set := range frozen {
		set.Task, set.ImageID, set.Package = task, sha1, pkg
		f.sets = append(f.sets, set)
	}
	return nil
}

type fakeAnnotation struct{}

func (fakeAnnotation) FindProfile(ctx context.Context, name string) (*annotation.Profile, error) {
	if name != "veiculos" {
		return nil, platform.ErrNotFound
	}
	return &annotation.Profile{Name: "veiculos", Fields: []annotation.Field{
		{Key: "weather", Type: annotation.TypeChoice, Options: []annotation.Option{{Value: "clear"}, {Value: "rain"}}},
	}}, nil
}

func newTestService() (*Service, *Memory, *fakeImages, *fakeBoxes) {
	img := &fakeImages{imgs: map[string]images.Image{
		"a": {SHA1: "a", Package: "p1", Status: images.StatusVerified, FileName: "a_v2.jpg", Ext: ".jpg", Fields: map[string]any{"weather": "rain"}},
		"b": {SHA1: "b", Package: "p1", Status: images.StatusVerified, OriginalName: "b.jpg", Ext: ".jpg", Fields: map[string]any{"weather": "clear"}},
		"c": {SHA1: "c", Package: "p2", Status: images.StatusVerified, Ext: ".jpg", Fields: map[string]any{}},
		"d": {SHA1: "d", Package: "p1", Status: images.StatusPending, Ext: ".jpg", Fields: map[string]any{"weather": "rain"}},
	}}
	bx := &fakeBoxes{
		tasks: []boxes.Task{{Name: "placa", Labels: []string{"placa"}}, {Name: "capacete", Labels: []string{"helmet", "no_helmet"}}},
		sets: []boxes.Set{
			{Task: "placa", ImageID: "a", Width: 100, Height: 50, Boxes: []boxes.Box{{Label: "placa", X: 1, Y: 2, W: 3, H: 4}}},
			{Task: "capacete", ImageID: "a", Width: 100, Height: 50, Boxes: []boxes.Box{{Label: "helmet"}, {Label: "helmet"}}},
			{Task: "capacete", ImageID: "d", Boxes: []boxes.Box{{Label: "helmet"}}}, // pendente: fora
		},
	}
	mem := NewMemory()
	s := New(mem, img, bx, fakeAnnotation{})
	return s, mem, img, bx
}

func TestCriarDatasetEVersao(t *testing.T) {
	ctx := context.Background()
	s, mem, img, _ := newTestService()

	d, err := s.CreateDataset(ctx, DatasetInput{Name: " motos-noite ", Profile: "veiculos",
		Query: Query{Packages: []string{"p1", "p1", ""}, Filters: []Filter{{Key: "fields.weather", Value: "rain"}, {Key: "", Value: "x"}}}}, "nathan")
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "motos-noite" || len(d.Query.Packages) != 1 || len(d.Query.Filters) != 1 || d.CreatedBy != "nathan" {
		t.Errorf("dataset: %+v", d)
	}

	// sem mensagem: 400
	if _, err := s.CreateVersion(ctx, "motos-noite", "  ", "nathan"); !errors.Is(err, platform.ErrInvalid) {
		t.Errorf("sem mensagem: %v", err)
	}

	v1, err := s.CreateVersion(ctx, "motos-noite", "primeira", "nathan")
	if err != nil {
		t.Fatal(err)
	}
	// p1 verificadas: a e b (d é pendente; c é de outro pacote)
	if v1.Number != 1 || v1.Summary.Images.Total != 2 || v1.Summary.Images.Fields["weather"]["rain"] != 1 {
		t.Errorf("v1: %+v", v1.Summary.Images)
	}
	if !slices.Equal(v1.Tasks, []string{"capacete", "placa"}) {
		t.Errorf("tarefas congeladas (todas, em ordem): %v", v1.Tasks)
	}
	if len(v1.Summary.Labels) != 2 || v1.Summary.Labels[0].Labels["helmet"].Boxes != 2 || v1.Summary.Labels[0].Labels["no_helmet"] != (boxes.LabelCount{}) {
		t.Errorf("labels: %+v", v1.Summary.Labels)
	}
	items, total, _ := s.Items(ctx, "motos-noite", 1, 10, 0)
	if total != 2 || items[0].SHA1 != "a" || items[0].FileName != "a_v2.jpg" || items[1].FileName != "b.jpg" {
		t.Fatalf("itens: %+v", items)
	}
	if b := items[0].Boxes["placa"]; b.Width != 100 || len(b.Boxes) != 1 || len(items[0].Boxes["capacete"].Boxes) != 2 {
		t.Errorf("caixas congeladas: %+v", items[0].Boxes)
	}
	if items[0].Hash == "" || items[0].Hash == items[1].Hash {
		t.Errorf("hashes: %q %q", items[0].Hash, items[1].Hash)
	}

	// a anotação muda depois: a v1 não muda; a v2 tem o hash novo
	oldHash := items[0].Hash
	a := img.imgs["a"]
	a.Fields = map[string]any{"weather": "clear"}
	img.imgs["a"] = a
	v2, err := s.CreateVersion(ctx, "motos-noite", "corrigi o tempo da a", "guilherme")
	if err != nil {
		t.Fatal(err)
	}
	if v2.Number != 2 || v2.CreatedBy != "guilherme" {
		t.Errorf("v2: %+v", v2)
	}
	items1, _, _ := s.Items(ctx, "motos-noite", 1, 10, 0)
	items2, _, _ := s.Items(ctx, "motos-noite", 2, 10, 0)
	if items1[0].Hash != oldHash || items1[0].Fields["weather"] != "rain" || items2[0].Hash == oldHash {
		t.Errorf("a v1 mudou ou a v2 não: %+v / %+v", items1[0], items2[0])
	}
	if list, _ := s.ListVersions(ctx, "motos-noite"); len(list) != 2 || list[0].Number != 2 {
		t.Errorf("versões (mais nova primeiro): %+v", list)
	}
	if d, _ := s.FindDataset(ctx, "motos-noite"); d.Latest == nil || d.Latest.Number != 2 || d.Latest.Images != 2 {
		t.Errorf("latest: %+v", d.Latest)
	}
	if len(mem.Items) != 4 {
		t.Errorf("itens gravados: %d", len(mem.Items))
	}

	// com versões, só apaga pedindo junto o histórico
	var conflict *ConflictError
	if err := s.DeleteDataset(ctx, "motos-noite", false); !errors.As(err, &conflict) {
		t.Errorf("apagar com versões sem pedir: %v", err)
	}
	if err := s.DeleteDataset(ctx, "motos-noite", true); err != nil {
		t.Fatalf("apagar com as versões: %v", err)
	}
	if len(mem.Versions) != 0 || len(mem.Items) != 0 || len(mem.Datasets) != 0 {
		t.Errorf("sobrou: %d versões, %d itens, %d datasets", len(mem.Versions), len(mem.Items), len(mem.Datasets))
	}
	if err := s.DeleteDataset(ctx, "motos-noite", true); !errors.Is(err, platform.ErrNotFound) {
		t.Errorf("apagar de novo: %v", err)
	}
}

func TestDatasetInvalido(t *testing.T) {
	ctx := context.Background()
	s, _, _, _ := newTestService()
	for name, in := range map[string]DatasetInput{
		"sem nome":                     {Profile: "veiculos"},
		"barra no nome":                {Name: "a/b", Profile: "veiculos"},
		"sem perfil":                   {Name: "x"},
		"perfil não existe":            {Name: "x", Profile: "nao-existe"},
		"tarefa não existe":            {Name: "x", Profile: "veiculos", Query: Query{Tasks: []string{"nao-existe"}}},
		"filtro sem prefixo":           {Name: "x", Profile: "veiculos", Query: Query{Filters: []Filter{{Key: "weather", Value: "rain"}}}},
		"filtro que a pesquisa recusa": {Name: "x", Profile: "veiculos", Query: Query{Filters: []Filter{{Key: "fields.x", Value: "1"}}}},
	} {
		if _, err := s.CreateDataset(ctx, in, "nathan"); !errors.Is(err, platform.ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := s.CreateDataset(ctx, DatasetInput{Name: "x", Profile: "veiculos"}, "n"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateDataset(ctx, DatasetInput{Name: "x", Profile: "veiculos"}, "n"); !errors.Is(err, platform.ErrDuplicate) {
		t.Errorf("nome repetido: %v", err)
	}
	// sem imagens que passem: a versão é recusada
	if _, err := s.CreateDataset(ctx, DatasetInput{Name: "vazio", Profile: "veiculos", Query: Query{Packages: []string{"p9"}}}, "n"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateVersion(ctx, "vazio", "nada", "n"); !errors.Is(err, platform.ErrInvalid) {
		t.Errorf("versão vazia: %v", err)
	}
	if err := s.DeleteDataset(ctx, "vazio", false); err != nil {
		t.Errorf("apagar sem versões: %v", err)
	}
}

// Só as tarefas pedidas entram (e a tarefa sem caixas aparece com zero).
func TestVersaoComTarefasEscolhidas(t *testing.T) {
	ctx := context.Background()
	s, _, _, _ := newTestService()
	if _, err := s.CreateDataset(ctx, DatasetInput{Name: "placas", Profile: "veiculos", Query: Query{Tasks: []string{"placa"}}}, "n"); err != nil {
		t.Fatal(err)
	}
	v, err := s.CreateVersion(ctx, "placas", "só placa", "n")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(v.Tasks, []string{"placa"}) || len(v.Summary.Labels) != 1 || v.Summary.Labels[0].Labels["placa"].Boxes != 1 {
		t.Errorf("%v %+v", v.Tasks, v.Summary.Labels)
	}
	items, _, _ := s.Items(ctx, "placas", 1, 10, 0)
	for _, it := range items {
		if _, ok := it.Boxes["capacete"]; ok {
			t.Errorf("%s com caixas de capacete", it.SHA1)
		}
	}
}

// Duas versões ao mesmo tempo: a segunda que grava pega o número seguinte.
type racingRepo struct {
	*Memory
	once bool
}

func (r *racingRepo) InsertVersion(ctx context.Context, v *Version) error {
	if !r.once {
		r.once = true
		// alguém gravou o mesmo número antes
		_ = r.Memory.InsertVersion(ctx, &Version{ID: "outro", Dataset: v.Dataset, Number: v.Number})
	}
	return r.Memory.InsertVersion(ctx, v)
}

func TestVersaoAoMesmoTempo(t *testing.T) {
	ctx := context.Background()
	s, mem, _, _ := newTestService()
	r := &racingRepo{Memory: mem}
	s.repo = r
	if _, err := s.CreateDataset(ctx, DatasetInput{Name: "x", Profile: "veiculos"}, "n"); err != nil {
		t.Fatal(err)
	}
	v, err := s.CreateVersion(ctx, "x", "v", "n")
	if err != nil {
		t.Fatal(err)
	}
	if v.Number != 2 {
		t.Errorf("número = %d, quer 2", v.Number)
	}
	// os itens da tentativa recusada foram apagados
	for _, it := range mem.Items {
		if it.Version != v.ID {
			t.Errorf("item órfão da tentativa: %+v", it)
		}
	}
}
