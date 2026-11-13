package api

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"SIDM/internal/crypt"
)

func TestUploadDescriptografaRC4Legado(t *testing.T) {
	env := newTestEnv(t)
	original := jpegWithINI("a", "GGB5H24")
	encrypted := string(crypt.EncryptRC4Legacy([]byte(original)))

	// Os equipamentos usam extensão própria para o arquivo criptografado.
	res := env.upload(t, upFile{"foto.enc", encrypted})
	r := res[0]
	if r.Status != "created" || r.Decrypted != crypt.FormatRC4Legacy {
		t.Fatalf("resultado = %+v", r)
	}
	if r.SHA1 != sha1Of(original) {
		t.Error("o sha1 tem que ser do arquivo ABERTO (senão a mesma foto vira duas imagens)")
	}
	if !r.INI {
		t.Error("o INI do arquivo aberto deveria ter sido lido")
	}

	img := env.db.images[r.SHA1]
	if img.Ext != ".jpg" || img.Encryption != crypt.FormatRC4Legacy {
		t.Errorf("ext=%q encryption=%q", img.Ext, img.Encryption)
	}
	onDisk, err := os.ReadFile(env.files.Abs(img.Path))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(onDisk, []byte(original)) {
		t.Error("no disco tem que estar o ORIGINAL aberto, byte a byte (a assinatura do INI depende disso)")
	}

	tmp, _ := os.ReadDir(env.root + "/tmp")
	if len(tmp) != 0 {
		t.Errorf("sobrou %d arquivo(s) em tmp", len(tmp))
	}
}

func TestUploadCriptografadaEAbertaSaoAMesmaImagem(t *testing.T) {
	env := newTestEnv(t)
	original := jpegWithINI("a", "AAA1111")

	res := env.upload(t,
		upFile{"aberta.jpg", original},
		upFile{"criptografada.enc", string(crypt.EncryptRC4Legacy([]byte(original)))},
	)
	if res[0].Status != "created" || res[1].Status != "duplicate" {
		t.Errorf("a versão criptografada da mesma foto deveria ser duplicata: %+v / %+v", res[0], res[1])
	}
}

func TestUploadComPerfilDeINIEmArquivoCriptografado(t *testing.T) {
	env := newTestEnv(t)
	env.withINIProfile(t)
	encrypted := string(crypt.EncryptRC4Legacy([]byte(jpegWithINI("a", "GGB5H24"))))

	res := env.uploadToWithQuery(t, testPkg, "&profile=ini-mfr1", upFile{"foto.enc", encrypted})
	img := env.db.images[res[0].SHA1]
	if !res[0].Prefilled || len(img.Vehicles) != 1 || img.Vehicles[0].Plate != "GGB5H24" {
		t.Errorf("o perfil de INI deveria funcionar no arquivo descriptografado: %+v", res[0])
	}
}

func TestUploadPGPRecusadoComMensagemClara(t *testing.T) {
	env := newTestEnv(t)
	pgp := "\x8a\x01\x02 dados |-----BEGIN PGP MESSAGE-----\n...\n-----END PGP MESSAGE-----"

	res := env.upload(t, upFile{"foto.enc", pgp})
	if res[0].Status != "error" || !strings.Contains(res[0].Error, "PGP") {
		t.Errorf("resultado = %+v", res[0])
	}
	tmp, _ := os.ReadDir(env.root + "/tmp")
	if len(tmp) != 0 {
		t.Error("arquivo recusado deixou lixo em tmp")
	}
}

func TestUploadExtensaoAindaValeParaArquivoAberto(t *testing.T) {
	env := newTestEnv(t)
	res := env.upload(t,
		upFile{"foto.gif", "\xFF\xD8\xFF jpeg com extensão errada"},
		upFile{"dados.bin", "nem jpeg nem criptografado"},
	)
	for _, r := range res {
		if r.Status != "error" || !strings.Contains(r.Error, "extensão") {
			t.Errorf("%s: %+v", r.File, r)
		}
	}
}
