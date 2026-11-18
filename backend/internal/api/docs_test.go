package api

import (
	"SIDM/internal/boxes"
	"SIDM/internal/catalog"
	"bufio"
	"bytes"
	"net/http"
	"sort"
	"strings"
	"testing"
)

// specOperations lê do openapi.yaml os pares "MÉTODO /caminho".
// Parser mínimo de propósito (sem dependência de lib YAML): o arquivo usa
// "  /caminho:" com 2 espaços dentro de "paths:" e "    get:" com 4.
func specOperations(t *testing.T) map[string]bool {
	t.Helper()
	ops := map[string]bool{}
	inPaths, path := false, ""

	sc := bufio.NewScanner(bytes.NewReader(openAPISpec))
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "paths:":
			inPaths = true
		case inPaths && len(line) > 0 && line[0] != ' ' && line[0] != '#':
			inPaths = false // terminou a seção paths (ex: "components:")
		case inPaths && strings.HasPrefix(line, "  /") && strings.HasSuffix(line, ":"):
			path = strings.TrimSuffix(strings.TrimSpace(line), ":")
		case inPaths && path != "":
			for _, m := range []string{"get", "post", "put", "delete", "patch"} {
				if line == "    "+m+":" {
					ops[strings.ToUpper(m)+" "+path] = true
				}
			}
		}
	}
	if len(ops) == 0 {
		t.Fatal("nenhuma operação encontrada no openapi.yaml: o formato do arquivo mudou?")
	}
	return ops
}

// Se você criar uma rota e esquecer de documentar (ou apagar uma rota e
// esquecer a doc), este teste falha e diz qual é.
func TestTodaRotaEstaDocumentada(t *testing.T) {
	env := newTestEnv(t)
	images := NewLegacyImages(env.db, env.files)
	cat := catalog.New(env.cat, env.files, images)
	srv := New(env.db, env.files, cat, boxes.New(env.box, images, env.files, cat, env.db))
	spec := specOperations(t)

	code := map[string]bool{}
	for _, rt := range srv.routes() {
		code[rt.method+" "+rt.path] = true
	}

	var missing, stale []string
	for op := range code {
		if !spec[op] {
			missing = append(missing, op)
		}
	}
	for op := range spec {
		if !code[op] {
			stale = append(stale, op)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)

	if len(missing) > 0 {
		t.Errorf("rotas sem documentação no openapi.yaml:\n  %s", strings.Join(missing, "\n  "))
	}
	if len(stale) > 0 {
		t.Errorf("documentadas no openapi.yaml mas não existem no código:\n  %s", strings.Join(stale, "\n  "))
	}
}

func TestDocsServidas(t *testing.T) {
	env := newTestEnv(t)

	rec := env.do("GET", "/openapi.yaml", nil, nil)
	assertStatus(t, rec, http.StatusOK)
	if !strings.HasPrefix(rec.Body.String(), "openapi: 3") {
		t.Errorf("openapi.yaml não começa com a versão: %.40q", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "yaml") {
		t.Errorf("Content-Type = %q", ct)
	}

	rec = env.do("GET", "/docs", nil, nil)
	assertStatus(t, rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), "SwaggerUIBundle") || !strings.Contains(rec.Body.String(), "/openapi.yaml") {
		t.Error("a página /docs não carrega o Swagger UI apontando para /openapi.yaml")
	}
}
