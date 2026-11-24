package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"SIDM/internal/filename"
	"SIDM/internal/images"
	"SIDM/internal/platform"
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
	return string(annotationJSON(b))
}

// annotationJSON leva um corpo "plano" (os campos do perfil veiculos e
// "vehicles", como era antes dos perfis) para o formato do PUT:
// {"fields": {...}, "objects": [{"type": "vehicle", "fields": {...}}], "complete": ...}.
func annotationJSON(flat map[string]any) []byte {
	body := map[string]any{"fields": map[string]any{}, "objects": []any{}}
	for k, v := range flat {
		switch k {
		case "complete":
			body["complete"] = v
		case "vehicles":
			objs := []any{}
			if vs, ok := v.([]map[string]string); ok {
				for _, x := range vs {
					objs = append(objs, map[string]any{"type": "vehicle", "fields": x})
				}
			}
			body["objects"] = objs
		default:
			body["fields"].(map[string]any)[k] = v
		}
	}
	out, _ := json.Marshal(body)
	return out
}

func TestListImages(t *testing.T) {
	env := newTestEnv(t)
	env.seedPending(t, "um")
	env.seedPending(t, "dois")

	rec := env.do("GET", "/images?package="+testPkg+"&status=pending&equipment=8003-MF4739&plate=abc1d23&limit=9999&skip=-5", nil, nil)
	assertStatus(t, rec, http.StatusOK)
	full := env.img.LastFilter

	// Aqui só interessa que os filtros CHEGAM no banco (ver lastFilter abaixo).
	// A contagem é conferida num pedido só com pacote e status:
	var resp struct {
		Total int64          `json:"total"`
		Items []images.Image `json:"items"`
	}
	decode(t, env.do("GET", "/images?package="+testPkg+"&status=pending", nil, nil), &resp)
	if resp.Total != 2 || len(resp.Items) != 2 {
		t.Errorf("total=%d items=%d", resp.Total, len(resp.Items))
	}
	_ = rec

	f := full
	if f.Package != testPkg || f.Status != "pending" || f.Fields["equipment"] != "8003-MF4739" {
		t.Errorf("filtro não repassado: %+v", f)
	}
	if f.Object["plate"] != "ABC1D23" {
		t.Errorf("placa deveria ir em maiúscula: %q", f.Object["plate"])
	}
	if f.Limit != 500 || f.Skip != 0 {
		t.Errorf("limit/skip deveriam ser limitados: limit=%d skip=%d", f.Limit, f.Skip)
	}

	// os filtros genéricos, de qualquer perfil
	env.do("GET", "/images?fields.riders=2&objects.color=black&vehicle_type=car", nil, nil)
	f = env.img.LastFilter
	if f.Fields["riders"] != "2" || f.Object["color"] != "black" || f.Object["type"] != "car" {
		t.Errorf("filtros genéricos: %+v", f)
	}
}

func TestListImagesPadroes(t *testing.T) {
	env := newTestEnv(t)
	assertStatus(t, env.do("GET", "/images", nil, nil), http.StatusOK)
	if env.img.LastFilter.Limit != 50 {
		t.Errorf("limit padrão = %d, want 50", env.img.LastFilter.Limit)
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
		if got := platform.IntParam(q, "limit", 50, 1, 500); got != tc.want {
			t.Errorf("platform.IntParam(%q) = %d, want %d", tc.raw, got, tc.want)
		}
	}
}

func TestGetImage(t *testing.T) {
	env := newTestEnv(t)
	sha := env.seedPending(t, "conteudo")

	t.Run("existe", func(t *testing.T) {
		rec := env.do("GET", "/images/"+sha, nil, nil)
		assertStatus(t, rec, http.StatusOK)
		var img images.Image
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

	rec := env.doJSON("PUT", "/images/"+sha, `{"fields": {"equipment":"8003-MF4739","day_time":"night"}}`)
	assertStatus(t, rec, http.StatusOK)

	img := env.img.Images[sha]
	if img.Status != images.StatusPending || img.Fields["day_time"] != "night" {
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

	var img images.Image
	decode(t, rec, &img)

	wantName := sha + "_v2_8003-MF4739_hikvision-233_F-08-L-N-E-C_05.07_20260903T112523Z_ABC1D23-M-8-T-WHT.jpg"
	if img.FileName != wantName {
		t.Errorf("file_name:\ngot:  %s\nwant: %s", img.FileName, wantName)
	}
	if img.Status != images.StatusVerified || img.PatternVersion != "v2" {
		t.Errorf("status=%q version=%q", img.Status, img.PatternVersion)
	}
	// O nome v2 é só um dado: o arquivo continua <sha1>.jpg.
	if img.Path != "dataset/"+testPkg+"/"+sha+".jpg" {
		t.Errorf("path = %q", img.Path)
	}
	if img.Objects[0].Fields["plate"] != "ABC1D23" {
		t.Errorf("placa deveria ser salva em maiúscula: %v", img.Objects[0].Fields["plate"])
	}
	stored := env.img.Images[sha].Fields["captured_at"].(time.Time)
	if !stored.Equal(time.Date(2026, 9, 3, 11, 25, 23, 0, time.UTC)) || stored.Location() != time.UTC {
		t.Errorf("captured_at não convertido para UTC: %v", stored)
	}

	if _, err := os.Stat(env.pkgFile(sha + ".jpg")); err != nil {
		t.Error("o arquivo tem que continuar <sha1>.jpg:", err)
	}
	if _, err := os.Stat(env.pkgFile(wantName)); !os.IsNotExist(err) {
		t.Error("nenhum arquivo com o nome v2 deveria existir no disco")
	}

	// histórico
	var statusEntry *images.HistoryEntry
	for i, h := range env.img.History {
		if h.Field == "status" {
			statusEntry = &env.img.History[i]
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
		assertStatus(t, env.doJSON("PUT", "/images/"+sha, `{"fields": {"day_time":"day"}}`), http.StatusConflict)
	})

	t.Run("completo recalcula o nome (o arquivo fica)", func(t *testing.T) {
		before := env.img.Images[sha].FileName
		rec := env.doJSON("PUT", "/images/"+sha, completeBody(func(b map[string]any) { b["day_time"] = "day" }))
		assertStatus(t, rec, http.StatusOK)

		after := env.img.Images[sha].FileName
		if after == before || !strings.Contains(after, "-D-") {
			t.Errorf("nome não atualizado: %s", after)
		}
		if _, err := os.Stat(env.pkgFile(sha + ".jpg")); err != nil {
			t.Error("o arquivo tem que continuar <sha1>.jpg")
		}
	})
}

func TestAnnotateInvalido(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantMsg string
	}{
		{"opção desconhecida", completeBody(func(b map[string]any) { b["day_time"] = "tarde" }), "fields.day_time"},
		{"condição desconhecida", completeBody(func(b map[string]any) { b["conditions"] = []string{"glare", "neve"} }), "fields.conditions"},
		{"equipamento não cadastrado", completeBody(func(b map[string]any) { b["equipment"] = "9999-XX" }), "não cadastrado"},
		{"câmera não cadastrada", completeBody(func(b map[string]any) { b["camera"] = "outra-cam" }), "não cadastrada"},
		{"placa inválida", completeBody(func(b map[string]any) {
			b["vehicles"] = []map[string]string{{"plate": "ABC", "plate_type": "br", "plate_category": "private", "type": "car", "color": "red"}}
		}), "objects[0].plate"},
		{"campo fora do perfil", `{"fields": {"daytime":"night"}}`, "não existe no perfil"},
		{"objeto fora do perfil", `{"objects": [{"type": "helmet", "fields": {}}]}`, "não existe no perfil"},
		{"campo com nome errado", `{"feilds": {}}`, "unknown field"},
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
			if env.img.Images[sha].Status != images.StatusPending {
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
	env.img.ReplaceErr = errBoom

	assertStatus(t, env.doJSON("PUT", "/images/"+sha, completeBody()), http.StatusInternalServerError)

	if _, err := os.Stat(env.pkgFile(sha + ".jpg")); err != nil {
		t.Error("o arquivo com nome de hash sumiu")
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
	before := &images.Image{SHA1: "x", Status: "pending", Fields: map[string]any{"day_time": "day", "conditions": []string{"glare"}}}
	after := &images.Image{SHA1: "x", Status: "pending", Fields: map[string]any{"day_time": "night", "conditions": []string{"glare"}}}

	entries := images.DiffHistory(before, after, "nathan")
	if len(entries) != 1 {
		t.Fatalf("esperava 1 mudança, veio %d: %+v", len(entries), entries)
	}
	e := entries[0]
	if e.Field != "day_time" || e.Old != "day" || e.New != "night" || e.ChangedBy != "nathan" || e.ImageID != "x" {
		t.Errorf("entrada errada: %+v", e)
	}

	if got := images.DiffHistory(after, after, "nathan"); len(got) != 0 {
		t.Errorf("sem mudanças deveria gerar 0 entradas, gerou %d", len(got))
	}
}

// Apagar uma imagem: ela sai do pacote e vai para a lixeira.
func TestExcluirImagem(t *testing.T) {
	env := newTestEnv(t)
	sha := env.seedVerified(t, "verificada") // tem histórico
	other := env.seedPending(t, "outra")
	file := env.pkgFile(sha + ".jpg")

	rec := env.do("DELETE", "/images/"+sha, nil, nil)
	assertStatus(t, rec, http.StatusOK)

	if _, ok := env.img.Images[sha]; ok {
		t.Error("o registro deveria ter sumido")
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Error("o arquivo deveria ter saído do pacote")
	}
	// vai para a lixeira: o registro e o arquivo (para voltar por uma versão)
	if tr, ok := env.img.Trash[sha]; !ok || tr.DeletedBy == "" || tr.Package != testPkg {
		t.Errorf("lixeira: %+v", tr)
	}
	if _, err := os.Stat(filepath.Join(env.root, "lixeira", sha+".jpg")); err != nil {
		t.Errorf("o arquivo deveria estar na lixeira: %v", err)
	}
	kept := false
	for _, h := range env.img.History {
		kept = kept || h.ImageID == sha
	}
	if !kept {
		t.Error("o histórico fica (a imagem pode voltar)")
	}
	if _, ok := env.img.Images[other]; !ok {
		t.Error("a outra imagem não pode ser afetada")
	}

	assertStatus(t, env.do("DELETE", "/images/"+sha, nil, nil), http.StatusNotFound)
	assertStatus(t, env.do("DELETE", "/images/nao-e-sha1", nil, nil), http.StatusBadRequest)
}

func TestExcluirImagemReservadaPorOutro(t *testing.T) {
	env := newTestEnv(t)
	sha := env.seedPending(t, "x")
	env.next(t, "maria", "")

	rec := env.do("DELETE", "/images/"+sha, nil, map[string]string{"X-User": "nathan"})
	assertStatus(t, rec, http.StatusConflict)
	if !strings.Contains(errorMsg(t, rec), "maria") {
		t.Errorf("a mensagem deveria dizer quem está com a imagem: %q", errorMsg(t, rec))
	}
	// quem está com ela pode apagar
	assertStatus(t, env.do("DELETE", "/images/"+sha, nil, map[string]string{"X-User": "maria"}), http.StatusOK)
}

// Nenhum campo é obrigatório para verificar: o que não deu para saber fica
// vazio, e no nome v2 vira uma posição vazia.
func TestVerificarComCamposDesconhecidos(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		inName string // trecho esperado no nome do arquivo
	}{
		{"sem data", completeBody(func(b map[string]any) { delete(b, "captured_at") }), "_05.07__ABC1D23-"},
		{"sem veículo", completeBody(func(b map[string]any) { b["vehicles"] = []any{} }), "Z_.jpg"},
		{"sem condições", completeBody(func(b map[string]any) { b["conditions"] = []string{} }), "-C__2026"},
		{"sem equipamento e câmera", completeBody(func(b map[string]any) { b["equipment"] = ""; b["camera"] = "" }), "_v2___F-"},
		{"placa ilegível", completeBody(func(b map[string]any) {
			b["vehicles"] = []map[string]string{{"plate": "", "plate_type": "", "plate_category": "", "type": "truck", "color": ""}}
		}), "Z_---T-.jpg"},
		{"tudo desconhecido", `{"complete": true}`, "_v2___-----___.jpg"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestEnv(t)
			sha := env.seedPending(t, "conteudo")

			rec := env.doJSON("PUT", "/images/"+sha, tc.body)
			assertStatus(t, rec, http.StatusOK)
			img := env.img.Images[sha]
			if img.Status != images.StatusVerified || !strings.Contains(img.FileName, tc.inName) {
				t.Errorf("status=%s nome=%q (esperava conter %q)", img.Status, img.FileName, tc.inName)
			}
			if _, err := os.Stat(env.pkgFile(sha + ".jpg")); err != nil {
				t.Error("o arquivo tem que continuar <sha1>.jpg")
			}
			// o nome volta a ser lido certinho (ex.: num upload em outro SIDM)
			if _, err := filename.Parse(img.FileName); err != nil {
				t.Errorf("Parse(%q): %v", img.FileName, err)
			}
		})
	}
}
