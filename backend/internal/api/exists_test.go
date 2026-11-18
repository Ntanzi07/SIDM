package api

import (
	"SIDM/internal/images"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestExists(t *testing.T) {
	env := newTestEnv(t)
	known := env.seedPending(t, "já existe")
	unknown := sha1Of("não existe")

	body := fmt.Sprintf(`{"sha1s":[%q, %q, %q]}`, strings.ToUpper(known), unknown, unknown)
	rec := env.doJSON("POST", "/images/exists", body)
	assertStatus(t, rec, http.StatusOK)

	var resp images.ExistsResponse
	decode(t, rec, &resp)
	if len(resp.Existing) != 1 || resp.Existing[0].SHA1 != known || resp.Existing[0].Status != "pending" || resp.Existing[0].Package != testPkg {
		t.Errorf("existing = %+v", resp.Existing)
	}
	if len(resp.Missing) != 1 || resp.Missing[0] != unknown {
		t.Errorf("missing = %v (repetidos deveriam ser removidos)", resp.Missing)
	}
}

func TestExistsInvalido(t *testing.T) {
	tooMany := make([]string, images.MaxExistsBatch+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("%q", sha1Of(fmt.Sprint(i)))
	}

	cases := map[string]string{
		"lista vazia":        `{"sha1s":[]}`,
		"hash inválido":      `{"sha1s":["nope"]}`,
		"campo errado":       `{"hashes":["x"]}`,
		"JSON quebrado":      `{`,
		"lote grande demais": `{"sha1s":[` + strings.Join(tooMany, ",") + `]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			env := newTestEnv(t)
			assertStatus(t, env.doJSON("POST", "/images/exists", body), http.StatusBadRequest)
		})
	}
}
