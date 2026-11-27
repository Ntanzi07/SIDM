package api

import (
	"net/http"
	"strings"
	"testing"

	"SIDM/internal/images"
)

// A resolução do veiculos vem do arquivo: o upload já mede, a tela recebe
// medido e salvar com outro valor não muda nada (quem dita é a imagem).
func TestResolucaoVemDaImagem(t *testing.T) {
	env := newTestEnv(t)
	res := env.upload(t, upFile{"a.jpg", realJPEG(t, 640, 480, 1)}, upFile{"b.jpg", realJPEG(t, 33, 33, 2)})
	ok, odd := res[0].SHA1, res[1].SHA1

	get := func(sha string) images.Image {
		var img images.Image
		decode(t, env.do("GET", "/images/"+sha, nil, nil), &img)
		return img
	}
	if got := get(ok).Fields["resolution"]; got != "640x480" {
		t.Errorf("depois do upload: resolution = %v", got)
	}
	// 33x33 não é opção do perfil: fica a medida do arquivo assim mesmo
	if got := get(odd).Fields["resolution"]; got != "33x33" {
		t.Errorf("33x33 fora das opções: resolution = %v", got)
	}

	// salvar com outra resolução: o servidor mede e põe a do arquivo
	rec := env.doJSON("PUT", "/images/"+ok, `{"fields": {"resolution": "1920x1080", "weather": "rain"}}`)
	assertStatus(t, rec, http.StatusOK)
	var saved images.Image
	decode(t, rec, &saved)
	if saved.Fields["resolution"] != "640x480" || saved.Fields["weather"] != "rain" {
		t.Errorf("salvo: %v", saved.Fields)
	}
	rec = env.doJSON("PUT", "/images/"+odd, `{"fields": {"resolution": "640x480"}}`)
	assertStatus(t, rec, http.StatusOK)
	var savedOdd images.Image // variável nova: decodificar num mapa existente soma as chaves
	decode(t, rec, &savedOdd)
	if savedOdd.Fields["resolution"] != "33x33" {
		t.Errorf("33x33 salvo: %v", savedOdd.Fields)
	}

	// verificar a de 33x33: o nome v2 sai com a resolução desconhecida
	rec = env.doJSON("PUT", "/images/"+odd, `{"fields": {"weather": "rain"}, "complete": true}`)
	assertStatus(t, rec, http.StatusOK)
	if img := env.img.Images[odd]; img.Fields["resolution"] != "33x33" || img.FileName == "" {
		t.Errorf("verificada 33x33: %v %q", img.Fields, img.FileName)
	}
}

// As imagens de antes da regra, com a resolução escolhida à mão: o ensaio
// conta (sem mudar nada), corrigir põe a do arquivo, refaz o nome v2 da
// verificada e deixa no histórico.
func TestCorrigirResolucaoDasAntigas(t *testing.T) {
	env := newTestEnv(t)
	sha := env.upload(t, upFile{"a.jpg", realJPEG(t, 640, 480, 3)})[0].SHA1
	assertStatus(t, env.doJSON("PUT", "/images/"+sha, `{"fields": {"weather": "rain"}, "complete": true}`), http.StatusOK)
	good := env.img.Images[sha].FileName

	// como estaria antes da regra: a resolução errada, escolhida à mão
	old := env.img.Images[sha]
	old.Fields["resolution"] = "1920x1080"
	old.FileName = "nome-antigo.jpg"
	env.img.Images[sha] = old

	var rep images.FromImageReport
	rec := env.doJSON("POST", "/system/from-image", `{"dry_run": true}`)
	assertStatus(t, rec, http.StatusOK)
	decode(t, rec, &rep)
	if !rep.DryRun || rep.Changed != 1 || rep.Verified != 1 || len(rep.Examples) != 1 || !strings.Contains(rep.Examples[0], "1920x1080 → 640x480") {
		t.Fatalf("ensaio: %+v", rep)
	}
	if env.img.Images[sha].Fields["resolution"] != "1920x1080" {
		t.Fatal("o ensaio gravou")
	}

	var fixed images.FromImageReport
	rec = env.doJSON("POST", "/system/from-image", `{"dry_run": false}`)
	assertStatus(t, rec, http.StatusOK)
	decode(t, rec, &fixed)
	img := env.img.Images[sha]
	if fixed.DryRun || fixed.Changed != 1 || img.Fields["resolution"] != "640x480" || img.FileName != good || img.Status != images.StatusVerified {
		t.Fatalf("corrigir: %+v / %v %q", fixed, img.Fields, img.FileName)
	}
	var logged bool
	for _, h := range env.img.History {
		logged = logged || (h.ImageID == sha && strings.Contains(h.ChangedBy, "via resolução da imagem"))
	}
	if !logged {
		t.Error("a correção não ficou no histórico")
	}
	// de novo: nada a fazer
	decode(t, env.doJSON("POST", "/system/from-image", `{"dry_run": true}`), &rep)
	if rep.Changed != 0 {
		t.Errorf("segunda vez: %+v", rep)
	}
}
