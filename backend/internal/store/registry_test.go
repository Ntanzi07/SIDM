package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestRegistryAtivarDesativarApagar(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	for _, reg := range []Registry{Equipments, Cameras} {
		t.Run(string(reg), func(t *testing.T) {
			create := s.CreateEquipment
			if reg == Cameras {
				create = s.CreateCamera
			}
			if err := create(ctx, "item-1"); err != nil {
				t.Fatal(err)
			}

			e, err := s.FindRegistry(ctx, reg, "item-1")
			if err != nil || !e.Active || e.Name != "item-1" {
				t.Fatalf("FindRegistry = %+v, %v", e, err)
			}
			if _, err := s.FindRegistry(ctx, reg, "nao-existe"); !errors.Is(err, ErrNotFound) {
				t.Errorf("inexistente: err = %v", err)
			}

			if err := s.SetRegistryActive(ctx, reg, "item-1", false); err != nil {
				t.Fatal(err)
			}
			if e, _ := s.FindRegistry(ctx, reg, "item-1"); e.Active {
				t.Error("deveria estar desativado")
			}
			if err := s.SetRegistryActive(ctx, reg, "nao-existe", true); !errors.Is(err, ErrNotFound) {
				t.Errorf("ativar inexistente: err = %v", err)
			}

			if err := s.DeleteRegistry(ctx, reg, "item-1"); err != nil {
				t.Fatal(err)
			}
			if _, err := s.FindRegistry(ctx, reg, "item-1"); !errors.Is(err, ErrNotFound) {
				t.Error("deveria ter sido apagado")
			}
			if err := s.DeleteRegistry(ctx, reg, "item-1"); !errors.Is(err, ErrNotFound) {
				t.Errorf("apagar duas vezes: err = %v", err)
			}
		})
	}
}

func TestCountImagesUsingERenameOnPending(t *testing.T) {
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

	if n, _ := s.CountImagesUsing(ctx, Equipments, "velho"); n != 3 {
		t.Errorf("CountImagesUsing(equipamento) = %d, want 3 (qualquer status)", n)
	}
	if n, _ := s.CountImagesUsing(ctx, Cameras, "cam-x"); n != 3 {
		t.Errorf("CountImagesUsing(câmera) = %d, want 3", n)
	}
	if n, _ := s.CountImagesUsing(ctx, Cameras, "velho"); n != 0 {
		t.Error("o nome do equipamento não pode contar como câmera")
	}

	now := ms(time.Now().Add(time.Minute))
	n, err := s.RenameOnPending(ctx, Equipments, "velho", "novo", now)
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

func TestRenamePackage(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	s.CreatePackage(ctx, "velho", "nathan")
	s.CreatePackage(ctx, "ocupado", "nathan")
	s.CreatePackage(ctx, "velho-2", "nathan") // prefixo parecido: não pode ser afetado

	img := newImage("a", time.Now())
	img.Package, img.Path = "velho", "dataset/velho/a.jpg"
	s.InsertImage(ctx, img)
	img2 := newImage("b", time.Now())
	img2.Package, img2.Path = "velho", "dataset/velho/b_v2_ção.jpg" // caractere multibyte no nome
	s.InsertImage(ctx, img2)
	keep := newImage("c", time.Now())
	keep.Package, keep.Path = "velho-2", "dataset/velho-2/c.jpg"
	s.InsertImage(ctx, keep)

	if _, err := s.RenamePackage(ctx, "velho", "ocupado"); !errors.Is(err, ErrDuplicate) {
		t.Errorf("para nome existente: err = %v, want ErrDuplicate", err)
	}
	if _, err := s.RenamePackage(ctx, "nao-existe", "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("inexistente: err = %v, want ErrNotFound", err)
	}

	n, err := s.RenamePackage(ctx, "velho", "novo-nome-maior")
	if err != nil || n != 2 {
		t.Fatalf("RenamePackage = %d, %v; want 2", n, err)
	}
	if _, err := s.FindPackage(ctx, "novo-nome-maior"); err != nil {
		t.Error("o cadastro do pacote deveria ter o nome novo")
	}
	if _, err := s.FindPackage(ctx, "velho"); !errors.Is(err, ErrNotFound) {
		t.Error("o nome antigo do pacote deveria ter sumido")
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
	if got, _ := s.FindImage(ctx, "c"); got.Package != "velho-2" {
		t.Error("pacote com prefixo parecido foi afetado")
	}
}

func TestDeletePackage(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	s.CreatePackage(ctx, "apagar", "nathan")
	s.CreatePackage(ctx, "manter", "nathan")
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

	n, err := s.DeletePackage(ctx, "apagar")
	if err != nil || n != 2 {
		t.Fatalf("DeletePackage = %d, %v; want 2", n, err)
	}
	if _, err := s.FindPackage(ctx, "apagar"); !errors.Is(err, ErrNotFound) {
		t.Error("o pacote deveria ter sumido")
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

	if _, err := s.DeletePackage(ctx, "apagar"); !errors.Is(err, ErrNotFound) {
		t.Errorf("apagar duas vezes: err = %v, want ErrNotFound", err)
	}
}
