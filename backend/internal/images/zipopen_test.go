package images

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Um zip64 (forçado por mais de 65.535 itens) com o "total de discos" do
// localizador 0, como gravam alguns programas: o Go puro recusa, o openZip abre.
func TestOpenZipComTotalDeDiscosZero(t *testing.T) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	const n = 1<<16 + 5
	for i := range n {
		if _, err := w.Create(fmt.Sprintf("f%05d.jpg", i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	loc := len(data) - eocdLen - zip64LocLen
	if string(data[loc:loc+4]) != zip64LocSignat {
		t.Fatalf("o zip do teste não é zip64")
	}
	binary.LittleEndian.PutUint32(data[loc+zip64LocDisks:], 0)

	name := filepath.Join(t.TempDir(), "z.zip")
	if err := os.WriteFile(name, data, 0o644); err != nil {
		t.Fatal(err)
	}
	// o motivo da correção: o archive/zip puro não abre
	if _, err := zip.OpenReader(name); !errors.Is(err, zip.ErrFormat) {
		t.Fatalf("o Go puro deveria recusar (o teste perdeu o sentido?): %v", err)
	}
	r, closer, err := openZip(name)
	if err != nil {
		t.Fatalf("openZip: %v", err)
	}
	defer closer.Close()
	if len(r.File) != n || r.File[n-1].Name != fmt.Sprintf("f%05d.jpg", n-1) {
		t.Errorf("itens: %d, último %q", len(r.File), r.File[len(r.File)-1].Name)
	}
	// o arquivo no disco não muda
	if got, _ := os.ReadFile(name); !bytes.Equal(got, data) {
		t.Error("o zip no disco mudou")
	}
}

// Zip normal abre igual; lixo continua dando erro de formato.
func TestOpenZipNormalELixo(t *testing.T) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, _ := w.Create("a.jpg")
	f.Write([]byte("x"))
	w.Close()
	dir := t.TempDir()
	ok := filepath.Join(dir, "ok.zip")
	os.WriteFile(ok, buf.Bytes(), 0o644)
	r, closer, err := openZip(ok)
	if err != nil || len(r.File) != 1 {
		t.Fatalf("zip normal: %v", err)
	}
	closer.Close()

	bad := filepath.Join(dir, "lixo.zip")
	os.WriteFile(bad, []byte("isto não é um zip PK\x05\x06 nem perto"), 0o644)
	if _, _, err := openZip(bad); !errors.Is(err, zip.ErrFormat) {
		t.Errorf("lixo: %v", err)
	}
}
