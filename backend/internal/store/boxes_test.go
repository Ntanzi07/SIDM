package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"SIDM/internal/boxes"
)

func boxSet(id, pkg string, labels ...string) BoxSet {
	set := BoxSet{
		ImageID: id, Package: pkg, Width: 640, Height: 480,
		Source:     BoxSource{Tool: "cvat", Format: "coco", File: "t.json", FileName: id + ".jpg"},
		ImportedBy: "nathan", ImportedAt: ms(time.Now()),
	}
	for i, l := range labels {
		set.Boxes = append(set.Boxes, boxes.Box{
			Label: l, X: float64(i), Y: 1.5, W: 10, H: 20, Occluded: i == 1,
			Attributes: map[string]any{"caractere": "A"},
		})
	}
	return set
}

func TestBoxSets(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if err := s.ReplaceBoxSets(ctx, []BoxSet{
		boxSet("a", "p1", "placa", "carro"),
		boxSet("b", "p1", "placa"),
		boxSet("c", "p2", "moto"),
	}); err != nil {
		t.Fatal(err)
	}

	got, err := s.FindBoxSet(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	if got.Package != "p1" || got.Width != 640 || len(got.Boxes) != 2 || got.Source.Tool != "cvat" {
		t.Errorf("set = %+v", got)
	}
	if b := got.Boxes[1]; b.Label != "carro" || !b.Occluded || b.Y != 1.5 || b.Attributes["caractere"] != "A" {
		t.Errorf("caixa = %+v", b)
	}
	if _, err := s.FindBoxSet(ctx, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("inexistente: %v", err)
	}

	// substituir (upsert) e quais já têm caixas
	if err := s.ReplaceBoxSets(ctx, []BoxSet{boxSet("a", "p1", "placa")}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.FindBoxSet(ctx, "a"); len(got.Boxes) != 1 {
		t.Error("reimportar deveria substituir as caixas")
	}
	has, err := s.BoxImageIDs(ctx, []string{"a", "c", "x"})
	if err != nil || !has["a"] || !has["c"] || has["x"] {
		t.Errorf("BoxImageIDs = %v, %v", has, err)
	}

	images, labels, err := s.BoxLabelCounts(ctx, "p1")
	if err != nil || images != 2 || labels["placa"] != 2 || labels["moto"] != 0 {
		t.Errorf("BoxLabelCounts = %d %v %v", images, labels, err)
	}

	if err := s.DeleteBoxSet(ctx, "b"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FindBoxSet(ctx, "b"); !errors.Is(err, ErrNotFound) {
		t.Error("deveria ter sido apagada")
	}
}

func TestCaixasNoRenomearEApagarPacote(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	s.CreatePackage(ctx, "velho", "nathan")
	s.CreatePackage(ctx, "outro", "nathan")
	img := newImage("a", time.Now())
	img.Package, img.Path = "velho", "dataset/velho/a.jpg"
	s.InsertImage(ctx, img)
	s.ReplaceBoxSets(ctx, []BoxSet{boxSet("a", "velho", "placa"), boxSet("z", "outro", "placa")})

	if _, err := s.RenamePackage(ctx, "velho", "novo"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.FindBoxSet(ctx, "a"); got.Package != "novo" {
		t.Errorf("as caixas deveriam ir junto com o pacote: %q", got.Package)
	}

	if _, err := s.DeletePackage(ctx, "novo"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FindBoxSet(ctx, "a"); !errors.Is(err, ErrNotFound) {
		t.Error("apagar o pacote deveria apagar as caixas")
	}
	if _, err := s.FindBoxSet(ctx, "z"); err != nil {
		t.Error("as caixas de outro pacote não podem ser apagadas")
	}
}
