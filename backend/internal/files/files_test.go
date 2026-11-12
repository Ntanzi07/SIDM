package files

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newStorage(t *testing.T) (*Storage, string) {
	t.Helper()
	root := t.TempDir()
	st, err := NewStorage(root)
	if err != nil {
		t.Fatal(err)
	}
	return st, root
}

func TestNewStorageCriaPastas(t *testing.T) {
	_, root := newStorage(t)
	for _, dir := range []string{"tmp", "staging", "dataset"} {
		info, err := os.Stat(filepath.Join(root, dir))
		if err != nil || !info.IsDir() {
			t.Errorf("pasta %s não criada: %v", dir, err)
		}
	}
}

func TestNewStorageIdempotente(t *testing.T) {
	_, root := newStorage(t)
	if _, err := NewStorage(root); err != nil {
		t.Errorf("chamar de novo numa pasta existente não pode falhar: %v", err)
	}
}

func TestReceiveAndStage(t *testing.T) {
	st, _ := newStorage(t)

	tmp, sum, size, err := st.Receive(strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if sum != "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d" { // sha1("hello")
		t.Errorf("sha1 errado: %s", sum)
	}
	if size != 5 {
		t.Errorf("tamanho errado: %d", size)
	}

	if err := st.Stage(tmp, sum, ".jpg"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(st.Abs(StagingPath(sum, ".jpg")))
	if err != nil || string(got) != "hello" {
		t.Errorf("conteúdo em staging errado: %q, %v", got, err)
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Error("temporário deveria ter sumido depois do Stage")
	}
}

// errReader simula uma conexão que cai no meio do upload.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("conexão caiu") }

func TestReceiveFalhaNaoDeixaLixo(t *testing.T) {
	st, root := newStorage(t)
	if _, _, _, err := st.Receive(errReader{}); err == nil {
		t.Fatal("esperava erro")
	}
	entries, _ := os.ReadDir(filepath.Join(root, "tmp"))
	if len(entries) != 0 {
		t.Errorf("upload que falhou deixou %d arquivo(s) em tmp", len(entries))
	}
}

func TestDiscard(t *testing.T) {
	st, _ := newStorage(t)
	tmp, _, _, _ := st.Receive(strings.NewReader("x"))
	st.Discard(tmp)
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Error("Discard não apagou o temporário")
	}
}

func TestMove(t *testing.T) {
	st, _ := newStorage(t)
	tmp, sum, _, _ := st.Receive(strings.NewReader("conteudo"))
	st.Stage(tmp, sum, ".jpg")

	from := StagingPath(sum, ".jpg")
	to := DatasetPath("nome-final.jpg")
	if err := st.Move(from, to); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(st.Abs(to)); err != nil {
		t.Error("arquivo não chegou no destino")
	}
	if _, err := os.Stat(st.Abs(from)); !os.IsNotExist(err) {
		t.Error("arquivo continuou na origem")
	}

	if err := st.Move("staging/nao-existe.jpg", to); err == nil {
		t.Error("mover arquivo inexistente deveria dar erro")
	}
}

func TestCaminhosRelativos(t *testing.T) {
	// No banco os caminhos usam "/" em qualquer sistema operacional.
	if got := StagingPath("abc", ".jpg"); got != "staging/abc.jpg" {
		t.Errorf("StagingPath = %q", got)
	}
	if got := DatasetPath("x.png"); got != "dataset/x.png" {
		t.Errorf("DatasetPath = %q", got)
	}

	st, root := newStorage(t)
	if got, want := st.Abs("dataset/x.png"), filepath.Join(root, "dataset", "x.png"); got != want {
		t.Errorf("Abs = %q, want %q", got, want)
	}
}
