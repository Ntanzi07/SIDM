package api

import (
	"net/http"
	"strings"
	"testing"

	"SIDM/internal/export"
	"SIDM/internal/store"
)

func TestTarefasCRUD(t *testing.T) {
	env := newTestEnv(t)

	rec := env.doJSON("POST", "/tasks", `{"name":"capacete","description":" motos ","labels":[" com_capacete ","sem_capacete","com_capacete",""]}`)
	assertStatus(t, rec, http.StatusCreated)
	task := env.db.tasks["capacete"]
	if task.Description != "motos" || strings.Join(task.Labels, ",") != "com_capacete,sem_capacete" || task.CreatedBy == "" {
		t.Errorf("tarefa = %+v", task)
	}
	assertStatus(t, env.doJSON("POST", "/tasks", `{"name":"capacete"}`), http.StatusConflict)
	for _, bad := range []string{`{"name":"Com Espaço"}`, `{"name":""}`, `{"name":"-x"}`, `{"nome":"x"}`, `{"name":"x","labels":["` + strings.Repeat("a", 65) + `"]}`} {
		if rec := env.doJSON("POST", "/tasks", bad); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", bad, rec.Code)
		}
	}

	// resumo: a verificada, b pendente
	a := env.seedPending(t, "a")
	assertStatus(t, env.doJSON("PUT", "/images/"+a, completeBody()), http.StatusOK)
	b := env.seedPending(t, "b")
	putTask(env, "capacete", a, "com_capacete", "com_capacete")
	putTask(env, "capacete", b)

	var view struct {
		store.Task
		Stats store.TaskStats `json:"stats"`
	}
	decode(t, env.do("GET", "/tasks/capacete", nil, nil), &view)
	st := view.Stats
	if st.Images != 2 || st.Verified != 1 || st.Negatives != 1 || st.Labels["com_capacete"] != 2 ||
		len(st.Packages) != 1 || st.Packages[0].Name != testPkg || st.Packages[0].Verified != 1 {
		t.Errorf("stats = %+v", st)
	}
	assertStatus(t, env.do("GET", "/tasks/nao-existe", nil, nil), http.StatusNotFound)

	var list page[taskView]
	decode(t, env.do("GET", "/tasks", nil, nil), &list)
	if list.Total != 1 || list.Items[0].Stats.Images != 2 {
		t.Errorf("lista = %+v", list)
	}

	// renomear leva caixas e perfis de export junto
	env.db.exportProf["p"] = export.Profile{Name: "p", Task: "capacete"}
	assertStatus(t, env.doJSON("POST", "/tasks", `{"name":"letras"}`), http.StatusCreated)
	assertStatus(t, env.doJSON("PUT", "/tasks/capacete", `{"name":"letras"}`), http.StatusConflict)
	assertStatus(t, env.doJSON("PUT", "/tasks/capacete", `{"name":"moto-capacete","labels":["com_capacete"]}`), http.StatusOK)
	if _, ok := env.boxSet("moto-capacete", a); !ok {
		t.Error("as caixas deveriam ir para o nome novo")
	}
	if env.db.exportProf["p"].Task != "moto-capacete" {
		t.Error("o perfil de export deveria apontar para o nome novo")
	}
	if got := env.db.tasks["moto-capacete"]; got.CreatedBy != task.CreatedBy || len(got.Labels) != 1 {
		t.Errorf("tarefa renomeada = %+v", got)
	}
	assertStatus(t, env.doJSON("PUT", "/tasks/sumiu", `{"name":"x"}`), http.StatusNotFound)

	rec = env.do("DELETE", "/tasks/moto-capacete", nil, nil)
	assertStatus(t, rec, http.StatusOK)
	if len(env.db.boxSets) != 0 {
		t.Error("apagar a tarefa deveria apagar as caixas dela")
	}
	assertStatus(t, env.do("DELETE", "/tasks/moto-capacete", nil, nil), http.StatusNotFound)
}
