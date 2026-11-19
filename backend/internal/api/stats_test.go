package api

import (
	"net/http"
	"testing"

	"SIDM/internal/images"
)

// A pesquisa conta só as imagens do perfil pedido: uma imagem do pacote de
// veículos nunca entra na conta do capacete.
func TestPesquisaPorPerfil(t *testing.T) {
	env := newTestEnv(t)
	env.withCapacete(t)
	res := env.uploadTo(t, "motos", upFile{"m1.jpg", "moto 1"}, upFile{"m2.jpg", "moto 2"}, upFile{"m3.jpg", "moto 3"})
	assertStatus(t, env.doJSON("PUT", "/images/"+res[0].SHA1,
		`{"fields": {"day_time": "night", "riders": 2}, "objects": [{"type": "plate", "fields": {"text": "abc1d23"}}], "complete": true}`), http.StatusOK)
	assertStatus(t, env.doJSON("PUT", "/images/"+res[1].SHA1, `{"fields": {"day_time": "day"}, "complete": true}`), http.StatusOK)
	assertStatus(t, env.doJSON("PUT", "/images/"+res[2].SHA1, `{"fields": {"day_time": "night"}}`), http.StatusOK) // rascunho
	env.upload(t, upFile{"carro.jpg", "um carro"})                                                                 // pacote de veículos

	stats := func(query string) images.Stats {
		t.Helper()
		rec := env.do("GET", "/images/stats?profile=capacete-moto"+query, nil, nil)
		assertStatus(t, rec, http.StatusOK)
		var st images.Stats
		decode(t, rec, &st)
		return st
	}

	// padrão: só as verificadas
	st := stats("")
	if st.Profile != "capacete-moto" || st.Total != 2 || st.Packages["motos"] != 2 || st.Fields["day_time"]["night"] != 1 || st.Fields["day_time"]["day"] != 1 {
		t.Errorf("verificadas: %+v", st)
	}
	if p := st.Objects["plate"]; p == nil || p.Images != 1 || p.Count != 1 {
		t.Errorf("placas: %+v", st.Objects)
	}
	if _, ok := st.Objects["helmet"]; ok {
		t.Error("tipo com caixa (CVAT) não entra")
	}
	if _, ok := st.Fields["riders"]; ok {
		t.Error("número não é contado por valor")
	}

	// com as pendentes; o carro (outro perfil) nunca entra
	if st = stats("&status=all"); st.Total != 3 || st.Status["verified"] != 2 || st.Status["pending"] != 1 {
		t.Errorf("todas: %+v", st)
	}
	// filtro pelo campo (pelo nome da opção também)
	if st = stats("&fields.day_time=night"); st.Total != 1 {
		t.Errorf("noite: %+v", st)
	}
	if st = stats("&status=all&fields.day_time=NIGHT"); st.Total != 2 {
		t.Errorf("noite, todas: %+v", st)
	}
	if st = stats("&objects.plate.text=abc1d23"); st.Total != 1 {
		t.Errorf("pela placa (em maiúsculas): %+v", st)
	}
	// pacote de outro perfil: zero, com os campos do perfil
	if st = stats("&status=all&package=" + testPkg); st.Total != 0 || st.Fields["day_time"] == nil {
		t.Errorf("outro pacote: %+v", st)
	}

	// erros
	assertStatus(t, env.do("GET", "/images/stats", nil, nil), http.StatusBadRequest)
	assertStatus(t, env.do("GET", "/images/stats?profile=nao-existe", nil, nil), http.StatusNotFound)
	for _, q := range []string{"&fields.weather=rain", "&fields.day_time=tarde", "&objects.helmet.color=black", "&objects.plate=x", "&objects.plate.cor=x", "&status=x"} {
		if rec := env.do("GET", "/images/stats?profile=capacete-moto"+q, nil, nil); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", q, rec.Code, rec.Body.String())
		}
	}
}
