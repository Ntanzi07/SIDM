package images

import (
	"SIDM/internal/annotation"
	"SIDM/internal/catalog"
	"SIDM/internal/importer"
	"SIDM/internal/platform"
	"SIDM/internal/platform/mongotest"
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Testes de INTEGRAÇÃO do MongoRepo (ver platform/mongotest): sem Mongo, são pulados.

func testRepo(t *testing.T) *MongoRepo {
	t.Helper()
	return NewMongoRepo(mongotest.DB(t))
}

// ms: o Mongo guarda datas com precisão de MILISSEGUNDOS. Um time.Now() com
// nanossegundos volta diferente do banco; nos testes, truncamos antes.
func ms(t time.Time) time.Time { return t.UTC().Truncate(time.Millisecond) }

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

func veh(plate, typ, color string) annotation.Object {
	return annotation.Object{Type: "vehicle", Fields: map[string]any{"plate": plate, "type": typ, "color": color}}
}

func fieldStr(f map[string]any, k string) string {
	s, _ := f[k].(string)
	return s
}

func TestImagensCRUD(t *testing.T) {
	s := testRepo(t)
	ctx := context.Background()
	img := newImage("aaaa", time.Now())

	if err := s.InsertImage(ctx, img); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertImage(ctx, img); !errors.Is(err, platform.ErrDuplicate) {
		t.Errorf("mesmo sha1 duas vezes: err = %v, want platform.ErrDuplicate", err)
	}

	got, err := s.FindImage(ctx, "aaaa")
	if err != nil {
		t.Fatal(err)
	}
	if got.OriginalName != img.OriginalName || !got.IngestedAt.Equal(img.IngestedAt) {
		t.Errorf("FindImage = %+v", got)
	}
	if got.Fields == nil || len(got.Fields) != 0 || got.Objects == nil {
		t.Errorf("sem anotação, fields e objects voltam vazios (não nil): %+v %+v", got.Fields, got.Objects)
	}

	if _, err := s.FindImage(ctx, "zzzz"); !errors.Is(err, platform.ErrNotFound) {
		t.Errorf("inexistente: err = %v, want platform.ErrNotFound", err)
	}

	// Replace com anotação completa.
	captured := ms(time.Date(2026, 9, 3, 11, 25, 23, 0, time.UTC))
	got.Status = StatusVerified
	got.Fields = map[string]any{"day_time": "night", "captured_at": captured, "conditions": []string{"glare", "low_contrast"}, "riders": 2.0}
	got.Objects = []annotation.Object{veh("ABC1D23", "truck", "white")}
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
	if again.Status != StatusVerified || fieldStr(again.Fields, "day_time") != "night" || len(again.Objects) != 1 {
		t.Errorf("Replace não persistiu: %+v", again)
	}
	// O que volta do Mongo tem os mesmos tipos do que foi gravado (bson.A
	// vira []string, a data vira time.Time): o histórico não acusa mudança.
	if !reflect.DeepEqual(again.Fields, got.Fields) || !reflect.DeepEqual(again.Objects, got.Objects) {
		t.Errorf("voltou com outros tipos:\n got  %#v\n want %#v", again.Fields, got.Fields)
	}

	ghost := newImage("nao-existe", time.Now())
	if err := s.ReplaceImage(ctx, ghost, ghost.UpdatedAt); !errors.Is(err, platform.ErrNotFound) {
		t.Errorf("Replace de inexistente: err = %v, want platform.ErrNotFound", err)
	}

	if err := s.DeleteImage(ctx, "aaaa"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FindImage(ctx, "aaaa"); !errors.Is(err, platform.ErrNotFound) {
		t.Error("imagem continuou existindo depois do Delete")
	}
}

func TestListImagesFiltros(t *testing.T) {
	s := testRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

	// A: noturna, um caminhão branco.
	a := newImage("a", base)
	a.Status, a.Fields = StatusVerified, map[string]any{"day_time": "night", "equipment": "8003-MF4739", "conditions": []string{"glare"}, "riders": 2.0, "helmet": true}
	a.Objects = []annotation.Object{veh("AAA1111", "truck", "white")}

	// B: noturna, caminhão PRETO + carro BRANCO (a pegadinha do $elemMatch).
	b := newImage("b", base.Add(time.Minute))
	b.Status, b.Fields = StatusVerified, map[string]any{"day_time": "night", "equipment": "1000-AAA"}
	b.Objects = []annotation.Object{veh("BBB2222", "truck", "black"), veh("CCC3333", "car", "white")}

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
		{"por período", ImageFilter{Fields: map[string]any{"day_time": "night"}}, "[a b]"},
		{"por equipamento", ImageFilter{Fields: map[string]any{"equipment": "1000-AAA"}}, "[b]"},
		{"lista contém o valor", ImageFilter{Fields: map[string]any{"conditions": "glare"}}, "[a]"},
		{"caminhão branco ($elemMatch)", ImageFilter{Object: map[string]any{"type": "truck", "color": "white"}}, "[a]"},
		{"só cor branca", ImageFilter{Object: map[string]any{"color": "white"}}, "[a b]"},
		{"por placa", ImageFilter{Object: map[string]any{"plate": "CCC3333"}}, "[b]"},
		{"filtros combinados sem resultado", ImageFilter{Fields: map[string]any{"day_time": "night"}, Status: StatusPending}, "[]"},
		{"número", ImageFilter{Fields: map[string]any{"riders": 2.0}}, "[a]"},
		{"sim/não", ImageFilter{Fields: map[string]any{"helmet": true}}, "[a]"},
		{"objeto de um tipo", ImageFilter{Objects: []ObjectFilter{{Type: "vehicle", Fields: map[string]any{"color": "white"}}}}, "[a b]"},
		{"tipo de objeto que não tem", ImageFilter{Objects: []ObjectFilter{{Type: "rider", Fields: map[string]any{"color": "white"}}}}, "[]"},
		{"dois objetos ($and)", ImageFilter{Objects: []ObjectFilter{
			{Type: "vehicle", Fields: map[string]any{"type": "truck", "color": "black"}},
			{Type: "vehicle", Fields: map[string]any{"type": "car"}},
		}}, "[b]"},
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
		items, _, _ := s.ListImages(ctx, ImageFilter{Fields: map[string]any{"equipment": "nada"}, Limit: 10})
		if items == nil {
			t.Error("esperava []Image{}, veio nil")
		}
	})
}

func TestExistingImages(t *testing.T) {
	s := testRepo(t)
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
	s := testRepo(t)
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

func TestPackageProgress(t *testing.T) {
	s := testRepo(t)
	ctx := context.Background()

	// antigo: 2 verificadas + 1 pendente. novo: 1 pendente. vazio: nada.
	for i, st := range []string{StatusVerified, StatusVerified, StatusPending} {
		img := newImage(fmt.Sprintf("antigo-%d", i), time.Now())
		img.Package, img.Status = "antigo", st
		s.InsertImage(ctx, img)
	}
	img := newImage("novo-0", time.Now())
	img.Package = "novo"
	s.InsertImage(ctx, img)
	outro := newImage("outro-0", time.Now())
	outro.Package = "outro" // pacote não pedido: não entra
	s.InsertImage(ctx, outro)

	got, err := s.PackageProgress(ctx, []string{"antigo", "novo", "vazio"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]catalog.Progress{
		"antigo": {Total: 3, Verified: 2, Pending: 1},
		"novo":   {Total: 1, Pending: 1},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("PackageProgress = %v, want %v", got, want)
	}
}

func TestClaimNextOrdemPularERecarregar(t *testing.T) {
	s := testRepo(t)
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
		if errors.Is(err, platform.ErrNotFound) {
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
	s := testRepo(t)
	ctx := context.Background()

	other := newImage("outro", time.Now())
	other.Package = "pacote-2"
	done := newImage("feita", time.Now())
	done.Status = StatusVerified
	s.InsertImage(ctx, other)
	s.InsertImage(ctx, done)

	if _, err := s.ClaimNext(ctx, "pacote-1", "nathan", "", time.Minute, time.Now()); !errors.Is(err, platform.ErrNotFound) {
		t.Errorf("só há imagem de outro pacote e verificada: err = %v, want platform.ErrNotFound", err)
	}
}

// O teste mais importante da fila: muita gente clicando "próxima" AO MESMO
// TEMPO nunca pode receber a mesma imagem.
func TestClaimNextConcorrente(t *testing.T) {
	s := testRepo(t)
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
		case errors.Is(r.err, platform.ErrNotFound):
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
	s := testRepo(t)
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

func TestPackageProgressInProgress(t *testing.T) {
	s := testRepo(t)
	ctx := context.Background()
	for _, id := range []string{"p1", "p2", "p3"} {
		s.InsertImage(ctx, newImage(id, time.Now()))
	}
	s.ClaimNext(ctx, "pacote-1", "nathan", "", time.Hour, time.Now())
	s.ClaimNext(ctx, "pacote-1", "maria", "", time.Hour, time.Now())
	s.ClaimNext(ctx, "pacote-1", "joao", "", -time.Minute, time.Now()) // já nasce vencido

	got, err := s.PackageProgress(ctx, []string{"pacote-1"})
	if err != nil {
		t.Fatal(err)
	}
	if p := got["pacote-1"]; p.InProgress != 2 || p.Pending != 3 {
		t.Errorf("in_progress=%d pending=%d; want 2 e 3", p.InProgress, p.Pending)
	}
}

func TestFindImagesByKeys(t *testing.T) {
	s := testRepo(t)
	ctx := context.Background()

	a := newImage("sha-a", time.Now())
	a.OriginalName = "foto.jpg"
	b := newImage("sha-b", time.Now())
	b.OriginalName = "foto.jpg" // mesmo nome, outra imagem
	c := newImage("sha-c", time.Now())
	c.OriginalName = "outra.jpg"
	d := newImage("sha-d", time.Now())
	d.OriginalName = "foto.jpg"
	d.Package = "pacote-2" // outro pacote: não pode aparecer
	for _, img := range []*Image{a, b, c, d} {
		s.InsertImage(ctx, img)
	}

	byName, err := s.FindImagesByKeys(ctx, "pacote-1", importer.MatchByOriginalName, []string{"foto.jpg", "nada.jpg"})
	if err != nil {
		t.Fatal(err)
	}
	if shas(byName) != "[sha-a sha-b]" && shas(byName) != "[sha-b sha-a]" {
		t.Errorf("por nome = %s, want sha-a e sha-b (só do pacote-1)", shas(byName))
	}

	byHash, _ := s.FindImagesByKeys(ctx, "pacote-1", importer.MatchBySHA1, []string{"sha-c", "sha-d"})
	if shas(byHash) != "[sha-c]" {
		t.Errorf("por hash = %s, want só sha-c (sha-d é de outro pacote)", shas(byHash))
	}

	// Mais de 1000 chaves: tem que funcionar em lotes.
	many := make([]string, 2500)
	for i := range many {
		many[i] = fmt.Sprintf("x%d.jpg", i)
	}
	many[2400] = "outra.jpg"
	got, err := s.FindImagesByKeys(ctx, "pacote-1", importer.MatchByOriginalName, many)
	if err != nil || shas(got) != "[sha-c]" {
		t.Errorf("2500 chaves: %s, %v", shas(got), err)
	}
}

func TestImagemComINI(t *testing.T) {
	s := testRepo(t)
	ctx := context.Background()

	img := newImage("com-ini", time.Now())
	img.INI = []importer.INIEntry{
		{Section: "Metrologico", Key: "Data", Value: "18/09/2026"},
		{Section: "NMetrologico", Key: "Placa", Value: "GGB5H24"},
		{Section: "NMetrologico", Key: "Chave.Com.Ponto", Value: "$valor"}, // nada disso quebra no Mongo
	}
	if err := s.InsertImage(ctx, img); err != nil {
		t.Fatal(err)
	}

	got, err := s.FindImage(ctx, "com-ini")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.INI) != 3 || got.INI[1] != img.INI[1] || got.INI[2] != img.INI[2] {
		t.Errorf("INI voltou diferente (ou fora de ordem): %+v", got.INI)
	}

	// Sem INI, o campo nem aparece no documento (omitempty).
	s.InsertImage(ctx, newImage("sem-ini", time.Now()))
	n, _ := s.db.Collection("images").CountDocuments(ctx, bson.D{{Key: "ini", Value: bson.D{{Key: "$exists", Value: true}}}})
	if n != 1 {
		t.Errorf("%d documentos com o campo ini, want 1", n)
	}
}

func TestDeleteImageHistory(t *testing.T) {
	s := testRepo(t)
	ctx := context.Background()
	now := ms(time.Now())
	s.InsertHistory(ctx, []HistoryEntry{
		{ImageID: "a", Field: "status", New: "verified", ChangedBy: "n", ChangedAt: now},
		{ImageID: "a", Field: "day_time", New: "night", ChangedBy: "n", ChangedAt: now},
		{ImageID: "b", Field: "status", New: "verified", ChangedBy: "n", ChangedAt: now},
	})
	if err := s.DeleteImageHistory(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	hist := s.db.Collection("image_history")
	if n, _ := hist.CountDocuments(ctx, bson.D{{Key: "image_id", Value: "a"}}); n != 0 {
		t.Errorf("sobraram %d entradas da imagem apagada", n)
	}
	if n, _ := hist.CountDocuments(ctx, bson.D{{Key: "image_id", Value: "b"}}); n != 1 {
		t.Error("o histórico de outra imagem não pode ser apagado")
	}
}

func TestCountUsingERenameOnPending(t *testing.T) {
	s := testRepo(t)
	ctx := context.Background()

	// 2 pendentes e 1 verificada com o equipamento "velho"; 1 pendente com outro.
	for i, st := range []string{StatusPending, StatusPending, StatusVerified} {
		img := newImage(fmt.Sprintf("img-%d", i), time.Now())
		img.Status, img.Fields = st, map[string]any{"equipment": "velho", "camera": "cam-x"}
		if err := s.InsertImage(ctx, img); err != nil {
			t.Fatal(err)
		}
	}
	other := newImage("img-outro", time.Now())
	other.Fields = map[string]any{"equipment": "outro"}
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
		if fieldStr(img.Fields, "equipment") != "novo" || !img.UpdatedAt.Equal(now) {
			t.Errorf("%s: equipment=%v updated_at=%v", id, img.Fields["equipment"], img.UpdatedAt)
		}
		if fieldStr(img.Fields, "camera") != "cam-x" {
			t.Error("renomear o equipamento não pode mexer na câmera")
		}
	}
	if v, _ := s.FindImage(ctx, "img-2"); fieldStr(v.Fields, "equipment") != "velho" {
		t.Error("a verificada é renomeada pela API (arquivo + banco), não aqui")
	}
	if o, _ := s.FindImage(ctx, "img-outro"); fieldStr(o.Fields, "equipment") != "outro" {
		t.Error("imagem de outro equipamento foi alterada")
	}
}

func TestRenamePackageImages(t *testing.T) {
	s := testRepo(t)
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
	want := map[string]any{
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
	s := testRepo(t)
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
		if _, err := s.FindImage(ctx, id); !errors.Is(err, platform.ErrNotFound) {
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

func TestVerifiedIDs(t *testing.T) {
	s := testRepo(t)
	ctx := context.Background()
	for i, st := range []string{StatusVerified, StatusPending, StatusVerified} {
		img := newImage(fmt.Sprintf("img-%d", i), time.Now())
		img.Status = st
		if err := s.InsertImage(ctx, img); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.VerifiedIDs(ctx, []string{"img-0", "img-1", "img-2", "nao-existe"})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(got) != "map[img-0:true img-2:true]" {
		t.Errorf("VerifiedIDs = %v", got)
	}
}

func shas(items []Image) string {
	out := make([]string, len(items))
	for i, img := range items {
		out[i] = img.SHA1
	}
	return fmt.Sprint(out)
}

// Uma imagem de antes dos perfis (campos soltos e "vehicles") vai para
// fields/objects, sem os vazios; rodar de novo não muda nada.
func TestMigrate(t *testing.T) {
	s := testRepo(t)
	ctx := context.Background()
	captured := time.Date(2026, 9, 3, 11, 25, 23, 0, time.UTC)
	coll := s.db.Collection("images")
	_, err := coll.InsertMany(ctx, []any{
		bson.D{
			{Key: "_id", Value: "velha"}, {Key: "package", Value: "p"}, {Key: "status", Value: StatusVerified},
			{Key: "equipment", Value: "8003-MF4739"}, {Key: "camera", Value: ""}, {Key: "day_time", Value: "night"},
			{Key: "conditions", Value: bson.A{"glare"}}, {Key: "captured_at", Value: captured},
			{Key: "vehicles", Value: bson.A{
				bson.D{{Key: "plate", Value: "ABC1D23"}, {Key: "plate_type", Value: "mercosul"}, {Key: "plate_category", Value: ""}, {Key: "type", Value: "car"}, {Key: "color", Value: "white"}},
				bson.D{{Key: "plate", Value: ""}, {Key: "type", Value: "truck"}},
			}},
		},
		bson.D{{Key: "_id", Value: "vazia"}, {Key: "package", Value: "p"}, {Key: "status", Value: StatusPending}, {Key: "conditions", Value: bson.A{}}},
		bson.D{{Key: "_id", Value: "nova"}, {Key: "package", Value: "p"}, {Key: "status", Value: StatusPending},
			{Key: "fields", Value: bson.D{{Key: "riders", Value: 2.0}}}},
	})
	if err != nil {
		t.Fatal(err)
	}

	n, err := s.Migrate(ctx)
	if err != nil || n != 2 {
		t.Fatalf("Migrate = %d, %v; want 2 (a nova já tem fields)", n, err)
	}

	old, _ := s.FindImage(ctx, "velha")
	want := map[string]any{"equipment": "8003-MF4739", "day_time": "night", "conditions": []string{"glare"}, "captured_at": captured}
	if !reflect.DeepEqual(old.Fields, want) {
		t.Errorf("fields = %#v\nwant     %#v", old.Fields, want)
	}
	wantObjs := []annotation.Object{
		{Type: "vehicle", Fields: map[string]any{"plate": "ABC1D23", "plate_type": "mercosul", "type": "car", "color": "white"}},
		{Type: "vehicle", Fields: map[string]any{"type": "truck"}},
	}
	if !reflect.DeepEqual(old.Objects, wantObjs) {
		t.Errorf("objects = %#v", old.Objects)
	}
	var raw bson.M
	coll.FindOne(ctx, bson.D{{Key: "_id", Value: "velha"}}).Decode(&raw)
	for _, k := range []string{"equipment", "camera", "day_time", "conditions", "captured_at", "vehicles"} {
		if _, ok := raw[k]; ok {
			t.Errorf("o campo antigo %q continuou no documento", k)
		}
	}

	empty, _ := s.FindImage(ctx, "vazia")
	if len(empty.Fields) != 0 || len(empty.Objects) != 0 {
		t.Errorf("vazia = %+v %+v", empty.Fields, empty.Objects)
	}
	if nova, _ := s.FindImage(ctx, "nova"); nova.Fields["riders"] != 2.0 {
		t.Errorf("a que já estava no formato novo mudou: %+v", nova.Fields)
	}

	if n, err := s.Migrate(ctx); err != nil || n != 0 {
		t.Errorf("segunda vez = %d, %v; want 0", n, err)
	}
	if err := s.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
}
