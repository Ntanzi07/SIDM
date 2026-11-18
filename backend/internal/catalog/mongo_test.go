package catalog

// Testes de INTEGRAÇÃO do MongoRepo (ver platform/mongotest): sem Mongo, são pulados.

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"SIDM/internal/platform"
	"SIDM/internal/platform/mongotest"
)

func testRepo(t *testing.T) *MongoRepo {
	t.Helper()
	return NewMongoRepo(mongotest.DB(t))
}

func pkg(name, by string) Package {
	return Package{Name: name, CreatedBy: by, CreatedAt: time.Now().UTC().Truncate(time.Millisecond)}
}

func TestMongoPacotes(t *testing.T) {
	r := testRepo(t)
	ctx := context.Background()

	for _, nb := range [][2]string{{"antigo", "nathan"}, {"novo", "maria"}, {"vazio", "nathan"}} {
		// a data é montada AQUI, depois da pausa: created_at diferente para testar a ordem
		if err := r.CreatePackage(ctx, pkg(nb[0], nb[1])); err != nil {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := r.CreatePackage(ctx, pkg("antigo", "outro")); !errors.Is(err, platform.ErrDuplicate) {
		t.Errorf("duplicado: err = %v, want ErrDuplicate", err)
	}
	if p, err := r.FindPackage(ctx, "novo"); err != nil || p.CreatedBy != "maria" {
		t.Errorf("FindPackage = %+v, %v", p, err)
	}
	if _, err := r.FindPackage(ctx, "nao-existe"); !errors.Is(err, platform.ErrNotFound) {
		t.Errorf("inexistente: err = %v, want ErrNotFound", err)
	}

	pkgs, total, err := r.ListPackages(ctx, "", 10, 0)
	if err != nil || total != 3 {
		t.Fatalf("ListPackages: %d, %v", total, err)
	}
	var order []string
	for _, p := range pkgs {
		order = append(order, p.Name)
	}
	if fmt.Sprint(order) != "[vazio novo antigo]" {
		t.Errorf("ordem = %v, want mais novos primeiro", order)
	}

	if err := r.RenamePackage(ctx, "antigo", "novo"); !errors.Is(err, platform.ErrDuplicate) {
		t.Errorf("renomear para existente: %v", err)
	}
	if err := r.RenamePackage(ctx, "nao-existe", "x"); !errors.Is(err, platform.ErrNotFound) {
		t.Errorf("renomear inexistente: %v", err)
	}
	if err := r.RenamePackage(ctx, "antigo", "renomeado"); err != nil {
		t.Fatal(err)
	}
	if p, err := r.FindPackage(ctx, "renomeado"); err != nil || p.CreatedBy != "nathan" {
		t.Errorf("renomeado = %+v, %v", p, err)
	}
	if err := r.DeletePackage(ctx, "renomeado"); err != nil {
		t.Fatal(err)
	}
	if err := r.DeletePackage(ctx, "renomeado"); !errors.Is(err, platform.ErrNotFound) {
		t.Errorf("apagar duas vezes: %v", err)
	}
}

func TestMongoListPackagesBuscaEPaginacao(t *testing.T) {
	r := testRepo(t)
	ctx := context.Background()
	for _, n := range []string{"Noturnas-Setembro", "Diurnas-2026", "Noturnas-Outubro"} {
		r.CreatePackage(ctx, pkg(n, "nathan"))
		time.Sleep(5 * time.Millisecond)
	}

	pkgs, total, err := r.ListPackages(ctx, "noturnas", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(pkgs) != 2 || pkgs[0].Name != "Noturnas-Outubro" {
		t.Errorf("busca: total=%d %+v; want 2, mais novo primeiro", total, pkgs)
	}
	pkgs, total, _ = r.ListPackages(ctx, "", 1, 1)
	if total != 3 || len(pkgs) != 1 || pkgs[0].Name != "Diurnas-2026" {
		t.Errorf("página 2 de 1: total=%d %+v", total, pkgs)
	}
	pkgs, total, _ = r.ListPackages(ctx, "nada", 10, 0)
	if total != 0 || pkgs == nil {
		t.Errorf("sem resultados: total=%d pkgs=%v (deve ser [] e não nil)", total, pkgs)
	}
}

func TestMongoCadastro(t *testing.T) {
	r := testRepo(t)
	ctx := context.Background()

	if err := r.Create(ctx, Equipments, "8003-MF4739"); err != nil {
		t.Fatal(err)
	}
	if err := r.Create(ctx, Equipments, "1000-AAA"); err != nil {
		t.Fatal(err)
	}
	if err := r.Create(ctx, Equipments, "8003-MF4739"); !errors.Is(err, platform.ErrDuplicate) {
		t.Errorf("duplicado: err = %v, want ErrDuplicate", err)
	}
	// Um inativo, inserido direto, não pode aparecer na lista nem no Exists.
	if _, err := r.db.Collection("equipments").InsertOne(ctx, bson.D{{Key: "name", Value: "9999-OLD"}, {Key: "active", Value: false}}); err != nil {
		t.Fatal(err)
	}

	names, total, err := r.Search(ctx, Equipments, "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(names) != "[1000-AAA 8003-MF4739]" || total != 2 {
		t.Errorf("Search = %v (total %d); esperava só ativos, em ordem alfabética", names, total)
	}
	if ok, _ := r.ActiveExists(ctx, Equipments, "8003-MF4739"); !ok {
		t.Error("ActiveExists(ativo) = false")
	}
	if ok, _ := r.ActiveExists(ctx, Equipments, "9999-OLD"); ok {
		t.Error("ActiveExists(inativo) = true")
	}

	// Câmeras usam a mesma lógica em outra coleção.
	if err := r.Create(ctx, Cameras, "hikvision-233"); err != nil {
		t.Fatal(err)
	}
	if cams, _, _ := r.Search(ctx, Cameras, "", 10, 0); fmt.Sprint(cams) != "[hikvision-233]" {
		t.Errorf("Search(câmeras) = %v", cams)
	}
	if ok, _ := r.ActiveExists(ctx, Cameras, "8003-MF4739"); ok {
		t.Error("equipamento não pode ser encontrado como câmera")
	}
	// lista vazia é [] e não nil (senão o JSON sai null)
	if names, _, _ := r.Search(ctx, Cameras, "nada", 10, 0); names == nil {
		t.Error("lista vazia deve ser []string{}, não nil")
	}
}

func TestMongoAtivarDesativarApagar(t *testing.T) {
	r := testRepo(t)
	ctx := context.Background()
	for _, reg := range []Registry{Equipments, Cameras} {
		t.Run(string(reg), func(t *testing.T) {
			if err := r.Create(ctx, reg, "item-1"); err != nil {
				t.Fatal(err)
			}
			e, err := r.Find(ctx, reg, "item-1")
			if err != nil || !e.Active || e.Name != "item-1" {
				t.Fatalf("Find = %+v, %v", e, err)
			}
			if _, err := r.Find(ctx, reg, "nao-existe"); !errors.Is(err, platform.ErrNotFound) {
				t.Errorf("inexistente: err = %v", err)
			}
			if err := r.SetActive(ctx, reg, "item-1", false); err != nil {
				t.Fatal(err)
			}
			if e, _ := r.Find(ctx, reg, "item-1"); e.Active {
				t.Error("deveria estar desativado")
			}
			if err := r.SetActive(ctx, reg, "nao-existe", true); !errors.Is(err, platform.ErrNotFound) {
				t.Errorf("ativar inexistente: err = %v", err)
			}
			if err := r.Delete(ctx, reg, "item-1"); err != nil {
				t.Fatal(err)
			}
			if _, err := r.Find(ctx, reg, "item-1"); !errors.Is(err, platform.ErrNotFound) {
				t.Error("deveria ter sido apagado")
			}
			if err := r.Delete(ctx, reg, "item-1"); !errors.Is(err, platform.ErrNotFound) {
				t.Errorf("apagar duas vezes: err = %v", err)
			}
		})
	}
}

func TestMongoSearch(t *testing.T) {
	r := testRepo(t)
	ctx := context.Background()
	for _, n := range []string{"8003-MF4739", "8005-MF4700", "9000-ABC", "7000-mf9999", "1.2-ponto"} {
		if err := r.Create(ctx, Equipments, n); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		q         string
		limit     int64
		skip      int64
		want      string
		wantTotal int64
	}{
		{"mf47", 10, 0, "[8003-MF4739 8005-MF4700]", 2}, // contém, no meio do nome
		{"MF9", 10, 0, "[7000-mf9999]", 1},              // sem diferenciar maiúscula
		{"", 2, 0, "[1.2-ponto 7000-mf9999]", 5},        // sem busca, paginado
		{"", 2, 4, "[9000-ABC]", 5},                     // última página
		{"xyz", 10, 0, "[]", 0},                         // nada
		{".", 10, 0, "[1.2-ponto]", 1},                  // "." é literal, não "qualquer caractere"
		{".*", 10, 0, "[]", 0},                          // regex do usuário é escapada
		{"(a+)+$", 10, 0, "[]", 0},                      // nem regex maliciosa passa
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("q=%q limit=%d skip=%d", tc.q, tc.limit, tc.skip), func(t *testing.T) {
			names, total, err := r.Search(ctx, Equipments, tc.q, tc.limit, tc.skip)
			if err != nil {
				t.Fatal(err)
			}
			if fmt.Sprint(names) != tc.want || total != tc.wantTotal {
				t.Errorf("got %v (total %d), want %s (total %d)", names, total, tc.want, tc.wantTotal)
			}
		})
	}
}
