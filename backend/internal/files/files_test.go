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
	for _, dir := range []string{"tmp", "dataset"} {
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

func TestReceiveAndStore(t *testing.T) {
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

	// Store cria a pasta do pacote se ela ainda não existir.
	rel := PendingPath("pacote-1", sum, ".jpg")
	if err := st.Store(tmp, rel); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(st.Abs(rel))
	if err != nil || string(got) != "hello" {
		t.Errorf("conteúdo errado: %q, %v", got, err)
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Error("temporário deveria ter sumido depois do Store")
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

func TestEnsurePackageDir(t *testing.T) {
	st, root := newStorage(t)
	if err := st.EnsurePackageDir("noturnas"); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(root, "dataset", "noturnas")); err != nil || !info.IsDir() {
		t.Errorf("pasta do pacote não criada: %v", err)
	}
	if err := st.EnsurePackageDir("noturnas"); err != nil {
		t.Errorf("criar de novo não pode falhar: %v", err)
	}
}

func TestMoveRenomeiaNaPastaDoPacote(t *testing.T) {
	st, _ := newStorage(t)
	tmp, sum, _, _ := st.Receive(strings.NewReader("conteudo"))
	from := PendingPath("pacote-1", sum, ".jpg")
	st.Store(tmp, from)

	to := VerifiedPath("pacote-1", "nome-v2.jpg")
	if err := st.Move(from, to); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(st.Abs(to)); err != nil {
		t.Error("arquivo não chegou no destino")
	}
	if _, err := os.Stat(st.Abs(from)); !os.IsNotExist(err) {
		t.Error("arquivo continuou com o nome antigo")
	}

	if err := st.Move("dataset/pacote-1/nao-existe.jpg", to); err == nil {
		t.Error("mover arquivo inexistente deveria dar erro")
	}
}

func TestCaminhosRelativos(t *testing.T) {
	// No banco os caminhos usam "/" em qualquer sistema operacional.
	cases := map[string]string{
		PackageDir("p1"):                 "dataset/p1",
		PendingPath("p1", "abc", ".jpg"): "dataset/p1/abc.jpg",
		VerifiedPath("p1", "x.png"):      "dataset/p1/x.png",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}

	st, root := newStorage(t)
	if got, want := st.Abs("dataset/p1/x.png"), filepath.Join(root, "dataset", "p1", "x.png"); got != want {
		t.Errorf("Abs = %q, want %q", got, want)
	}
}

func TestRenamePackageDir(t *testing.T) {
	st, root := newStorage(t)
	tmp, sum, _, _ := st.Receive(strings.NewReader("x"))
	st.Store(tmp, PendingPath("antigo", sum, ".jpg"))

	if err := st.RenamePackageDir("antigo", "novo"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "dataset", "novo", sum+".jpg")); err != nil {
		t.Error("o arquivo deveria estar na pasta nova")
	}
	if _, err := os.Stat(filepath.Join(root, "dataset", "antigo")); !os.IsNotExist(err) {
		t.Error("a pasta antiga deveria ter sumido")
	}

	st.EnsurePackageDir("outro")
	if err := st.RenamePackageDir("novo", "outro"); err == nil {
		t.Error("renomear para uma pasta que já existe deveria falhar (senão misturaria pacotes)")
	}

	// Pacote que nunca recebeu arquivo: não tem pasta, e tudo bem.
	if err := st.RenamePackageDir("sem-pasta", "com-pasta"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "dataset", "com-pasta")); err != nil {
		t.Error("deveria ter criado a pasta nova")
	}
}

func TestRemovePackageDir(t *testing.T) {
	st, root := newStorage(t)
	tmp, sum, _, _ := st.Receive(strings.NewReader("x"))
	st.Store(tmp, PendingPath("apagar", sum, ".jpg"))
	st.EnsurePackageDir("manter")

	if err := st.RemovePackageDir("apagar"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "dataset", "apagar")); !os.IsNotExist(err) {
		t.Error("a pasta deveria ter sido apagada")
	}
	if _, err := os.Stat(filepath.Join(root, "dataset", "manter")); err != nil {
		t.Error("as outras pastas não podem ser afetadas")
	}

	for _, bad := range []string{"", ".", "..", "../tmp", "a/b"} {
		if err := st.RemovePackageDir(bad); err == nil {
			t.Errorf("RemovePackageDir(%q) deveria recusar", bad)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "tmp")); err != nil {
		t.Fatal("a proteção falhou: apagou algo fora de dataset/")
	}
}
