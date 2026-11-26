package api

import (
	"net/http"
	"testing"

	"SIDM/internal/images"
)

func TestSistemaDiscoELixeira(t *testing.T) {
	env := newTestEnv(t)
	assertStatus(t, env.doJSON("POST", "/packages", `{"name": "p"}`), http.StatusCreated)
	res := env.uploadTo(t, "p", upFile{"a.jpg", realJPEG(t, 32, 32, 7)}, upFile{"b.jpg", realJPEG(t, 32, 32, 8)})
	assertStatus(t, env.do("DELETE", "/images/"+res[0].SHA1, nil, nil), http.StatusOK)

	var st images.Storage
	rec := env.do("GET", "/system/storage", nil, nil)
	assertStatus(t, rec, http.StatusOK)
	decode(t, rec, &st)
	if st.Disk.Total == 0 || st.Disk.Free == 0 || st.Disk.Free > st.Disk.Total || st.Trash.Count != 1 || st.Trash.Bytes == 0 || st.Uploads.Items == nil {
		t.Fatalf("resumo: %+v", st)
	}

	// mais de 30 dias: nada (acabou de ir para a lixeira)
	var purge images.PurgeResult
	rec = env.doJSON("POST", "/system/trash/purge", `{"older_than_days": 30}`)
	assertStatus(t, rec, http.StatusOK)
	decode(t, rec, &purge)
	if purge.Images != 0 {
		t.Errorf("30 dias: %+v", purge)
	}
	// tudo
	rec = env.doJSON("POST", "/system/trash/purge", `{"older_than_days": 0}`)
	decode(t, rec, &purge)
	if purge.Images != 1 || purge.Bytes == 0 {
		t.Errorf("tudo: %+v", purge)
	}
	decode(t, env.do("GET", "/system/storage", nil, nil), &st)
	if st.Trash.Count != 0 {
		t.Errorf("lixeira depois: %+v", st.Trash)
	}
	assertStatus(t, env.doJSON("POST", "/system/trash/purge", `{"older_than_days": -1}`), http.StatusBadRequest)
}
