package store

// Testes de INTEGRAÇÃO: rodam contra o Mongo de verdade (docker compose up -d mongo).
// Usam o usuário root do .env e um banco descartável (sidm_test_<timestamp>),
// apagado no fim de cada teste. O banco "sidm" de verdade nunca é tocado.
//
// Sem Mongo disponível, os testes são pulados (t.Skip): assim o `go test ./...`
// não quebra numa máquina sem Docker. Para forçar outra URI: MONGO_TEST_URI.

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	_ = godotenv.Load("../../../.env") // backend/internal/store -> SIDM/.env

	uri := os.Getenv("MONGO_TEST_URI")
	if uri == "" {
		user := os.Getenv("MONGO_ROOT_USER")
		if user == "" {
			t.Skip("sem MONGO_TEST_URI nem MONGO_ROOT_USER no .env: pulando integração")
		}
		port := os.Getenv("MONGO_PORT")
		if port == "" {
			port = "27017"
		}
		u := url.URL{
			Scheme:   "mongodb",
			User:     url.UserPassword(user, os.Getenv("MONGO_ROOT_PASSWORD")),
			Host:     "localhost:" + port,
			Path:     "/",
			RawQuery: "authSource=admin",
		}
		uri = u.String()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	dbName := fmt.Sprintf("sidm_test_%d", time.Now().UnixNano())
	s, err := New(ctx, uri, dbName)
	if err != nil {
		t.Skipf("Mongo indisponível (%v): pulando integração", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_ = s.db.Drop(bg)
		_ = s.Close(bg)
	})

	// Os mesmos índices únicos do mongo/init/01-init.js.
	for _, coll := range []string{"equipments", "cameras"} {
		_, err := s.db.Collection(coll).Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys:    bson.D{{Key: "name", Value: 1}},
			Options: options.Index().SetUnique(true),
		})
		if err != nil {
			t.Fatalf("criar índice em %s: %v", coll, err)
		}
	}
	return s
}

// ms: o Mongo guarda datas com precisão de MILISSEGUNDOS. Um time.Now() com
// nanossegundos volta diferente do banco; nos testes, truncamos antes.
func ms(t time.Time) time.Time { return t.UTC().Truncate(time.Millisecond) }

func TestNewURIInvalida(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if _, err := New(ctx, "http://nao-e-mongo", "x"); err == nil {
		t.Error("esquema inválido deveria falhar no Connect")
	}
	if _, err := New(ctx, "mongodb://localhost:1", "x"); err == nil {
		t.Error("porta sem Mongo deveria falhar no Ping")
	}
}

func TestPing(t *testing.T) {
	s := testStore(t)
	if err := s.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCadastro(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if err := s.CreateEquipment(ctx, "8003-MF4739"); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateEquipment(ctx, "1000-AAA"); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateEquipment(ctx, "8003-MF4739"); !errors.Is(err, ErrDuplicate) {
		t.Errorf("duplicado: err = %v, want ErrDuplicate", err)
	}

	// Um inativo, inserido direto, não pode aparecer na lista nem no Exists.
	_, err := s.db.Collection("equipments").InsertOne(ctx, bson.D{{Key: "name", Value: "9999-OLD"}, {Key: "active", Value: false}})
	if err != nil {
		t.Fatal(err)
	}

	names, err := s.ListEquipments(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(names) != "[1000-AAA 8003-MF4739]" {
		t.Errorf("ListEquipments = %v (esperava só ativos, em ordem alfabética)", names)
	}

	if ok, _ := s.EquipmentExists(ctx, "8003-MF4739"); !ok {
		t.Error("EquipmentExists(ativo) = false")
	}
	if ok, _ := s.EquipmentExists(ctx, "9999-OLD"); ok {
		t.Error("EquipmentExists(inativo) = true")
	}

	// Câmeras usam a mesma lógica em outra collection.
	if err := s.CreateCamera(ctx, "hikvision-233"); err != nil {
		t.Fatal(err)
	}
	cams, _ := s.ListCameras(ctx)
	if len(cams) != 1 || cams[0] != "hikvision-233" {
		t.Errorf("ListCameras = %v", cams)
	}
	if ok, _ := s.CameraExists(ctx, "8003-MF4739"); ok {
		t.Error("equipamento não pode ser encontrado como câmera")
	}
}

func TestListaVaziaNaoENil(t *testing.T) {
	s := testStore(t)
	names, err := s.ListCameras(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if names == nil {
		t.Error("lista vazia deve ser []string{}, não nil (senão o JSON sai null)")
	}
}

func newImage(sha string, ingested time.Time) *Image {
	return &Image{
		SHA1:         sha,
		Status:       StatusPending,
		OriginalName: sha + ".jpg",
		Path:         "staging/" + sha + ".jpg",
		Ext:          ".jpg",
		Size:         10,
		IngestedAt:   ms(ingested),
		UpdatedAt:    ms(ingested),
	}
}

func TestImagensCRUD(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	img := newImage("aaaa", time.Now())

	if err := s.InsertImage(ctx, img); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertImage(ctx, img); !errors.Is(err, ErrDuplicate) {
		t.Errorf("mesmo sha1 duas vezes: err = %v, want ErrDuplicate", err)
	}

	got, err := s.FindImage(ctx, "aaaa")
	if err != nil {
		t.Fatal(err)
	}
	if got.OriginalName != img.OriginalName || !got.IngestedAt.Equal(img.IngestedAt) {
		t.Errorf("FindImage = %+v", got)
	}
	if got.CapturedAt != nil {
		t.Error("pending sem data deveria voltar CapturedAt nil")
	}

	if _, err := s.FindImage(ctx, "zzzz"); !errors.Is(err, ErrNotFound) {
		t.Errorf("inexistente: err = %v, want ErrNotFound", err)
	}

	// Replace com anotação completa.
	captured := ms(time.Date(2026, 9, 3, 11, 25, 23, 0, time.UTC))
	got.Status = StatusReviewed
	got.DayTime = "night"
	got.CapturedAt = &captured
	got.Vehicles = []Vehicle{{Plate: "ABC1D23", PlateType: "mercosul", PlateCategory: "private", Type: "truck", Color: "white"}}
	if err := s.ReplaceImage(ctx, got); err != nil {
		t.Fatal(err)
	}

	again, _ := s.FindImage(ctx, "aaaa")
	if again.Status != StatusReviewed || again.DayTime != "night" || len(again.Vehicles) != 1 {
		t.Errorf("Replace não persistiu: %+v", again)
	}
	if again.CapturedAt == nil || !again.CapturedAt.Equal(captured) {
		t.Errorf("captured_at = %v, want %v", again.CapturedAt, captured)
	}

	if err := s.ReplaceImage(ctx, newImage("nao-existe", time.Now())); !errors.Is(err, ErrNotFound) {
		t.Errorf("Replace de inexistente: err = %v, want ErrNotFound", err)
	}

	if err := s.DeleteImage(ctx, "aaaa"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FindImage(ctx, "aaaa"); !errors.Is(err, ErrNotFound) {
		t.Error("imagem continuou existindo depois do Delete")
	}
}

func TestListImagesFiltros(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

	// A: noturna, um caminhão branco.
	a := newImage("a", base)
	a.Status, a.DayTime, a.Equipment = StatusReviewed, "night", "8003-MF4739"
	a.Vehicles = []Vehicle{{Plate: "AAA1111", Type: "truck", Color: "white"}}

	// B: noturna, caminhão PRETO + carro BRANCO (a pegadinha do $elemMatch).
	b := newImage("b", base.Add(time.Minute))
	b.Status, b.DayTime, b.Equipment = StatusReviewed, "night", "1000-AAA"
	b.Vehicles = []Vehicle{
		{Plate: "BBB2222", Type: "truck", Color: "black"},
		{Plate: "CCC3333", Type: "car", Color: "white"},
	}

	// C: pending, sem anotação.
	c := newImage("c", base.Add(2*time.Minute))

	for _, img := range []*Image{c, a, b} { // inseridas fora de ordem de propósito
		if err := s.InsertImage(ctx, img); err != nil {
			t.Fatal(err)
		}
	}

	cases := []struct {
		name   string
		filter ImageFilter
		want   string // sha1s esperados, na ordem
	}{
		{"sem filtro, mais antigas primeiro", ImageFilter{}, "[a b c]"},
		{"por status", ImageFilter{Status: StatusPending}, "[c]"},
		{"por período", ImageFilter{DayTime: "night"}, "[a b]"},
		{"por equipamento", ImageFilter{Equipment: "1000-AAA"}, "[b]"},
		{"caminhão branco ($elemMatch)", ImageFilter{VehicleType: "truck", VehicleColor: "white"}, "[a]"},
		{"só cor branca", ImageFilter{VehicleColor: "white"}, "[a b]"},
		{"por placa", ImageFilter{Plate: "CCC3333"}, "[b]"},
		{"filtros combinados sem resultado", ImageFilter{DayTime: "night", Status: StatusPending}, "[]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.filter.Limit = 50
			items, total, err := s.ListImages(ctx, tc.filter)
			if err != nil {
				t.Fatal(err)
			}
			if got := shas(items); got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
			if int(total) != len(items) {
				t.Errorf("total = %d, items = %d", total, len(items))
			}
		})
	}

	t.Run("paginação", func(t *testing.T) {
		items, total, err := s.ListImages(ctx, ImageFilter{Limit: 1, Skip: 1})
		if err != nil {
			t.Fatal(err)
		}
		if shas(items) != "[b]" || total != 3 {
			t.Errorf("página 2 de 1 = %s, total %d; want [b], 3", shas(items), total)
		}
	})

	t.Run("sem resultados devolve slice vazio", func(t *testing.T) {
		items, _, _ := s.ListImages(ctx, ImageFilter{Equipment: "nada", Limit: 10})
		if items == nil {
			t.Error("esperava []Image{}, veio nil")
		}
	})
}

func shas(items []Image) string {
	out := make([]string, len(items))
	for i, img := range items {
		out[i] = img.SHA1
	}
	return fmt.Sprint(out)
}

func TestExistingImages(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	s.InsertImage(ctx, newImage("x1", time.Now()))
	reviewed := newImage("x2", time.Now())
	reviewed.Status = StatusReviewed
	s.InsertImage(ctx, reviewed)

	found, err := s.ExistingImages(ctx, []string{"x1", "x2", "x3"})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 || found["x1"] != StatusPending || found["x2"] != StatusReviewed {
		t.Errorf("ExistingImages = %v", found)
	}
	if _, ok := found["x3"]; ok {
		t.Error("x3 não existe e apareceu no resultado")
	}
}

func TestInsertHistory(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if err := s.InsertHistory(ctx, nil); err != nil {
		t.Errorf("lista vazia deveria ser no-op, deu %v", err)
	}

	now := ms(time.Now())
	err := s.InsertHistory(ctx, []HistoryEntry{
		{ImageID: "a", Field: "day_time", Old: "day", New: "night", ChangedBy: "nathan", ChangedAt: now},
		{ImageID: "a", Field: "status", Old: "pending", New: "reviewed", ChangedBy: "nathan", ChangedAt: now},
	})
	if err != nil {
		t.Fatal(err)
	}

	n, err := s.db.Collection("image_history").CountDocuments(ctx, bson.D{{Key: "image_id", Value: "a"}})
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("histórico com %d entradas, want 2", n)
	}
}
