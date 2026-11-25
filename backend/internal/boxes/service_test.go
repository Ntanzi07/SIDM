package boxes

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"SIDM/internal/annotation"
	"SIDM/internal/platform"
)

// fakeImages: só o que o serviço usa fora do import (o import é testado na
// API, com arquivos de verdade).
type fakeImages struct{ verified map[string]bool }

func (f fakeImages) BySHA1(ctx context.Context, shas []string) ([]ImageRef, error) { return nil, nil }
func (f fakeImages) ByOriginalName(ctx context.Context, names []string) ([]ImageRef, error) {
	return nil, nil
}
func (f fakeImages) Exists(ctx context.Context, sha1 string) (bool, error) { return true, nil }
func (f fakeImages) InPackage(ctx context.Context, pkg string, limit, skip int64) ([]ImageRef, int64, error) {
	return nil, 0, nil
}
func (f fakeImages) Verified(ctx context.Context, ids []string) (map[string]bool, error) {
	return f.verified, nil
}

func (f fakeImages) LinkBoxes(context.Context, string, string, []annotation.SeenBox, string) (annotation.LinkResult, error) {
	return annotation.LinkResult{}, nil
}

// SearchIDs: as imagens de verified, todas (o filtro é testado na API).
func (f fakeImages) SearchIDs(ctx context.Context, q url.Values) ([]string, error) {
	ids := []string{}
	for id := range f.verified {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids, nil
}

type fakeListener struct {
	log *[]string
	err error
}

func (l fakeListener) TaskRenamed(ctx context.Context, oldName, newName string) error {
	*l.log = append(*l.log, oldName+"->"+newName)
	return l.err
}

func set(task, sha, pkg string, labels ...string) Set {
	s := Set{Task: task, ImageID: sha, Package: pkg}
	for _, l := range labels {
		s.Boxes = append(s.Boxes, Box{Label: l, W: 1, H: 1})
	}
	return s
}

func TestServiceTarefas(t *testing.T) {
	mem := NewMemory()
	var log []string
	listener := &fakeListener{log: &log}
	s := New(mem, fakeImages{verified: map[string]bool{"a": true}}, nil, nil, nil, listener)
	ctx := context.Background()

	// criar: valida e limpa
	task, err := s.CreateTask(ctx, TaskInput{Name: " capacete ", Description: " motos ", Labels: []string{" com ", "sem", "com", ""}}, "nathan")
	if err != nil {
		t.Fatal(err)
	}
	if task.Name != "capacete" || task.Description != "motos" || strings.Join(task.Labels, ",") != "com,sem" || task.CreatedBy != "nathan" || task.CreatedAt.IsZero() {
		t.Errorf("tarefa = %+v", task)
	}
	var invalid *InvalidError
	for _, in := range []TaskInput{{Name: "Com Espaço"}, {Name: ""}, {Name: "-x"}, {Name: "x", Labels: []string{strings.Repeat("a", 65)}}} {
		if _, err := s.CreateTask(ctx, in, "n"); !errors.As(err, &invalid) {
			t.Errorf("%+v: %v", in, err)
		}
	}
	if _, err := s.CreateTask(ctx, TaskInput{Name: "capacete"}, "n"); !errors.Is(err, platform.ErrDuplicate) {
		t.Errorf("duplicada: %v", err)
	}

	// resumo: a verificada, b pendente (sem caixas), c em outro pacote
	mem.Put(set("capacete", "a", "p1", "com", "com"))
	mem.Put(set("capacete", "b", "p1"))
	mem.Put(set("capacete", "c", "p2", "sem"))
	mem.Put(set("outra", "a", "p1", "x"))
	st, err := s.Stats(ctx, "capacete")
	if err != nil {
		t.Fatal(err)
	}
	if st.Images != 3 || st.Verified != 1 || st.Negatives != 1 || st.Boxes != 3 || st.Labels["com"] != 2 ||
		fmt.Sprint(st.Packages) != "[{p1 2 1} {p2 1 0}]" {
		t.Errorf("stats = %+v", st)
	}

	// renomear: caixas, aviso, e a tarefa por último
	mem.Tasks["outra"] = Task{Name: "outra"}
	if _, err := s.UpdateTask(ctx, "capacete", TaskInput{Name: "outra"}); !errors.Is(err, platform.ErrDuplicate) {
		t.Errorf("para nome existente: %v", err)
	}
	got, err := s.UpdateTask(ctx, "capacete", TaskInput{Name: "moto-capacete", Labels: []string{"com"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.CreatedBy != "nathan" || fmt.Sprint(log) != "[capacete->moto-capacete]" {
		t.Errorf("rename = %+v, avisos %v", got, log)
	}
	if _, ok := mem.Get("moto-capacete", "a"); !ok {
		t.Error("as caixas deveriam ir para o nome novo")
	}
	if _, ok := mem.Tasks["capacete"]; ok {
		t.Error("o nome antigo deveria sumir")
	}

	// o aviso falhou: a tarefa fica com o nome antigo (repetir termina)
	listener.err = errors.New("banco fora")
	if _, err := s.UpdateTask(ctx, "moto-capacete", TaskInput{Name: "capacete2"}); err == nil {
		t.Error("deveria falhar")
	}
	if _, ok := mem.Tasks["moto-capacete"]; !ok {
		t.Error("com o aviso falhando, a tarefa tem que continuar com o nome antigo")
	}
	listener.err = nil
	if _, err := s.UpdateTask(ctx, "moto-capacete", TaskInput{Name: "capacete2"}); err != nil {
		t.Fatalf("repetir: %v", err)
	}
	if _, ok := mem.Get("capacete2", "a"); !ok {
		t.Error("repetir deveria terminar a mudança")
	}

	// apagar: as caixas dela, e só dela
	n, err := s.DeleteTask(ctx, "capacete2")
	if err != nil || n != 3 {
		t.Errorf("delete = %d, %v", n, err)
	}
	if _, ok := mem.Get("outra", "a"); !ok {
		t.Error("as caixas da outra tarefa têm que ficar")
	}
	if _, err := s.DeleteTask(ctx, "capacete2"); !errors.Is(err, platform.ErrNotFound) {
		t.Errorf("delete de novo: %v", err)
	}
}

func TestServiceAvisos(t *testing.T) {
	mem := NewMemory()
	s := New(mem, fakeImages{}, nil, nil, nil)
	ctx := context.Background()
	mem.Put(set("t", "a", "velho", "placa"))
	mem.Put(set("t", "b", "outro", "placa"))
	mem.Put(set("u", "a", "velho"))

	if err := s.PackageRenamed(ctx, "velho", "novo"); err != nil {
		t.Fatal(err)
	}
	if got, _ := mem.Get("t", "a"); got.Package != "novo" {
		t.Errorf("pacote = %q", got.Package)
	}
	if err := s.PackageDeleted(ctx, "novo"); err != nil {
		t.Fatal(err)
	}
	if len(mem.Sets) != 1 {
		t.Errorf("sobrou %d (deveria sobrar só a do outro pacote)", len(mem.Sets))
	}
	if err := s.DeleteImage(ctx, "b"); err != nil || len(mem.Sets) != 0 {
		t.Errorf("DeleteImage: %v, sobrou %d", err, len(mem.Sets))
	}
}

// emptyProfiles: perfil sem tipos de objeto (os atributos ficam como vieram).
type emptyProfiles struct{}

func (emptyProfiles) ProfileOf(context.Context, string) (*annotation.Profile, error) {
	return &annotation.Profile{Name: "vazio"}, nil
}

// Edições no mesmo milissegundo do import (o relógio parado): cada uma tem
// de gerar uma versão nova, senão a versão velha passaria.
func TestEdicoesNoMesmoMilissegundo(t *testing.T) {
	ctx := context.Background()
	mem := NewMemory()
	at := time.Date(2026, 11, 19, 12, 0, 0, 937_000_000, time.UTC)
	mem.Put(Set{Task: "capacete", ImageID: "a", Package: "p", ImportedAt: at, Boxes: []Box{{Label: "helmet", W: 1, H: 1}}})
	s := New(mem, fakeImages{}, nil, nil, emptyProfiles{})
	s.now = func() time.Time { return at }

	edit := func(v time.Time) (*Set, error) {
		in := AttributesInput{Version: v}
		in.Boxes = make([]struct {
			Attributes map[string]any `json:"attributes"`
		}, 1)
		return s.UpdateAttributes(ctx, "a", "capacete", in, "nathan")
	}
	first, err := edit(at)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Version().After(at) {
		t.Fatalf("a versão não mudou: %s", first.Version())
	}
	second, err := edit(first.Version())
	if err != nil {
		t.Fatal(err)
	}
	if !second.Version().After(first.Version()) {
		t.Fatalf("a segunda versão não é maior: %s", second.Version())
	}
	for _, old := range []time.Time{at, first.Version()} {
		if _, err := edit(old); !errors.Is(err, ErrConflict) {
			t.Errorf("versão velha %s: %v", old.Format(time.RFC3339Nano), err)
		}
	}
}

// As labels das tarefas pedidas, só nas imagens da pesquisa: caixas e
// imagens por label, e as labels da tarefa que não aparecem com zero.
func TestContarLabels(t *testing.T) {
	ctx := context.Background()
	mem := NewMemory()
	mem.Tasks["capacete"] = Task{Name: "capacete", Labels: []string{"helmet", "no_helmet", "rider"}}
	mem.Tasks["placa"] = Task{Name: "placa", Labels: []string{"placa"}}
	mem.Put(Set{Task: "capacete", ImageID: "a", Boxes: []Box{{Label: "helmet"}, {Label: "helmet"}, {Label: "rider"}}})
	mem.Put(Set{Task: "capacete", ImageID: "b", Boxes: []Box{{Label: "helmet"}}})
	mem.Put(Set{Task: "capacete", ImageID: "c"}) // anotada sem caixas (negativa)
	mem.Put(Set{Task: "placa", ImageID: "a", Boxes: []Box{{Label: "placa"}}})
	mem.Put(Set{Task: "capacete", ImageID: "fora", Boxes: []Box{{Label: "helmet"}}}) // fora da pesquisa
	s := New(mem, fakeImages{verified: map[string]bool{"a": true, "b": true, "c": true, "d": true}}, nil, nil, nil)

	st, err := s.LabelStats(ctx, nil, []string{"capacete"})
	if err != nil {
		t.Fatal(err)
	}
	if st.Images != 4 || len(st.Tasks) != 1 {
		t.Fatalf("%+v", st)
	}
	c := st.Tasks[0]
	if c.Images != 3 || c.Boxes != 4 || c.Labels["helmet"] != (LabelCount{Images: 2, Boxes: 3}) ||
		c.Labels["rider"] != (LabelCount{Images: 1, Boxes: 1}) || c.Labels["no_helmet"] != (LabelCount{}) {
		t.Errorf("capacete: %+v", c)
	}
	// sem tarefa: todas, em ordem
	if st, _ = s.LabelStats(ctx, nil, nil); len(st.Tasks) != 2 || st.Tasks[1].Task != "placa" || st.Tasks[1].Labels["placa"].Boxes != 1 {
		t.Errorf("todas: %+v", st.Tasks)
	}
	if _, err := s.LabelStats(ctx, nil, []string{"nao-existe"}); !errors.Is(err, platform.ErrNotFound) {
		t.Errorf("tarefa inexistente: %v", err)
	}
}

func TestTarefasDosPacotes(t *testing.T) {
	mem := NewMemory()
	mem.Put(Set{Task: "placa", ImageID: "a", Package: "p1"})
	mem.Put(Set{Task: "capacete", ImageID: "b", Package: "p1"})
	mem.Put(Set{Task: "placa", ImageID: "c", Package: "p1"})
	mem.Put(Set{Task: "placa", ImageID: "d", Package: "p2"})
	got, err := mem.PackageTasks(context.Background())
	if err != nil || fmt.Sprint(got) != "map[p1:[capacete placa] p2:[placa]]" {
		t.Errorf("%v %v", got, err)
	}
}
