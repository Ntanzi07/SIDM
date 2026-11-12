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
	for _, coll := range []string{"equipments", "cameras", "packages"} {
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
		Package:      "pacote-1",
		Status:       StatusPending,
		OriginalName: sha + ".jpg",
		Path:         "dataset/pacote-1/" + sha + ".jpg",
		Ext:          ".jpg",
		Size:         10,
		QueuedAt:     ms(ingested),
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
	got.Status = StatusVerified
	got.DayTime = "night"
	got.CapturedAt = &captured
	got.Vehicles = []Vehicle{{Plate: "ABC1D23", PlateType: "mercosul", PlateCategory: "private", Type: "truck", Color: "white"}}
	prev := got.UpdatedAt
	got.UpdatedAt = ms(time.Now().Add(time.Second))
	if err := s.ReplaceImage(ctx, got, prev); err != nil {
		t.Fatal(err)
	}

	// Gravar de novo com o updated_at ANTIGO: alguém já gravou no meio.
	if err := s.ReplaceImage(ctx, got, prev); !errors.Is(err, ErrConflict) {
		t.Errorf("updated_at desatualizado: err = %v, want ErrConflict", err)
	}

	again, _ := s.FindImage(ctx, "aaaa")
	if again.Status != StatusVerified || again.DayTime != "night" || len(again.Vehicles) != 1 {
		t.Errorf("Replace não persistiu: %+v", again)
	}
	if again.CapturedAt == nil || !again.CapturedAt.Equal(captured) {
		t.Errorf("captured_at = %v, want %v", again.CapturedAt, captured)
	}

	ghost := newImage("nao-existe", time.Now())
	if err := s.ReplaceImage(ctx, ghost, ghost.UpdatedAt); !errors.Is(err, ErrNotFound) {
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
	a.Status, a.DayTime, a.Equipment = StatusVerified, "night", "8003-MF4739"
	a.Vehicles = []Vehicle{{Plate: "AAA1111", Type: "truck", Color: "white"}}

	// B: noturna, caminhão PRETO + carro BRANCO (a pegadinha do $elemMatch).
	b := newImage("b", base.Add(time.Minute))
	b.Status, b.DayTime, b.Equipment = StatusVerified, "night", "1000-AAA"
	b.Vehicles = []Vehicle{
		{Plate: "BBB2222", Type: "truck", Color: "black"},
		{Plate: "CCC3333", Type: "car", Color: "white"},
	}

	// C: pending, sem anotação, em outro pacote.
	c := newImage("c", base.Add(2*time.Minute))
	c.Package = "pacote-2"

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
		{"por pacote", ImageFilter{Package: "pacote-1"}, "[a b]"},
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
	verified := newImage("x2", time.Now())
	verified.Status = StatusVerified
	verified.Package = "outro-pacote"
	s.InsertImage(ctx, verified)

	found, err := s.ExistingImages(ctx, []string{"x1", "x2", "x3"})
	if err != nil {
		t.Fatal(err)
	}
	want1 := ExistingImage{Package: "pacote-1", Status: StatusPending}
	want2 := ExistingImage{Package: "outro-pacote", Status: StatusVerified}
	if len(found) != 2 || found["x1"] != want1 || found["x2"] != want2 {
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
		{ImageID: "a", Field: "status", Old: "pending", New: "verified", ChangedBy: "nathan", ChangedAt: now},
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

func TestPacotes(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if err := s.CreatePackage(ctx, "antigo", "nathan"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond) // created_at diferente para testar a ordem
	if err := s.CreatePackage(ctx, "novo", "maria"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if err := s.CreatePackage(ctx, "vazio", "nathan"); err != nil {
		t.Fatal(err)
	}
	if err := s.CreatePackage(ctx, "antigo", "outro"); !errors.Is(err, ErrDuplicate) {
		t.Errorf("duplicado: err = %v, want ErrDuplicate", err)
	}

	p, err := s.FindPackage(ctx, "novo")
	if err != nil || p.CreatedBy != "maria" {
		t.Errorf("FindPackage = %+v, %v", p, err)
	}
	if _, err := s.FindPackage(ctx, "nao-existe"); !errors.Is(err, ErrNotFound) {
		t.Errorf("inexistente: err = %v, want ErrNotFound", err)
	}

	// antigo: 2 verificadas + 1 pendente. novo: 1 pendente. vazio: nada.
	for i, st := range []string{StatusVerified, StatusVerified, StatusPending} {
		img := newImage(fmt.Sprintf("antigo-%d", i), time.Now())
		img.Package, img.Status = "antigo", st
		s.InsertImage(ctx, img)
	}
	img := newImage("novo-0", time.Now())
	img.Package = "novo"
	s.InsertImage(ctx, img)

	pkgs, err := s.ListPackages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]PackageSummary{}
	var order []string
	for _, p := range pkgs {
		got[p.Name] = p
		order = append(order, p.Name)
	}

	if fmt.Sprint(order) != "[vazio novo antigo]" {
		t.Errorf("ordem = %v, want mais novos primeiro", order)
	}
	check := func(name string, total, verified, pending int64) {
		t.Helper()
		p := got[name]
		if p.Total != total || p.Verified != verified || p.Pending != pending {
			t.Errorf("%s: total=%d verified=%d pending=%d; want %d/%d/%d",
				name, p.Total, p.Verified, p.Pending, total, verified, pending)
		}
	}
	check("antigo", 3, 2, 1)
	check("novo", 1, 0, 1)
	check("vazio", 0, 0, 0)

	if got["antigo"].CreatedBy != "nathan" {
		t.Error("o resumo deveria trazer os dados do pacote (inline)")
	}
}

func TestClaimNextOrdemPularERecarregar(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for i, id := range []string{"q1", "q2", "q3"} {
		s.InsertImage(ctx, newImage(id, base.Add(time.Duration(i)*time.Minute)))
	}
	now := ms(time.Now())
	lease := 15 * time.Minute

	next := func(user, skip string) string {
		t.Helper()
		img, err := s.ClaimNext(ctx, "pacote-1", user, skip, lease, now)
		if errors.Is(err, ErrNotFound) {
			return "<vazia>"
		}
		if err != nil {
			t.Fatal(err)
		}
		if img.ClaimedBy != user || img.ClaimedUntil == nil || !img.ClaimedUntil.Equal(now.Add(lease)) {
			t.Errorf("imagem %s não veio reservada para %s: %+v", img.SHA1, user, img)
		}
		return img.SHA1
	}

	if got := next("nathan", ""); got != "q1" {
		t.Errorf("primeira = %s, want q1", got)
	}
	if got := next("nathan", ""); got != "q1" {
		t.Errorf("recarregar = %s, want q1 de novo", got)
	}
	if got := next("maria", ""); got != "q2" {
		t.Errorf("maria = %s, want q2 (q1 está com nathan)", got)
	}
	if got := next("nathan", "q1"); got != "q3" {
		t.Errorf("nathan pulando q1 = %s, want q3", got)
	}
	if got := next("joao", ""); got != "q1" {
		t.Errorf("joao = %s, want q1 (pulada, voltou livre no fim da fila)", got)
	}
	if got := next("ana", ""); got != "<vazia>" {
		t.Errorf("ana = %s, want fila vazia (todas reservadas)", got)
	}

	// Prazo vencido: a imagem volta para a fila.
	later := now.Add(lease + time.Second)
	img, err := s.ClaimNext(ctx, "pacote-1", "ana", "", lease, later)
	if err != nil {
		t.Fatalf("com prazos vencidos, ana deveria receber uma imagem: %v", err)
	}
	if img.ClaimedBy != "ana" {
		t.Errorf("claimed_by = %q", img.ClaimedBy)
	}
}

func TestClaimNextOutroPacoteEVerificadas(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	other := newImage("outro", time.Now())
	other.Package = "pacote-2"
	done := newImage("feita", time.Now())
	done.Status = StatusVerified
	s.InsertImage(ctx, other)
	s.InsertImage(ctx, done)

	if _, err := s.ClaimNext(ctx, "pacote-1", "nathan", "", time.Minute, time.Now()); !errors.Is(err, ErrNotFound) {
		t.Errorf("só há imagem de outro pacote e verificada: err = %v, want ErrNotFound", err)
	}
}

// O teste mais importante da fila: muita gente clicando "próxima" AO MESMO
// TEMPO nunca pode receber a mesma imagem.
func TestClaimNextConcorrente(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	const images, users = 5, 20

	for i := 0; i < images; i++ {
		s.InsertImage(ctx, newImage(fmt.Sprintf("c%d", i), time.Now()))
	}

	type result struct {
		sha string
		err error
	}
	results := make(chan result, users)
	start := make(chan struct{})
	for u := 0; u < users; u++ {
		go func(user string) {
			<-start // todos largam juntos
			img, err := s.ClaimNext(ctx, "pacote-1", user, "", time.Minute, time.Now())
			if err != nil {
				results <- result{err: err}
				return
			}
			results <- result{sha: img.SHA1}
		}(fmt.Sprintf("user-%d", u))
	}
	close(start)

	seen := map[string]bool{}
	empty := 0
	for u := 0; u < users; u++ {
		r := <-results
		switch {
		case errors.Is(r.err, ErrNotFound):
			empty++
		case r.err != nil:
			t.Fatal(r.err)
		case seen[r.sha]:
			t.Fatalf("a imagem %s foi entregue para duas pessoas", r.sha)
		default:
			seen[r.sha] = true
		}
	}
	if len(seen) != images || empty != users-images {
		t.Errorf("%d imagens entregues e %d filas vazias; want %d e %d", len(seen), empty, images, users-images)
	}
}

func TestReleaseClaim(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	s.InsertImage(ctx, newImage("r1", time.Now()))
	s.ClaimNext(ctx, "pacote-1", "nathan", "", time.Hour, time.Now())

	s.ReleaseClaim(ctx, "r1", "maria") // não é dela: não pode liberar
	if img, _ := s.FindImage(ctx, "r1"); img.ClaimedBy != "nathan" {
		t.Fatal("maria liberou a imagem do nathan")
	}

	if err := s.ReleaseClaim(ctx, "r1", "nathan"); err != nil {
		t.Fatal(err)
	}
	img, _ := s.FindImage(ctx, "r1")
	if img.ClaimedBy != "" || img.ClaimedUntil != nil {
		t.Errorf("release não limpou: %+v", img)
	}
}

func TestPacotesInProgress(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	s.CreatePackage(ctx, "pacote-1", "nathan")
	for _, id := range []string{"p1", "p2", "p3"} {
		s.InsertImage(ctx, newImage(id, time.Now()))
	}
	s.ClaimNext(ctx, "pacote-1", "nathan", "", time.Hour, time.Now())
	s.ClaimNext(ctx, "pacote-1", "maria", "", time.Hour, time.Now())
	s.ClaimNext(ctx, "pacote-1", "joao", "", -time.Minute, time.Now()) // já nasce vencido

	pkgs, err := s.ListPackages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if pkgs[0].InProgress != 2 || pkgs[0].Pending != 3 {
		t.Errorf("in_progress=%d pending=%d; want 2 e 3", pkgs[0].InProgress, pkgs[0].Pending)
	}
}
