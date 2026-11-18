package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"SIDM/internal/catalog"
)

func TestCountUsingERenameOnPending(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	// 2 pendentes e 1 verificada com o equipamento "velho"; 1 pendente com outro.
	for i, st := range []string{StatusPending, StatusPending, StatusVerified} {
		img := newImage(fmt.Sprintf("img-%d", i), time.Now())
		img.Status, img.Equipment, img.Camera = st, "velho", "cam-x"
		if err := s.InsertImage(ctx, img); err != nil {
			t.Fatal(err)
		}
	}
	other := newImage("img-outro", time.Now())
	other.Equipment = "outro"
	s.InsertImage(ctx, other)

	if n, _ := s.CountUsing(ctx, catalog.Equipments, "velho"); n != 3 {
		t.Errorf("CountUsing(equipamento) = %d, want 3 (qualquer status)", n)
	}
	if n, _ := s.CountUsing(ctx, catalog.Cameras, "cam-x"); n != 3 {
		t.Errorf("CountUsing(câmera) = %d, want 3", n)
	}
	if n, _ := s.CountUsing(ctx, catalog.Cameras, "velho"); n != 0 {
		t.Error("o nome do equipamento não pode contar como câmera")
	}

	now := ms(time.Now().Add(time.Minute))
	n, err := s.RenameOnPending(ctx, catalog.Equipments, "velho", "novo", now)
	if err != nil || n != 2 {
		t.Fatalf("RenameOnPending = %d, %v; want 2 (só as pendentes)", n, err)
	}

	for _, id := range []string{"img-0", "img-1"} {
		img, _ := s.FindImage(ctx, id)
		if img.Equipment != "novo" || !img.UpdatedAt.Equal(now) {
			t.Errorf("%s: equipment=%q updated_at=%v", id, img.Equipment, img.UpdatedAt)
		}
		if img.Camera != "cam-x" {
			t.Error("renomear o equipamento não pode mexer na câmera")
		}
	}
	if v, _ := s.FindImage(ctx, "img-2"); v.Equipment != "velho" {
		t.Error("a verificada é renomeada pela API (arquivo + banco), não aqui")
	}
	if o, _ := s.FindImage(ctx, "img-outro"); o.Equipment != "outro" {
		t.Error("imagem de outro equipamento foi alterada")
	}
}

func TestRenamePackageImages(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	img := newImage("a", time.Now())
	img.Package, img.Path = "velho", "dataset/velho/a.jpg"
	s.InsertImage(ctx, img)
	img2 := newImage("b", time.Now())
	img2.Package, img2.Path = "velho", "dataset/velho/b_v2_ção.jpg" // caractere multibyte no nome
	s.InsertImage(ctx, img2)
	keep := newImage("c", time.Now())
	keep.Package, keep.Path = "velho-2", "dataset/velho-2/c.jpg" // prefixo parecido: não pode ser afetado
	s.InsertImage(ctx, keep)

	n, err := s.RenamePackageImages(ctx, "velho", "novo-nome-maior")
	if err != nil || n != 2 {
		t.Fatalf("RenamePackageImages = %d, %v; want 2", n, err)
	}
	want := map[string]string{
		"a": "dataset/novo-nome-maior/a.jpg",
		"b": "dataset/novo-nome-maior/b_v2_ção.jpg",
		"c": "dataset/velho-2/c.jpg",
	}
	for id, path := range want {
		got, _ := s.FindImage(ctx, id)
		if got.Path != path {
			t.Errorf("%s: path = %q, want %q", id, got.Path, path)
		}
	}
	if got, _ := s.FindImage(ctx, "a"); got.Package != "novo-nome-maior" {
		t.Errorf("package = %q", got.Package)
	}
	if got, _ := s.FindImage(ctx, "c"); got.Package != "velho-2" {
		t.Error("pacote com prefixo parecido foi afetado")
	}
}

func TestDeletePackageImages(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	for _, id := range []string{"x1", "x2"} {
		img := newImage(id, time.Now())
		img.Package = "apagar"
		s.InsertImage(ctx, img)
	}
	keep := newImage("k1", time.Now())
	keep.Package = "manter"
	s.InsertImage(ctx, keep)

	now := ms(time.Now())
	s.InsertHistory(ctx, []HistoryEntry{
		{ImageID: "x1", Field: "status", New: "verified", ChangedBy: "n", ChangedAt: now},
		{ImageID: "x2", Field: "status", New: "verified", ChangedBy: "n", ChangedAt: now},
		{ImageID: "k1", Field: "status", New: "verified", ChangedBy: "n", ChangedAt: now},
	})

	n, err := s.DeletePackageImages(ctx, "apagar")
	if err != nil || n != 2 {
		t.Fatalf("DeletePackageImages = %d, %v; want 2", n, err)
	}
	for _, id := range []string{"x1", "x2"} {
		if _, err := s.FindImage(ctx, id); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s deveria ter sumido", id)
		}
	}
	hist := s.db.Collection("image_history")
	if c, _ := hist.CountDocuments(ctx, bson.D{{Key: "image_id", Value: bson.D{{Key: "$in", Value: bson.A{"x1", "x2"}}}}}); c != 0 {
		t.Errorf("sobraram %d entradas de histórico", c)
	}
	if c, _ := hist.CountDocuments(ctx, bson.D{{Key: "image_id", Value: "k1"}}); c != 1 {
		t.Error("o histórico de outro pacote não pode ser apagado")
	}
	if _, err := s.FindImage(ctx, "k1"); err != nil {
		t.Error("a imagem de outro pacote não pode ser apagada")
	}
	if n, err := s.DeletePackageImages(ctx, "apagar"); err != nil || n != 0 {
		t.Errorf("apagar duas vezes: %d, %v", n, err)
	}
}
