package api

import (
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"SIDM/internal/store"
)

// next pede a próxima imagem do pacote de teste como `user`.
func (e *testEnv) next(t *testing.T, user, body string) (*store.Image, int) {
	t.Helper()
	headers := map[string]string{"Content-Type": "application/json"}
	if user != "" {
		headers["X-User"] = user
	}
	rec := e.do("POST", "/packages/"+testPkg+"/next", strings.NewReader(body), headers)
	if rec.Code != http.StatusOK {
		return nil, rec.Code
	}
	var img store.Image
	decode(t, rec, &img)
	return &img, rec.Code
}

// seedQueue sobe n imagens em ordem (a primeira é a mais antiga da fila).
func (e *testEnv) seedQueue(t *testing.T, contents ...string) []string {
	t.Helper()
	shas := make([]string, len(contents))
	for i, c := range contents {
		shas[i] = e.seedPending(t, c)
		img := e.db.images[shas[i]]
		img.QueuedAt = time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC) // ordem determinística
		e.db.images[shas[i]] = img
	}
	return shas
}

func TestNextDuasPessoasRecebemImagensDiferentes(t *testing.T) {
	env := newTestEnv(t)
	shas := env.seedQueue(t, "a", "b", "c")

	nathan, _ := env.next(t, "nathan", "")
	maria, _ := env.next(t, "maria", "")

	if nathan.SHA1 != shas[0] || maria.SHA1 != shas[1] {
		t.Errorf("nathan=%s maria=%s; want %s e %s", nathan.SHA1, maria.SHA1, shas[0], shas[1])
	}
	if nathan.ClaimedBy != "nathan" || nathan.ClaimedUntil == nil {
		t.Errorf("imagem não reservada: %+v", nathan)
	}
}

func TestNextRecarregarDevolveAMesma(t *testing.T) {
	env := newTestEnv(t)
	shas := env.seedQueue(t, "a", "b")

	first, _ := env.next(t, "nathan", "")
	again, _ := env.next(t, "nathan", "")
	if first.SHA1 != shas[0] || again.SHA1 != shas[0] {
		t.Errorf("recarregar deveria devolver a mesma imagem: %s depois %s", first.SHA1, again.SHA1)
	}
}

func TestNextPularMandaParaOFimDaFila(t *testing.T) {
	env := newTestEnv(t)
	shas := env.seedQueue(t, "a", "b", "c")

	a, _ := env.next(t, "nathan", "")
	b, _ := env.next(t, "nathan", `{"skip":"`+a.SHA1+`"}`)
	c, _ := env.next(t, "nathan", `{"skip":"`+b.SHA1+`"}`)
	back, _ := env.next(t, "nathan", `{"skip":"`+c.SHA1+`"}`)

	got := []string{a.SHA1, b.SHA1, c.SHA1, back.SHA1}
	want := []string{shas[0], shas[1], shas[2], shas[0]}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ordem ao pular = %v, want %v", got, want)
		}
	}
}

func TestNextFilaVazia(t *testing.T) {
	env := newTestEnv(t)
	sha := env.seedPending(t, "única")
	env.next(t, "nathan", "")

	if _, code := env.next(t, "maria", ""); code != http.StatusNoContent {
		t.Errorf("única imagem está com nathan: maria deveria receber 204, recebeu %d", code)
	}

	rec := env.do("PUT", "/images/"+sha, strings.NewReader(completeBody()),
		map[string]string{"Content-Type": "application/json", "X-User": "nathan"})
	assertStatus(t, rec, http.StatusOK)
	if _, code := env.next(t, "nathan", ""); code != http.StatusNoContent {
		t.Errorf("imagem verificada saiu da fila: esperava 204, veio %d", code)
	}
}

func TestNextPrazoVencidoLiberaAImagem(t *testing.T) {
	env := newTestEnv(t)
	shas := env.seedQueue(t, "a")
	env.next(t, "nathan", "")

	// Nathan sumiu: o prazo dele venceu.
	img := env.db.images[shas[0]]
	past := time.Now().Add(-time.Minute)
	img.ClaimedUntil = &past
	env.db.images[shas[0]] = img

	maria, code := env.next(t, "maria", "")
	if code != http.StatusOK || maria.SHA1 != shas[0] || maria.ClaimedBy != "maria" {
		t.Errorf("imagem com prazo vencido deveria ir para a maria: code=%d img=%+v", code, maria)
	}
}

func TestNextErros(t *testing.T) {
	env := newTestEnv(t)

	if _, code := env.next(t, "", ""); code != http.StatusBadRequest {
		t.Errorf("sem X-User: %d, want 400", code)
	}
	if _, code := env.next(t, "nathan", `{"skip":"nope"}`); code != http.StatusBadRequest {
		t.Errorf("skip inválido: %d, want 400", code)
	}
	if _, code := env.next(t, "nathan", `{"pular":"x"}`); code != http.StatusBadRequest {
		t.Errorf("campo desconhecido: %d, want 400", code)
	}
	rec := env.do("POST", "/packages/nao-existe/next", nil, map[string]string{"X-User": "nathan"})
	assertStatus(t, rec, http.StatusNotFound)
}

func TestAnnotateImagemDeOutraPessoa(t *testing.T) {
	env := newTestEnv(t)
	shas := env.seedQueue(t, "a")
	env.next(t, "nathan", "")

	rec := env.do("PUT", "/images/"+shas[0], strings.NewReader(`{"day_time":"night"}`),
		map[string]string{"Content-Type": "application/json", "X-User": "maria"})
	assertStatus(t, rec, http.StatusConflict)
	if !strings.Contains(errorMsg(t, rec), "nathan") {
		t.Errorf("a mensagem deveria dizer com quem está: %q", errorMsg(t, rec))
	}
}

func TestAnnotateRascunhoRenovaOPrazo(t *testing.T) {
	env := newTestEnv(t)
	shas := env.seedQueue(t, "a")
	env.next(t, "nathan", "")

	soon := time.Now().Add(time.Minute) // quase vencendo
	img := env.db.images[shas[0]]
	img.ClaimedUntil = &soon
	env.db.images[shas[0]] = img

	rec := env.do("PUT", "/images/"+shas[0], strings.NewReader(`{"day_time":"night"}`),
		map[string]string{"Content-Type": "application/json", "X-User": "nathan"})
	assertStatus(t, rec, http.StatusOK)

	got := env.db.images[shas[0]]
	if got.ClaimedBy != "nathan" || !got.ClaimedUntil.After(time.Now().Add(10*time.Minute)) {
		t.Errorf("rascunho deveria renovar o prazo: %+v", got)
	}
}

func TestAnnotateVerificarTiraDaFila(t *testing.T) {
	env := newTestEnv(t)
	shas := env.seedQueue(t, "a")
	env.next(t, "nathan", "")

	rec := env.do("PUT", "/images/"+shas[0], strings.NewReader(completeBody()),
		map[string]string{"Content-Type": "application/json", "X-User": "nathan"})
	assertStatus(t, rec, http.StatusOK)

	got := env.db.images[shas[0]]
	if got.ClaimedBy != "" || got.ClaimedUntil != nil {
		t.Errorf("verificada deveria sair da fila: %+v", got)
	}
}

func TestAnnotateConflitoOptimisticLocking(t *testing.T) {
	env := newTestEnv(t)
	sha := env.seedPending(t, "a")

	// Entre o nosso "ler" e o nosso "gravar", outra pessoa grava.
	env.db.beforeReplace = func() {
		env.db.mu.Lock()
		img := env.db.images[sha]
		img.UpdatedAt = time.Now().Add(time.Second)
		env.db.images[sha] = img
		env.db.mu.Unlock()
	}

	rec := env.doJSON("PUT", "/images/"+sha, completeBody())
	assertStatus(t, rec, http.StatusConflict)

	if _, err := os.Stat(env.pkgFile(sha + ".jpg")); err != nil {
		t.Error("em conflito, o rename deveria ser desfeito")
	}
}

func TestRelease(t *testing.T) {
	env := newTestEnv(t)
	shas := env.seedQueue(t, "a")
	env.next(t, "nathan", "")

	// Só o dono libera.
	env.do("POST", "/images/"+shas[0]+"/release", nil, map[string]string{"X-User": "maria"})
	if env.db.images[shas[0]].ClaimedBy != "nathan" {
		t.Fatal("maria não pode liberar a imagem do nathan")
	}

	rec := env.do("POST", "/images/"+shas[0]+"/release", nil, map[string]string{"X-User": "nathan"})
	assertStatus(t, rec, http.StatusNoContent)
	if env.db.images[shas[0]].ClaimedBy != "" {
		t.Error("release não liberou")
	}

	maria, _ := env.next(t, "maria", "")
	if maria == nil || maria.SHA1 != shas[0] {
		t.Error("depois do release, a imagem deveria estar livre para a maria")
	}

	assertStatus(t, env.do("POST", "/images/"+shas[0]+"/release", nil, nil), http.StatusBadRequest)
}

func TestPackagesContaQuemEstaAnotando(t *testing.T) {
	env := newTestEnv(t)
	env.seedQueue(t, "a", "b", "c")
	env.next(t, "nathan", "")
	env.next(t, "maria", "")

	var pkgs []store.PackageSummary
	decode(t, env.do("GET", "/packages", nil, nil), &pkgs)
	if pkgs[0].InProgress != 2 || pkgs[0].Pending != 3 {
		t.Errorf("in_progress=%d pending=%d; want 2 e 3", pkgs[0].InProgress, pkgs[0].Pending)
	}
}
