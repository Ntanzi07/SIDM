package api

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"SIDM/internal/images"
)

func makeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(content))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestEnvioDeZipEmPartes(t *testing.T) {
	env := newTestEnv(t)
	assertStatus(t, env.doJSON("POST", "/packages", `{"name": "grande"}`), http.StatusCreated)
	data := makeZip(t, map[string]string{
		"pasta/a.jpg": realJPEG(t, 32, 32, 1), "pasta/sub/b.jpg": realJPEG(t, 32, 32, 2), "c.jpg": realJPEG(t, 32, 32, 3),
		"dup.jpg": realJPEG(t, 32, 32, 1), "leia.txt": "não é imagem", "__MACOSX/._a.jpg": "lixo do mac", "pasta/": "",
	})
	start := fmt.Sprintf(`{"file_name": "fotos.zip", "size": %d}`, len(data))
	rec := env.doJSON("POST", "/packages/grande/zip-uploads", start)
	assertStatus(t, rec, http.StatusOK)
	var z images.ZipUpload
	decode(t, rec, &z)
	if z.Status != images.ZipReceiving || z.Received != 0 {
		t.Fatalf("início: %+v", z)
	}
	put := func(offset int, part []byte) *httpResult {
		return env.raw("PUT", fmt.Sprintf("/zip-uploads/%s/chunk?offset=%d", z.ID, offset), part)
	}
	half := len(data) / 2
	if res := put(0, data[:half]); res.code != http.StatusOK {
		t.Fatalf("parte 1: %d %s", res.code, res.body)
	}
	// parte fora de ordem: 409 dizendo onde parou
	if res := put(0, data[half:]); res.code != http.StatusConflict || !strings.Contains(res.body, fmt.Sprintf(`"received":%d`, half)) {
		t.Errorf("fora de ordem: %d %s", res.code, res.body)
	}
	// terminar antes de chegar tudo: 400
	assertStatus(t, env.doJSON("POST", "/zip-uploads/"+z.ID+"/finish", ""), http.StatusBadRequest)
	// escolher o mesmo zip de novo: o mesmo envio, para retomar
	rec = env.doJSON("POST", "/packages/grande/zip-uploads", start)
	var again images.ZipUpload
	decode(t, rec, &again)
	if again.ID != z.ID || again.Received != int64(half) {
		t.Errorf("retomar: %+v", again)
	}
	if res := put(half, data[half:]); res.code != http.StatusOK {
		t.Fatalf("parte 2: %d %s", res.code, res.body)
	}
	assertStatus(t, env.doJSON("POST", "/zip-uploads/"+z.ID+"/finish", ""), http.StatusAccepted)

	deadline := time.Now().Add(5 * time.Second)
	for {
		rec = env.do("GET", "/zip-uploads/"+z.ID, nil, nil)
		decode(t, rec, &z)
		if z.Status == images.ZipDone || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if z.Status != images.ZipDone || z.Total != 4 || z.Created != 3 || z.Duplicates != 1 || z.Skipped != 2 || z.ErrorCount != 0 || z.Running {
		t.Fatalf("fim: %+v", z)
	}
	rec = env.do("GET", "/images?package=grande", nil, nil)
	if !strings.Contains(rec.Body.String(), `"original_name":"b.jpg"`) {
		t.Errorf("o nome dentro da pasta do zip: %s", rec.Body.String())
	}
	// o zip some do disco depois de gravado; o envio continua na lista
	rec = env.do("GET", "/packages/grande/zip-uploads", nil, nil)
	if !strings.Contains(rec.Body.String(), z.ID) {
		t.Errorf("lista: %s", rec.Body.String())
	}
	assertStatus(t, env.do("DELETE", "/zip-uploads/"+z.ID, nil, nil), http.StatusOK)
	assertStatus(t, env.do("GET", "/zip-uploads/"+z.ID, nil, nil), http.StatusNotFound)
}

func TestEnvioDeZipCorrompido(t *testing.T) {
	env := newTestEnv(t)
	assertStatus(t, env.doJSON("POST", "/packages", `{"name": "x"}`), http.StatusCreated)
	rec := env.doJSON("POST", "/packages/x/zip-uploads", `{"file_name": "ruim.zip", "size": 5}`)
	var z images.ZipUpload
	decode(t, rec, &z)
	env.raw("PUT", "/zip-uploads/"+z.ID+"/chunk?offset=0", []byte("12345"))
	assertStatus(t, env.doJSON("POST", "/zip-uploads/"+z.ID+"/finish", ""), http.StatusAccepted)
	for i := 0; i < 100 && z.Status != images.ZipFailed; i++ {
		time.Sleep(10 * time.Millisecond)
		decode(t, env.do("GET", "/zip-uploads/"+z.ID, nil, nil), &z)
	}
	if z.Status != images.ZipFailed || z.Error == "" {
		t.Errorf("corrompido: %+v", z)
	}
	assertStatus(t, env.doJSON("POST", "/packages/x/zip-uploads", `{"file_name": "nao.rar", "size": 5}`), http.StatusBadRequest)
}

type httpResult struct {
	code int
	body string
}

func (e *testEnv) raw(method, path string, body []byte) *httpResult {
	rec := e.do(method, path, bytes.NewReader(body), map[string]string{"Content-Type": "application/octet-stream"})
	return &httpResult{rec.Code, rec.Body.String()}
}
