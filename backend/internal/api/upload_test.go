package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"SIDM/internal/store"
)

func TestUploadResultados(t *testing.T) {
	env := newTestEnv(t)
	res := env.upload(t,
		upFile{"a.jpg", "imagem A"},
		upFile{"copia-de-a.JPG", "imagem A"}, // mesmo conteúdo, outro nome
		upFile{"b.png", "imagem B"},
		upFile{"leia-me.txt", "não é imagem"},
	)

	want := []string{"created", "duplicate", "created", "error"}
	if len(res) != len(want) {
		t.Fatalf("esperava %d resultados, veio %d: %+v", len(want), len(res), res)
	}
	for i, w := range want {
		if res[i].Status != w {
			t.Errorf("arquivo %s: status = %q, want %q", res[i].File, res[i].Status, w)
		}
	}
	if res[0].SHA1 != sha1Of("imagem A") || res[1].SHA1 != res[0].SHA1 {
		t.Errorf("sha1 errado: %+v", res[:2])
	}

	staging, _ := os.ReadDir(filepath.Join(env.root, "staging"))
	tmp, _ := os.ReadDir(filepath.Join(env.root, "tmp"))
	if len(staging) != 2 {
		t.Errorf("staging com %d arquivos, want 2", len(staging))
	}
	if len(tmp) != 0 {
		t.Errorf("sobrou lixo em tmp: %d arquivos", len(tmp))
	}
}

func TestUploadGravaDocumentoPending(t *testing.T) {
	env := newTestEnv(t)
	sha := env.seedPending(t, "conteudo")

	img := env.db.images[sha]
	if img.Status != store.StatusPending {
		t.Errorf("status = %q", img.Status)
	}
	if img.OriginalName != "original.jpg" || img.Ext != ".jpg" || img.Size != int64(len("conteudo")) {
		t.Errorf("documento errado: %+v", img)
	}
	if img.Path != "staging/"+sha+".jpg" {
		t.Errorf("path = %q", img.Path)
	}
	if img.IngestedAt.IsZero() {
		t.Error("ingested_at vazio")
	}
}

func TestUploadPrefill(t *testing.T) {
	content := "imagem com nome v2"
	sha := sha1Of(content)
	v2 := sha + "_v2_8003-MF4739_hikvision-233_F-01-L-D-E-S_01_20261103T221400Z_KOY7386-B-1-B-WHT.jpg"

	t.Run("hash do nome bate com o conteúdo", func(t *testing.T) {
		env := newTestEnv(t)
		res := env.upload(t, upFile{v2, content})
		if !res[0].Prefilled {
			t.Fatal("esperava prefilled")
		}
		img := env.db.images[sha]
		if img.Equipment != "8003-MF4739" || img.DayTime != "day" || len(img.Vehicles) != 1 {
			t.Errorf("metadados não pré-preenchidos: %+v", img)
		}
		if img.Status != store.StatusPending {
			t.Error("pré-preenchida deve continuar pending até alguém conferir")
		}
	})

	t.Run("hash do nome NÃO bate", func(t *testing.T) {
		env := newTestEnv(t)
		res := env.upload(t, upFile{v2, "outro conteúdo"})
		if res[0].Prefilled {
			t.Error("não deveria pré-preencher: o nome mente sobre o conteúdo")
		}
	})
}

func TestUploadNomeComCaminho(t *testing.T) {
	env := newTestEnv(t)
	res := env.upload(t, upFile{`../../etc/passwd.jpg`, "x"})
	if res[0].File != "passwd.jpg" {
		t.Errorf("caminho do cliente não foi removido: %q", res[0].File)
	}
}

func TestUploadNaoMultipart(t *testing.T) {
	env := newTestEnv(t)
	rec := env.doJSON("POST", "/images", `{"files":[]}`)
	assertStatus(t, rec, http.StatusBadRequest)
	if !strings.Contains(errorMsg(t, rec), "multipart") {
		t.Errorf("mensagem pouco clara: %q", errorMsg(t, rec))
	}
}
