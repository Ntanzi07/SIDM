package boxes

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

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
