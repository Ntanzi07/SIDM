package api

import (
	"SIDM/internal/catalog"
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

	inPkg, _ := os.ReadDir(filepath.Join(env.root, "dataset", testPkg))
	tmp, _ := os.ReadDir(filepath.Join(env.root, "tmp"))
	if len(inPkg) != 2 {
		t.Errorf("pasta do pacote com %d arquivos, want 2", len(inPkg))
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
	if img.Package != testPkg {
		t.Errorf("package = %q", img.Package)
	}
	if img.Path != "dataset/"+testPkg+"/"+sha+".jpg" {
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
	rec := env.doJSON("POST", "/images?package="+testPkg, `{"files":[]}`)
	assertStatus(t, rec, http.StatusBadRequest)
	if !strings.Contains(errorMsg(t, rec), "multipart") {
		t.Errorf("mensagem pouco clara: %q", errorMsg(t, rec))
	}
}

func TestUploadRepetidaEmOutroPacoteEPulada(t *testing.T) {
	env := newTestEnv(t)
	env.cat.Packages["pacote-novo"] = catalog.Package{Name: "pacote-novo"}
	shaA := env.seedPending(t, "imagem A") // já está no pacote-teste

	res := env.uploadTo(t, "pacote-novo",
		upFile{"a-de-novo.jpg", "imagem A"},
		upFile{"nova.jpg", "imagem nova"},
	)

	if res[0].Status != "duplicate" || res[0].ExistingPackage != testPkg {
		t.Errorf("repetida: %+v (want duplicate, existing_package=%s)", res[0], testPkg)
	}
	if res[1].Status != "created" {
		t.Errorf("o resto do lote deveria seguir: %+v", res[1])
	}

	if env.db.images[shaA].Package != testPkg {
		t.Error("a repetida não pode mudar de pacote")
	}
	novo, _ := os.ReadDir(filepath.Join(env.root, "dataset", "pacote-novo"))
	if len(novo) != 1 {
		t.Errorf("pacote-novo deveria ter só a imagem nova, tem %d arquivos", len(novo))
	}
}

func TestUploadPacoteObrigatorio(t *testing.T) {
	env := newTestEnv(t)

	t.Run("sem pacote", func(t *testing.T) {
		rec := env.do("POST", "/images", strings.NewReader(""), map[string]string{"Content-Type": "multipart/form-data; boundary=x"})
		assertStatus(t, rec, http.StatusBadRequest)
	})
	t.Run("pacote inexistente", func(t *testing.T) {
		rec := env.do("POST", "/images?package=nao-existe", strings.NewReader(""), map[string]string{"Content-Type": "multipart/form-data; boundary=x"})
		assertStatus(t, rec, http.StatusNotFound)
	})
}
