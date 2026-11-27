package api

import (
	"bufio"
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"SIDM/internal/imports"
)

type importLine struct {
	Progress *imports.Step   `json:"progress"`
	Report   *imports.Report `json:"report"`
	Error    string          `json:"error"`
}

// postFileStream manda o CSV pedindo o andamento (uma linha JSON por passo).
func (e *testEnv) postFileStream(t *testing.T, path, content string) []importLine {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	w, _ := mw.CreateFormFile("file", "dados.csv")
	w.Write([]byte(content))
	mw.Close()
	rec := e.do("POST", path, &body, map[string]string{
		"Content-Type": mw.FormDataContentType(), "X-User": "nathan", "Accept": "application/x-ndjson",
	})
	assertStatus(t, rec, http.StatusOK)
	if ct := rec.Header().Get("Content-Type"); ct != "application/x-ndjson" {
		t.Fatalf("Content-Type = %q", ct)
	}
	var lines []importLine
	sc := bufio.NewScanner(rec.Body)
	for sc.Scan() {
		var l importLine
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			t.Fatalf("linha que não é JSON: %q", sc.Text())
		}
		lines = append(lines, l)
	}
	return lines
}

// Com Accept: application/x-ndjson, o import manda as fases e, no fim, o
// relatório; quem não pede recebe o relatório como sempre (os outros testes).
func TestImportComAndamento(t *testing.T) {
	env := newTestEnv(t)
	env.withProfile(t)
	env.seedNamed(t, "a.jpg", "conteudo A")
	env.seedNamed(t, "b.jpg", "conteudo B")

	check := func(dryRun bool) {
		url := "/packages/" + testPkg + "/import?profile=csv-teste"
		if !dryRun {
			url += "&dry_run=false"
		}
		lines := env.postFileStream(t, url, importCSV)
		last := lines[len(lines)-1]
		if last.Report == nil || last.Report.DryRun != dryRun || last.Report.Applied != 2 {
			t.Fatalf("última linha (dry_run=%v): %+v", dryRun, last)
		}
		phases := map[string]imports.Step{}
		var order []string
		for _, l := range lines[:len(lines)-1] {
			if l.Progress == nil {
				t.Fatalf("linha do meio sem progress: %+v", l)
			}
			if _, seen := phases[l.Progress.Phase]; !seen {
				order = append(order, l.Progress.Phase)
			}
			phases[l.Progress.Phase] = *l.Progress // a última de cada fase
		}
		want := "lendo conferindo cadastros"
		if !dryRun {
			want += " gravando"
		}
		if got := strings.Join(order, " "); got != want {
			t.Errorf("fases (dry_run=%v): %q, queria %q", dryRun, got, want)
		}
		if c := phases[imports.PhaseChecking]; c.Total != 3 || c.Done != 3 {
			t.Errorf("conferindo termina em 3 de 3: %+v", c)
		}
		if g, ok := phases[imports.PhaseWriting]; ok && (g.Total != 2 || g.Done != 2) {
			t.Errorf("gravando termina em 2 de 2: %+v", g)
		}
	}
	check(true)
	check(false)
}

// Um erro no meio (o CSV sem a coluna da imagem) vem na última linha.
func TestImportComAndamentoErro(t *testing.T) {
	env := newTestEnv(t)
	env.withProfile(t)
	lines := env.postFileStream(t, "/packages/"+testPkg+"/import?profile=csv-teste", "outra;coisa\n1;2\n")
	last := lines[len(lines)-1]
	if last.Error == "" || !strings.Contains(last.Error, "coluna") || last.Report != nil {
		t.Errorf("erro no fim: %+v", last)
	}
}
