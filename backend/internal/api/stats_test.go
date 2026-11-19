package api

import (
	"net/http"
	"testing"

	"SIDM/internal/boxes"
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

// Vários pacotes na pesquisa, e as labels das tarefas com os mesmos filtros.
func TestPesquisaPacotesELabels(t *testing.T) {
	env := newTestEnv(t)
	env.withCapacete(t)
	assertStatus(t, env.doJSON("POST", "/packages", `{"name": "motos2", "profile": "capacete-moto"}`), http.StatusCreated)
	a := env.uploadTo(t, "motos", upFile{"a.jpg", "moto a"}, upFile{"b.jpg", "moto b"})
	c := env.uploadTo(t, "motos2", upFile{"c.jpg", "moto c"})
	for i, sha := range []string{a[0].SHA1, a[1].SHA1, c[0].SHA1} {
		dt := "day"
		if i == 0 {
			dt = "night"
		}
		assertStatus(t, env.doJSON("PUT", "/images/"+sha, `{"fields": {"day_time": "`+dt+`"}, "complete": true}`), http.StatusOK)
	}
	total := func(query string) int64 {
		t.Helper()
		rec := env.do("GET", "/images/stats?profile=capacete-moto"+query, nil, nil)
		assertStatus(t, rec, http.StatusOK)
		var st images.Stats
		decode(t, rec, &st)
		return st.Total
	}
	if n := total(""); n != 3 {
		t.Errorf("todos os pacotes: %d", n)
	}
	if n := total("&package=motos&package=motos2"); n != 3 {
		t.Errorf("os dois: %d", n)
	}
	if n := total("&package=motos2"); n != 1 {
		t.Errorf("motos2: %d", n)
	}
	if n := total("&package=motos2&package=" + testPkg); n != 1 {
		t.Errorf("motos2 + pacote de outro perfil: %d", n)
	}

	// caixas: capacete em a (2) e c (1); placa em b
	env.box.Tasks["capacete"] = boxes.Task{Name: "capacete", Labels: []string{"helmet", "no_helmet"}}
	env.box.Tasks["placa"] = boxes.Task{Name: "placa", Labels: []string{"placa"}}
	env.box.Put(boxes.Set{Task: "capacete", ImageID: a[0].SHA1, Package: "motos", Boxes: []boxes.Box{{Label: "helmet"}, {Label: "no_helmet"}}})
	env.box.Put(boxes.Set{Task: "capacete", ImageID: c[0].SHA1, Package: "motos2", Boxes: []boxes.Box{{Label: "helmet"}}})
	env.box.Put(boxes.Set{Task: "placa", ImageID: a[1].SHA1, Package: "motos", Boxes: []boxes.Box{{Label: "placa"}}})
	labels := func(query string) boxes.LabelStats {
		t.Helper()
		rec := env.do("GET", "/boxes/stats?profile=capacete-moto"+query, nil, nil)
		assertStatus(t, rec, http.StatusOK)
		var st boxes.LabelStats
		decode(t, rec, &st)
		return st
	}
	st := labels("")
	if st.Images != 3 || len(st.Tasks) != 2 || st.Tasks[0].Task != "capacete" || st.Tasks[0].Labels["helmet"].Boxes != 2 || st.Tasks[1].Labels["placa"].Images != 1 {
		t.Errorf("todas as tarefas: %+v", st)
	}
	// só a noite (a): na tarefa capacete, 1 imagem, helmet 1 e no_helmet 1
	st = labels("&task=capacete&fields.day_time=night")
	if st.Images != 1 || len(st.Tasks) != 1 || st.Tasks[0].Images != 1 || st.Tasks[0].Labels["helmet"].Boxes != 1 || st.Tasks[0].Labels["no_helmet"].Boxes != 1 {
		t.Errorf("noite: %+v", st)
	}
	// pacote motos2: só c
	if st = labels("&package=motos2&task=capacete&task=placa"); st.Images != 1 || st.Tasks[0].Boxes != 1 || st.Tasks[1].Boxes != 0 {
		t.Errorf("motos2: %+v", st)
	}
	assertStatus(t, env.do("GET", "/boxes/stats?profile=capacete-moto&task=nao-existe", nil, nil), http.StatusNotFound)
	assertStatus(t, env.do("GET", "/boxes/stats?profile=nao-existe", nil, nil), http.StatusNotFound)
	assertStatus(t, env.do("GET", "/boxes/stats", nil, nil), http.StatusBadRequest)
	assertStatus(t, env.do("GET", "/boxes/stats?profile=capacete-moto&fields.weather=rain", nil, nil), http.StatusBadRequest)
}
