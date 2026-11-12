package files

import (
	"os"
	"strings"
	"testing"
)

func TestReceiveAndStage(t *testing.T) {
	st, err := NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	tmp, sum, size, err := st.Receive(strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	// sha1("hello") conhecido
	if sum != "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d" {
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
