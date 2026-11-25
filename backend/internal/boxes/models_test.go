package boxes

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"SIDM/internal/annotation"
)

// plateProfile: um perfil com o tipo "plate" de caixa, que só tem o campo text.
type plateProfile struct{}

func (plateProfile) ProfileOf(context.Context, string) (*annotation.Profile, error) {
	return &annotation.Profile{Name: "placas", Objects: []annotation.ObjectType{{
		Key: "plate", Label: "Placa", Box: true,
		Fields: []annotation.Field{{Key: "text", Label: "Texto", Type: annotation.TypeText}},
	}}}, nil
}

func TestEnsureTask(t *testing.T) {
	ctx := context.Background()
	mem := NewMemory()
	s := New(mem, fakeImages{}, nil, nil, emptyProfiles{})

	if err := s.EnsureTask(ctx, TaskInput{Name: "pre-lpr", Description: "do modelo", Labels: []string{"car", "plate"}}, "ana"); err != nil {
		t.Fatal(err)
	}
	task, _ := s.FindTask(ctx, "pre-lpr")
	if task == nil || strings.Join(task.Labels, ",") != "car,plate" || task.CreatedBy != "ana" {
		t.Fatalf("criada: %+v", task)
	}
	// de novo: só junta as que faltam, sem tirar as que alguém pôs
	if err := mem.AddTaskLabels(ctx, "pre-lpr", []string{"moto"}, time.Now()); err != nil { // alguém pôs uma label pela tela
		t.Fatal(err)
	}
	if err := s.EnsureTask(ctx, TaskInput{Name: "pre-lpr", Labels: []string{"plate", "truck"}}, "bia"); err != nil {
		t.Fatal(err)
	}
	task, _ = s.FindTask(ctx, "pre-lpr")
	if strings.Join(task.Labels, ",") != "car,plate,moto,truck" || task.Description != "do modelo" {
		t.Errorf("depois: %+v", task)
	}
	if err := s.EnsureTask(ctx, TaskInput{Name: "Nome Ruim"}, "ana"); err == nil {
		t.Error("nome inválido deveria falhar")
	}
}

func TestPutModelSets(t *testing.T) {
	ctx := context.Background()
	mem := NewMemory()
	s := New(mem, fakeImages{}, nil, nil, plateProfile{})
	edited := time.Now()

	mem.Put(Set{Task: "pre-lpr", ImageID: "cvat", Package: "p", Source: Source{Tool: "cvat"}})
	mem.Put(Set{Task: "pre-lpr", ImageID: "editada", Package: "p", Source: Source{Tool: "lpr"}, UpdatedAt: &edited})
	mem.Put(Set{Task: "pre-lpr", ImageID: "velha", Package: "p", Source: Source{Tool: "lpr"}, Boxes: []Box{{Label: "car"}}})
	mem.Put(Set{Task: "outra", ImageID: "nova", Package: "p", Source: Source{Tool: "cvat"}}) // outra tarefa não conta

	in := func(id string) Set {
		return Set{Task: "pre-lpr", ImageID: id, Package: "p", Width: 640, Height: 480, Source: Source{File: "run:1"}, Boxes: []Box{
			{Label: "plate", W: 10, H: 5, Attributes: map[string]any{"text": "ABC1D23", "lixo": 1}},
			{Label: "A", W: 1, H: 1, Attributes: map[string]any{"score": 0.9}},
		}}
	}
	written, skipped, err := s.PutModelSets(ctx, []Set{in("cvat"), in("editada"), in("velha"), in("nova")}, "lpr", "ana")
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(skipped)
	if written != 2 || strings.Join(skipped, ",") != "cvat,editada" {
		t.Fatalf("written %d, skipped %v", written, skipped)
	}
	sets, _ := s.FindSets(ctx, "pre-lpr", []string{"cvat", "velha", "nova"})
	if sets["cvat"].Source.Tool != "cvat" {
		t.Error("o Set do CVAT mudou")
	}
	got := sets["nova"]
	if got.Source != (Source{Tool: "lpr", Format: "rpc", File: "run:1"}) || got.ImportedBy != "ana" || got.Width != 640 || len(got.Boxes) != 2 {
		t.Fatalf("gravado: %+v", got)
	}
	// o tipo plate do perfil só tem text; a label sem tipo fica como veio
	if len(got.Boxes[0].Attributes) != 1 || got.Boxes[0].Attributes["text"] != "ABC1D23" {
		t.Errorf("atributos da placa: %v", got.Boxes[0].Attributes)
	}
	if got.Boxes[1].Attributes["score"] != 0.9 {
		t.Errorf("atributos do caractere: %v", got.Boxes[1].Attributes)
	}
	if len(sets["velha"].Boxes) != 2 {
		t.Errorf("o Set do próprio modelo é regravado: %+v", sets["velha"])
	}
}
