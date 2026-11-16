package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"SIDM/internal/store"
)

// completeBody gera uma anotação completa e válida; mods altera campos.
func completeBody(mods ...func(map[string]any)) string {
	b := map[string]any{
		"equipment":     "8003-MF4739",
		"camera":        "hikvision-233",
		"direction":     "front",
		"resolution":    "1920x1080",
		"capture_angle": "lateral",
		"day_time":      "night",
		"image_range":   "narrow",
		"weather":       "rain",
		"conditions":    []string{"glare", "low_contrast"},
		"captured_at":   "2026-09-03T08:25:23-03:00",
		"vehicles": []map[string]string{{
			"plate": "abc1d23", "plate_type": "mercosul", "plate_category": "private",
			"type": "truck", "color": "white",
		}},
		"complete": true,
	}
	for _, m := range mods {
		m(b)
	}
	out, _ := json.Marshal(b)
	return string(out)
}

func TestListImages(t *testing.T) {
	env := newTestEnv(t)
	env.seedPending(t, "um")
	env.seedPending(t, "dois")

	rec := env.do("GET", "/images?package="+testPkg+"&status=pending&equipment=8003-MF4739&plate=abc1d23&limit=9999&skip=-5", nil, nil)
	assertStatus(t, rec, http.StatusOK)
	full := env.db.lastFilter

	// Aqui só interessa que os filtros CHEGAM no banco (ver lastFilter abaixo).
	// A contagem é conferida num pedido só com pacote e status:
	var resp struct {
		Total int64         `json:"total"`
		Items []store.Image `json:"items"`
	}
	decode(t, env.do("GET", "/images?package="+testPkg+"&status=pending", nil, nil), &resp)
	if resp.Total != 2 || len(resp.Items) != 2 {
		t.Errorf("total=%d items=%d", resp.Total, len(resp.Items))
	}
	_ = rec

	f := full
	if f.Package != testPkg || f.Status != "pending" || f.Equipment != "8003-MF4739" {
		t.Errorf("filtro não repassado: %+v", f)
	}
	if f.Plate != "ABC1D23" {
		t.Errorf("placa deveria ir em maiúscula: %q", f.Plate)
	}
	if f.Limit != 500 || f.Skip != 0 {
		t.Errorf("limit/skip deveriam ser limitados: limit=%d skip=%d", f.Limit, f.Skip)
	}
}

func TestListImagesPadroes(t *testing.T) {
	env := newTestEnv(t)
	assertStatus(t, env.do("GET", "/images", nil, nil), http.StatusOK)
	if env.db.lastFilter.Limit != 50 {
		t.Errorf("limit padrão = %d, want 50", env.db.lastFilter.Limit)
	}
}

func TestListImagesVaziaRetornaArray(t *testing.T) {
	env := newTestEnv(t)
	rec := env.do("GET", "/images", nil, nil)
	if !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Errorf("lista vazia deve ser [] e não null: %s", rec.Body.String())
	}
}

func TestIntParam(t *testing.T) {
	cases := []struct {
		raw  string
		want int64
	}{
		{"", 50}, {"abc", 50}, {"10", 10}, {"0", 1}, {"9999", 500},
	}
	for _, tc := range cases {
		q := map[string][]string{"limit": {tc.raw}}
		if got := intParam(q, "limit", 50, 1, 500); got != tc.want {
			t.Errorf("intParam(%q) = %d, want %d", tc.raw, got, tc.want)
		}
	}
}

func TestGetImage(t *testing.T) {
	env := newTestEnv(t)
	sha := env.seedPending(t, "conteudo")

	t.Run("existe", func(t *testing.T) {
		rec := env.do("GET", "/images/"+sha, nil, nil)
		assertStatus(t, rec, http.StatusOK)
		var img store.Image
		decode(t, rec, &img)
		if img.SHA1 != sha {
			t.Errorf("sha1 = %q", img.SHA1)
		}
	})
	t.Run("não existe", func(t *testing.T) {
		assertStatus(t, env.do("GET", "/images/"+sha1Of("nada"), nil, nil), http.StatusNotFound)
	})
	t.Run("sha1 inválido", func(t *testing.T) {
		assertStatus(t, env.do("GET", "/images/123", nil, nil), http.StatusBadRequest)
	})
}

func TestImageFile(t *testing.T) {
	env := newTestEnv(t)
	sha := env.seedPending(t, "bytes da imagem")

	rec := env.do("GET", "/images/"+sha+"/file", nil, nil)
	assertStatus(t, rec, http.StatusOK)
	if rec.Body.String() != "bytes da imagem" {
		t.Errorf("conteúdo = %q", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("Content-Type = %q", ct)
	}
}

func TestAnnotateRascunho(t *testing.T) {
	env := newTestEnv(t)
	sha := env.seedPending(t, "conteudo")

	rec := env.doJSON("PUT", "/images/"+sha, `{"equipment":"8003-MF4739","day_time":"night"}`)
	assertStatus(t, rec, http.StatusOK)

	img := env.db.images[sha]
	if img.Status != store.StatusPending || img.DayTime != "night" {
		t.Errorf("rascunho não salvo como pending: %+v", img)
	}
	if _, err := os.Stat(env.pkgFile(sha + ".jpg")); err != nil {
		t.Error("rascunho não deveria renomear o arquivo")
	}
}

func TestAnnotateCompleto(t *testing.T) {
	env := newTestEnv(t)
	sha := env.seedPending(t, "conteudo")

	rec := env.do("PUT", "/images/"+sha, strings.NewReader(completeBody()),
		map[string]string{"Content-Type": "application/json", "X-User": "nathan"})
	assertStatus(t, rec, http.StatusOK)

	var img store.Image
	decode(t, rec, &img)

	wantName := sha + "_v2_8003-MF4739_hikvision-233_F-08-L-N-E-C_05.07_20260903T112523Z_ABC1D23-M-8-T-WHT.jpg"
	if img.FileName != wantName {
		t.Errorf("file_name:\ngot:  %s\nwant: %s", img.FileName, wantName)
	}
	if img.Status != store.StatusVerified || img.PatternVersion != "v2" {
		t.Errorf("status=%q version=%q", img.Status, img.PatternVersion)
	}
	if img.Path != "dataset/"+testPkg+"/"+wantName {
		t.Errorf("path = %q", img.Path)
	}
	if img.Vehicles[0].Plate != "ABC1D23" {
		t.Errorf("placa deveria ser salva em maiúscula: %q", img.Vehicles[0].Plate)
	}
	if !img.CapturedAt.Equal(time.Date(2026, 9, 3, 11, 25, 23, 0, time.UTC)) {
		t.Errorf("captured_at não convertido para UTC: %v", img.CapturedAt)
	}

	if _, err := os.Stat(env.pkgFile(wantName)); err != nil {
		t.Error("arquivo não foi renomeado na pasta do pacote:", err)
	}
	if _, err := os.Stat(env.pkgFile(sha + ".jpg")); !os.IsNotExist(err) {
		t.Error("o arquivo com nome de hash continuou existindo")
	}

	// histórico
	var statusEntry *store.HistoryEntry
	for i, h := range env.db.history {
		if h.Field == "status" {
			statusEntry = &env.db.history[i]
		}
	}
	if statusEntry == nil {
		t.Fatal("histórico sem a mudança de status")
	}
	if statusEntry.Old != "pending" || statusEntry.New != "verified" || statusEntry.ChangedBy != "nathan" {
		t.Errorf("histórico errado: %+v", *statusEntry)
	}
}

func TestAnnotateEditarVerificada(t *testing.T) {
	env := newTestEnv(t)
	sha := env.seedPending(t, "conteudo")
	assertStatus(t, env.doJSON("PUT", "/images/"+sha, completeBody()), http.StatusOK)

	t.Run("rascunho é recusado", func(t *testing.T) {
		assertStatus(t, env.doJSON("PUT", "/images/"+sha, `{"day_time":"day"}`), http.StatusConflict)
	})

	t.Run("completo renomeia de novo na pasta do pacote", func(t *testing.T) {
		before := env.db.images[sha].FileName
		rec := env.doJSON("PUT", "/images/"+sha, completeBody(func(b map[string]any) { b["day_time"] = "day" }))
		assertStatus(t, rec, http.StatusOK)

		after := env.db.images[sha].FileName
		if after == before || !strings.Contains(after, "-D-") {
			t.Errorf("nome não atualizado: %s", after)
		}
		if _, err := os.Stat(env.pkgFile(before)); !os.IsNotExist(err) {
			t.Error("nome antigo continuou existindo")
		}
		if _, err := os.Stat(env.pkgFile(after)); err != nil {
			t.Error("arquivo com nome novo não existe")
		}
	})
}

func TestAnnotateInvalido(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantMsg string
	}{
		{"código desconhecido", completeBody(func(b map[string]any) { b["day_time"] = "tarde" }), "período"},
		{"sem data", completeBody(func(b map[string]any) { delete(b, "captured_at") }), "captured_at"},
		{"sem veículos", completeBody(func(b map[string]any) { b["vehicles"] = []any{} }), "veículos"},
		{"sem condições", completeBody(func(b map[string]any) { b["conditions"] = []string{} }), "condições"},
		{"equipamento não cadastrado", completeBody(func(b map[string]any) { b["equipment"] = "9999-XX" }), "não cadastrado"},
		{"câmera não cadastrada", completeBody(func(b map[string]any) { b["camera"] = "outra-cam" }), "não cadastrada"},
		{"placa inválida", completeBody(func(b map[string]any) {
			b["vehicles"] = []map[string]string{{"plate": "ABC", "plate_type": "br", "plate_category": "private", "type": "car", "color": "red"}}
		}), "placa"},
		{"campo com nome errado", `{"daytime":"night"}`, "unknown field"},
		{"JSON quebrado", `{`, "JSON inválido"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestEnv(t)
			sha := env.seedPending(t, "conteudo")

			rec := env.doJSON("PUT", "/images/"+sha, tc.body)
			assertStatus(t, rec, http.StatusBadRequest)
			if msg := errorMsg(t, rec); !strings.Contains(msg, tc.wantMsg) {
				t.Errorf("mensagem %q não contém %q", msg, tc.wantMsg)
			}
			if env.db.images[sha].Status != store.StatusPending {
				t.Error("anotação inválida não pode mudar o status")
			}
			if _, err := os.Stat(env.pkgFile(sha + ".jpg")); err != nil {
				t.Error("anotação inválida não pode renomear o arquivo")
			}
		})
	}
}

func TestAnnotateDesfazRenameSeBancoFalhar(t *testing.T) {
	env := newTestEnv(t)
	sha := env.seedPending(t, "conteudo")
	env.db.replaceErr = errBoom

	assertStatus(t, env.doJSON("PUT", "/images/"+sha, completeBody()), http.StatusInternalServerError)

	if _, err := os.Stat(env.pkgFile(sha + ".jpg")); err != nil {
		t.Error("o rename deveria ter sido desfeito: arquivo com nome de hash sumiu")
	}
	entries, _ := os.ReadDir(filepath.Join(env.root, "dataset", testPkg))
	if len(entries) != 1 {
		t.Errorf("pasta do pacote deveria ter 1 arquivo, tem %d", len(entries))
	}
}

func TestAnnotateImagemInexistente(t *testing.T) {
	env := newTestEnv(t)
	assertStatus(t, env.doJSON("PUT", "/images/"+sha1Of("nada"), completeBody()), http.StatusNotFound)
}

func TestDiffHistory(t *testing.T) {
	before := &store.Image{SHA1: "x", Status: "pending", DayTime: "day", Conditions: []string{"glare"}}
	after := &store.Image{SHA1: "x", Status: "pending", DayTime: "night", Conditions: []string{"glare"}}

	entries := diffHistory(before, after, "nathan")
	if len(entries) != 1 {
		t.Fatalf("esperava 1 mudança, veio %d: %+v", len(entries), entries)
	}
	e := entries[0]
	if e.Field != "day_time" || e.Old != "day" || e.New != "night" || e.ChangedBy != "nathan" || e.ImageID != "x" {
		t.Errorf("entrada errada: %+v", e)
	}

	if got := diffHistory(after, after, "nathan"); len(got) != 0 {
		t.Errorf("sem mudanças deveria gerar 0 entradas, gerou %d", len(got))
	}
}
