package crypt

import (
	"bytes"
	"errors"
	"testing"
)

// Um "JPEG" com INI no final, como sai do equipamento antes de criptografar.
var original = append([]byte{0xFF, 0xD8, 0xFF, 0xC0, 0x01, 0x02, 0xFF, 0xD9}, []byte("\n[NMetrologico]\nPlaca = ABC1D23\n")...)

func TestOpenRC4Legado(t *testing.T) {
	encrypted := EncryptRC4Legacy(original)
	if IsJPEG(encrypted) {
		t.Fatal("o arquivo criptografado não deveria parecer JPEG")
	}

	plain, format, err := Open(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if format != FormatRC4Legacy {
		t.Errorf("formato = %q", format)
	}
	if !bytes.Equal(plain, original) {
		t.Error("o arquivo aberto tem que ser IGUAL ao original, byte a byte (a assinatura do INI depende disso)")
	}
}

func TestOpenNaoAlteraAEntrada(t *testing.T) {
	encrypted := EncryptRC4Legacy(original)
	backup := append([]byte(nil), encrypted...)
	Open(encrypted)
	if !bytes.Equal(encrypted, backup) {
		t.Error("Open alterou os dados de entrada")
	}
}

func TestOpenJPEGAberto(t *testing.T) {
	plain, format, err := Open(original)
	if err != nil || format != FormatPlain || !bytes.Equal(plain, original) {
		t.Errorf("JPEG aberto: format=%q err=%v", format, err)
	}
}

func TestOpenPGP(t *testing.T) {
	data := append([]byte("\x8a\x01\x02 dados cifrados "), []byte("|-----BEGIN PGP MESSAGE-----\n...")...)
	if _, _, err := Open(data); !errors.Is(err, ErrPGPUnsupported) {
		t.Errorf("err = %v, want ErrPGPUnsupported", err)
	}
}

func TestOpenDesconhecido(t *testing.T) {
	for name, data := range map[string][]byte{
		"PNG":   []byte("\x89PNG\r\n\x1a\n...."),
		"texto": []byte("não sou imagem"),
		"vazio": {},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := Open(data); !errors.Is(err, ErrNotEncrypted) {
				t.Errorf("err = %v, want ErrNotEncrypted", err)
			}
		})
	}
}

func TestChaveLegadaIgualAoEquipamento(t *testing.T) {
	// sha256 de um único byte 0x00, conferido à parte (sha256sum de printf '\0').
	want := "6e340b9cffb37a989ca544e6bb780a2c78901d3fb33738768511a30617afa01d"
	got := ""
	for _, b := range legacyKey {
		got += string("0123456789abcdef"[b>>4]) + string("0123456789abcdef"[b&15])
	}
	if got != want {
		t.Errorf("chave = %s, want %s", got, want)
	}
}
