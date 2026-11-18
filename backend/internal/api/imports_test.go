package api

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"SIDM/internal/annotation"
	"SIDM/internal/images"
	"SIDM/internal/importer"
	"SIDM/internal/imports"
	"SIDM/internal/platform"
)

const testProfileJSON = `{
  "name": "csv-teste",
  "reader": { "type": "csv", "delimiter": ";" },
  "match": { "column": "Arquivo", "by": "original_name" },
  "fields": {
    "equipment":   { "value": "8003-MF4739" },
    "day_time":    { "column": "Turno", "map": { "D": "day", "N": "night" } },
    "weather":     { "column": "Clima", "map": { "Limpo": "clear", "Chuva": "rain" } },
    "captured_at": { "column": "DataHora", "date_format": "02/01/2006 15:04:05", "timezone": "America/Sao_Paulo" }
  },
  "vehicles": {
    "mode": "row",
    "fields": {
      "plate": { "column": "Placa" },
      "type":  { "column": "Tipo", "map": { "Carro": "car", "Caminhão": "truck" } }
    }
  }
}`

// postFile manda um arquivo no campo "file".
func (e *testEnv) postFile(t *testing.T, path, field, content string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	w, _ := mw.CreateFormFile(field, "dados.csv")
	w.Write([]byte(content))
	mw.Close()
	return e.do("POST", path, &body, map[string]string{"Content-Type": mw.FormDataContentType(), "X-User": "nathan"})
}

// withProfile cadastra o perfil de teste.
func (e *testEnv) withProfile(t *testing.T) {
	t.Helper()
	rec := e.do("POST", "/import-profiles", strings.NewReader(testProfileJSON),
		map[string]string{"Content-Type": "application/json", "X-User": "nathan"})
	assertStatus(t, rec, http.StatusCreated)
}

// seedNamed sobe uma imagem com um nome original específico.
func (e *testEnv) seedNamed(t *testing.T, name, content string) string {
	t.Helper()
	res := e.upload(t, upFile{name, content})
	if res[0].Status != "created" {
		t.Fatalf("seed %s: %+v", name, res[0])
	}
	return res[0].SHA1
}

// ---------- perfis ----------

func TestCreateProfile(t *testing.T) {
	env := newTestEnv(t)
	env.withProfile(t)

	p := env.imp.Profiles["csv-teste"]
	if p.CreatedBy != "nathan" || p.UpdatedAt.IsZero() {
		t.Errorf("created_by/updated_at não preenchidos: %+v", p)
	}
	if p.Fields["day_time"].Map["N"] != "night" {
		t.Error("o perfil não foi salvo inteiro")
	}

	// de novo: duplicado
	rec := env.doJSON("POST", "/import-profiles", testProfileJSON)
	assertStatus(t, rec, http.StatusConflict)
	if !strings.Contains(errorMsg(t, rec), "PUT") {
		t.Errorf("a mensagem deveria sugerir o PUT: %q", errorMsg(t, rec))
	}
}

func TestCreateProfileInvalido(t *testing.T) {
	cases := map[string]struct {
		body string
		want string
	}{
		"campo com nome errado": {strings.Replace(testProfileJSON, `"column": "Turno"`, `"colum": "Turno"`, 1), "colum"},
		"valor de map inválido": {strings.Replace(testProfileJSON, `"N": "night"`, `"N": "noite"`, 1), "noite"},
		"leitor desconhecido":   {strings.Replace(testProfileJSON, `"type": "csv"`, `"type": "xlsx"`, 1), "reader.type"},
		"JSON quebrado":         {`{"name":`, "JSON inválido"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			env := newTestEnv(t)
			rec := env.doJSON("POST", "/import-profiles", tc.body)
			assertStatus(t, rec, http.StatusBadRequest)
			if !strings.Contains(errorMsg(t, rec), tc.want) {
				t.Errorf("mensagem %q não menciona %q", errorMsg(t, rec), tc.want)
			}
		})
	}
}

func TestReplaceProfile(t *testing.T) {
	env := newTestEnv(t)
	env.withProfile(t)

	updated := strings.Replace(testProfileJSON, `"Chuva": "rain"`, `"Chuva": "rain", "Garoa": "rain"`, 1)
	rec := env.do("PUT", "/import-profiles/csv-teste", strings.NewReader(updated),
		map[string]string{"Content-Type": "application/json", "X-User": "maria"})
	assertStatus(t, rec, http.StatusOK)

	p := env.imp.Profiles["csv-teste"]
	if p.Fields["weather"].Map["Garoa"] != "rain" {
		t.Error("PUT não atualizou o map")
	}
	if p.CreatedBy != "nathan" {
		t.Errorf("created_by deveria continuar sendo quem criou: %q", p.CreatedBy)
	}

	t.Run("nome diferente da URL", func(t *testing.T) {
		assertStatus(t, env.doJSON("PUT", "/import-profiles/outro", testProfileJSON), http.StatusBadRequest)
	})
	t.Run("perfil inexistente", func(t *testing.T) {
		body := strings.Replace(testProfileJSON, `"csv-teste"`, `"nao-existe"`, 1)
		assertStatus(t, env.doJSON("PUT", "/import-profiles/nao-existe", body), http.StatusNotFound)
	})
}

func TestRenomearPerfil(t *testing.T) {
	env := newTestEnv(t)
	env.withProfile(t)

	rec := env.doJSON("PATCH", "/import-profiles/csv-teste", `{"name":"csv-equipamento-8003"}`)
	assertStatus(t, rec, http.StatusOK)
	var p importer.Profile
	decode(t, rec, &p)
	if p.Name != "csv-equipamento-8003" || p.Fields["day_time"].Map["N"] != "night" {
		t.Errorf("perfil renomeado = %+v", p)
	}
	if _, ok := env.imp.Profiles["csv-teste"]; ok {
		t.Error("o nome antigo deveria ter sumido")
	}
	if env.imp.Profiles["csv-equipamento-8003"].CreatedBy != "nathan" {
		t.Error("renomear não pode mudar quem criou")
	}

	// e o import passa a funcionar pelo nome novo
	env.seedNamed(t, "img001.jpg", "a")
	csv := "Arquivo;Turno;Clima;DataHora;Placa;Tipo\nimg001.jpg;N;Chuva;03/09/2026 08:25:23;ABC1D23;Carro\n"
	assertStatus(t, env.postFile(t, "/packages/"+testPkg+"/import?profile=csv-equipamento-8003", "file", csv), http.StatusOK)
	assertStatus(t, env.postFile(t, "/packages/"+testPkg+"/import?profile=csv-teste", "file", csv), http.StatusNotFound)
}

func TestRenomearPerfilErros(t *testing.T) {
	env := newTestEnv(t)
	env.withProfile(t)
	env.imp.Profiles["ocupado"] = importer.Profile{Name: "ocupado"}

	cases := map[string]struct {
		path, body string
		status     int
	}{
		"nome já existe":    {"/import-profiles/csv-teste", `{"name":"ocupado"}`, http.StatusConflict},
		"nome com espaço":   {"/import-profiles/csv-teste", `{"name":"csv teste"}`, http.StatusBadRequest},
		"nome com barra":    {"/import-profiles/csv-teste", `{"name":"../x"}`, http.StatusBadRequest},
		"nome vazio":        {"/import-profiles/csv-teste", `{"name":""}`, http.StatusBadRequest},
		"mesmo nome":        {"/import-profiles/csv-teste", `{"name":"csv-teste"}`, http.StatusBadRequest},
		"conteúdo no PATCH": {"/import-profiles/csv-teste", testProfileJSON, http.StatusBadRequest},
		"inexistente":       {"/import-profiles/nao-existe", `{"name":"x"}`, http.StatusNotFound},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assertStatus(t, env.doJSON("PATCH", tc.path, tc.body), tc.status)
		})
	}
	if _, ok := env.imp.Profiles["csv-teste"]; !ok {
		t.Error("nenhum erro pode ter mexido no perfil")
	}
}

func TestApagarPerfil(t *testing.T) {
	env := newTestEnv(t)
	env.withProfile(t)

	rec := env.do("DELETE", "/import-profiles/csv-teste", nil, nil)
	assertStatus(t, rec, http.StatusOK)
	if _, ok := env.imp.Profiles["csv-teste"]; ok {
		t.Error("o perfil deveria ter sido apagado")
	}
	assertStatus(t, env.do("GET", "/import-profiles/csv-teste", nil, nil), http.StatusNotFound)
	assertStatus(t, env.do("DELETE", "/import-profiles/csv-teste", nil, nil), http.StatusNotFound)
}

func TestListEGetProfiles(t *testing.T) {
	env := newTestEnv(t)
	env.withProfile(t)

	var resp platform.Page[importer.Profile]
	decode(t, env.do("GET", "/import-profiles?q=CSV", nil, nil), &resp)
	if resp.Total != 1 || resp.Items[0].Name != "csv-teste" {
		t.Errorf("lista = %+v", resp)
	}

	assertStatus(t, env.do("GET", "/import-profiles/csv-teste", nil, nil), http.StatusOK)
	assertStatus(t, env.do("GET", "/import-profiles/nao-existe", nil, nil), http.StatusNotFound)
}

// ---------- inspeção ----------

func TestInspect(t *testing.T) {
	env := newTestEnv(t)
	csv := "Arquivo;Clima\na.jpg;Chuva\nb.jpg;Limpo\nc.jpg;Chuva\n"

	// O nome e o caractere codificado funcionam. (";" cru o Go descarta.)
	for _, delim := range []string{"semicolon", "%3B"} {
		t.Run(delim, func(t *testing.T) {
			rec := env.postFile(t, "/imports/inspect?delimiter="+delim, "file", csv)
			assertStatus(t, rec, http.StatusOK)

			var ins importer.Inspection
			decode(t, rec, &ins)
			if ins.Rows != 3 || len(ins.Columns) != 2 {
				t.Errorf("inspeção = %+v", ins)
			}
			if ins.Distinct["Clima"].Values[0].Value != "Chuva" || ins.Distinct["Clima"].Values[0].Count != 2 {
				t.Errorf("distintos de Clima = %+v", ins.Distinct["Clima"])
			}
		})
	}
}

func TestInspectErros(t *testing.T) {
	env := newTestEnv(t)

	t.Run("sem o campo file", func(t *testing.T) {
		assertStatus(t, env.postFile(t, "/imports/inspect", "outro", "A\n1\n"), http.StatusBadRequest)
	})
	t.Run("separador errado dá dica", func(t *testing.T) {
		rec := env.postFile(t, "/imports/inspect", "file", "A;B\n1;2\n")
		assertStatus(t, rec, http.StatusBadRequest)
		if !strings.Contains(errorMsg(t, rec), "delimiter") {
			t.Errorf("a mensagem deveria sugerir o delimiter: %q", errorMsg(t, rec))
		}
	})
	t.Run("delimitador desconhecido", func(t *testing.T) {
		assertStatus(t, env.postFile(t, "/imports/inspect?delimiter=hashtag", "file", "A\n1\n"), http.StatusBadRequest)
	})
	t.Run("tab", func(t *testing.T) {
		assertStatus(t, env.postFile(t, "/imports/inspect?delimiter=tab", "file", "A\tB\n1\t2\n"), http.StatusOK)
	})
	t.Run("não é multipart", func(t *testing.T) {
		assertStatus(t, env.doJSON("POST", "/imports/inspect", `{}`), http.StatusBadRequest)
	})
}

// ---------- import ----------

const importCSV = `Arquivo;Turno;Clima;DataHora;Placa;Tipo
a.jpg;N;Chuva;03/09/2026 08:25:23;ABC1D23;Caminhão
a.jpg;N;Chuva;03/09/2026 08:25:23;KOY7386;Carro
b.jpg;D;Garoa;03/09/2026 09:00:00;;
nao-existe.jpg;D;Limpo;03/09/2026 09:00:00;;
`

func decodeImport(t *testing.T, rec *httptest.ResponseRecorder) imports.Report {
	t.Helper()
	assertStatus(t, rec, http.StatusOK)
	var rep imports.Report
	decode(t, rec, &rep)
	return rep
}

func TestImportEnsaioNaoGrava(t *testing.T) {
	env := newTestEnv(t)
	env.withProfile(t)
	shaA := env.seedNamed(t, "a.jpg", "conteudo A")
	env.seedNamed(t, "b.jpg", "conteudo B")

	// sem dry_run na URL: o padrão é ENSAIO
	rep := decodeImport(t, env.postFile(t, "/packages/"+testPkg+"/import?profile=csv-teste", "file", importCSV))

	if !rep.DryRun {
		t.Fatal("sem dry_run=false, o padrão tem que ser ensaio")
	}
	if rep.Rows != 4 || rep.Records != 3 || rep.Matched != 2 || rep.Applied != 2 {
		t.Errorf("rows=%d records=%d matched=%d applied=%d", rep.Rows, rep.Records, rep.Matched, rep.Applied)
	}
	if rep.NotFoundCount != 1 || rep.NotFound[0] != "nao-existe.jpg" {
		t.Errorf("not_found = %v (%d)", rep.NotFound, rep.NotFoundCount)
	}
	if rep.Unmapped["weather"]["Garoa"] != 1 {
		t.Errorf("Garoa deveria aparecer sem tradução: %v", rep.Unmapped)
	}
	if fv(env.img.Images[shaA].Fields, "day_time") != "" {
		t.Error("o ensaio gravou no rascunho!")
	}
	if len(env.img.History) != 0 {
		t.Error("o ensaio gravou histórico!")
	}
}

func TestImportAplica(t *testing.T) {
	env := newTestEnv(t)
	env.withProfile(t)
	shaA := env.seedNamed(t, "a.jpg", "conteudo A")
	shaB := env.seedNamed(t, "b.jpg", "conteudo B")

	rep := decodeImport(t, env.postFile(t, "/packages/"+testPkg+"/import?profile=csv-teste&dry_run=false", "file", importCSV))
	if rep.DryRun || rep.Applied != 2 {
		t.Fatalf("dry_run=%v applied=%d", rep.DryRun, rep.Applied)
	}

	a := env.img.Images[shaA]
	if a.Status != images.StatusPending {
		t.Error("o import NUNCA verifica: a imagem tem que continuar pending")
	}
	if fv(a.Fields, "equipment") != "8003-MF4739" || fv(a.Fields, "day_time") != "night" || fv(a.Fields, "weather") != "rain" {
		t.Errorf("campos não preenchidos: %+v", a)
	}
	if len(a.Objects) != 2 || fv(a.Objects[0].Fields, "plate") != "ABC1D23" || fv(a.Objects[1].Fields, "type") != "car" {
		t.Errorf("veículos (duas linhas agrupadas) = %+v", a.Objects)
	}
	if a.Fields["captured_at"] != time.Date(2026, 9, 3, 11, 25, 23, 0, time.UTC) {
		t.Errorf("captured_at = %v (08:25 em São Paulo = 11:25 UTC)", a.Fields["captured_at"])
	}

	b := env.img.Images[shaB]
	if fv(b.Fields, "weather") != "" || fv(b.Fields, "day_time") != "day" {
		t.Errorf("b: Garoa (sem tradução) deveria ficar vazio: %+v", b)
	}

	var byImport bool
	for _, h := range env.img.History {
		if h.ChangedBy == "nathan via import:csv-teste" {
			byImport = true
		}
	}
	if !byImport {
		t.Error("o histórico deveria registrar que veio do import")
	}

	// Rodar de novo: nada novo para aplicar.
	again := decodeImport(t, env.postFile(t, "/packages/"+testPkg+"/import?profile=csv-teste&dry_run=false", "file", importCSV))
	if again.Applied != 0 || again.Unchanged != 2 {
		t.Errorf("segunda vez: applied=%d unchanged=%d; want 0 e 2", again.Applied, again.Unchanged)
	}
}

func TestImportRespeitaQuemJaAnotou(t *testing.T) {
	env := newTestEnv(t)
	env.withProfile(t)
	sha := env.seedNamed(t, "a.jpg", "conteudo A")

	// Alguém já anotou o clima como "fog".
	assertStatus(t, env.doJSON("PUT", "/images/"+sha, `{"fields": {"weather":"fog"}}`), http.StatusOK)

	decodeImport(t, env.postFile(t, "/packages/"+testPkg+"/import?profile=csv-teste&dry_run=false", "file", importCSV))
	img := env.img.Images[sha]
	if fv(img.Fields, "weather") != "fog" {
		t.Errorf("overwrite=false: o clima anotado deveria ser mantido, virou %q", fv(img.Fields, "weather"))
	}
	if fv(img.Fields, "day_time") != "night" {
		t.Error("os campos vazios deveriam ser preenchidos")
	}

	decodeImport(t, env.postFile(t, "/packages/"+testPkg+"/import?profile=csv-teste&dry_run=false&overwrite=true", "file", importCSV))
	if fv(env.img.Images[sha].Fields, "weather") != "rain" {
		t.Error("overwrite=true: o valor do CSV deveria substituir")
	}
}

func TestImportPulaVerificadasEReservadas(t *testing.T) {
	env := newTestEnv(t)
	env.withProfile(t)
	shaA := env.seedNamed(t, "a.jpg", "conteudo A")
	shaB := env.seedNamed(t, "b.jpg", "conteudo B")

	// a: verificada. b: com a maria na fila.
	assertStatus(t, env.doJSON("PUT", "/images/"+shaA, completeBody()), http.StatusOK)
	img := env.img.Images[shaB]
	until := time.Now().Add(time.Hour)
	img.ClaimedBy, img.ClaimedUntil = "maria", &until
	env.img.Images[shaB] = img

	rep := decodeImport(t, env.postFile(t, "/packages/"+testPkg+"/import?profile=csv-teste&dry_run=false", "file", importCSV))
	if rep.SkippedVerified != 1 || rep.SkippedClaimed != 1 || rep.Applied != 0 {
		t.Errorf("verified=%d claimed=%d applied=%d; want 1, 1, 0", rep.SkippedVerified, rep.SkippedClaimed, rep.Applied)
	}
	if fv(env.img.Images[shaB].Fields, "day_time") != "" {
		t.Error("o import não pode mexer na imagem que a maria está anotando")
	}
}

func TestImportCadastraEquipamentoECameraNovos(t *testing.T) {
	env := newTestEnv(t)
	profile := strings.Replace(testProfileJSON, `"value": "8003-MF4739"`, `"value": "9999 NOVO"`, 1)
	profile = strings.Replace(profile, `"equipment":`, `"camera": { "column": "Camera" },
    "equipment":`, 1)
	assertStatus(t, env.doJSON("POST", "/import-profiles", profile), http.StatusCreated)
	sha := env.seedNamed(t, "a.jpg", "conteudo A")
	env.seedNamed(t, "b.jpg", "conteudo B")

	csv := "Arquivo;Camera\na.jpg;Intelbras VIP 3230\nb.jpg;hikvision-233\n"
	path := "/packages/" + testPkg + "/import?profile=csv-teste"

	// Ensaio: lista o que seria criado, mas não cria.
	rep := decodeImport(t, env.postFile(t, path, "file", csv))
	if len(rep.NewEquipments) != 1 || rep.NewEquipments[0] != "9999-NOVO" {
		t.Errorf("new_equipments = %v (o nome deveria vir sanitizado)", rep.NewEquipments)
	}
	if len(rep.NewCameras) != 1 || rep.NewCameras[0] != "Intelbras-VIP-3230" {
		t.Errorf("new_cameras = %v (hikvision-233 já existe e não entra)", rep.NewCameras)
	}
	if env.cat.Equipments["9999-NOVO"] || env.cat.Cameras["Intelbras-VIP-3230"] {
		t.Fatal("o ensaio cadastrou!")
	}

	// Aplicar: cadastra e grava a imagem já com o nome sanitizado.
	rep = decodeImport(t, env.postFile(t, path+"&dry_run=false", "file", csv))
	if !env.cat.Equipments["9999-NOVO"] || !env.cat.Cameras["Intelbras-VIP-3230"] {
		t.Errorf("deveria ter cadastrado: equipments=%v cameras=%v", env.cat.Equipments, env.cat.Cameras)
	}
	img := env.img.Images[sha]
	if fv(img.Fields, "equipment") != "9999-NOVO" || fv(img.Fields, "camera") != "Intelbras-VIP-3230" {
		t.Errorf("a imagem deveria usar o nome sanitizado: equipment=%q camera=%q", fv(img.Fields, "equipment"), fv(img.Fields, "camera"))
	}

	// Rodar de novo: nada novo para criar.
	again := decodeImport(t, env.postFile(t, path, "file", csv))
	if len(again.NewEquipments) != 0 || len(again.NewCameras) != 0 {
		t.Errorf("segunda vez não deveria ter novos: %v %v", again.NewEquipments, again.NewCameras)
	}
}

func TestImportNomeImpossivelDeSanitizar(t *testing.T) {
	env := newTestEnv(t)
	profile := strings.Replace(testProfileJSON, `"equipment":   { "value": "8003-MF4739" },`,
		`"equipment":   { "column": "Equip" },`, 1)
	assertStatus(t, env.doJSON("POST", "/import-profiles", profile), http.StatusCreated)
	sha := env.seedNamed(t, "a.jpg", "conteudo A")

	csv := "Arquivo;Equip;Turno\na.jpg;8003/MF;N\n"
	rep := decodeImport(t, env.postFile(t, "/packages/"+testPkg+"/import?profile=csv-teste&dry_run=false", "file", csv))

	if len(rep.InvalidNames) != 1 || rep.InvalidNames[0].Value != "8003/MF" || rep.InvalidNames[0].Line != 2 {
		t.Errorf("invalid_names = %+v", rep.InvalidNames)
	}
	if len(rep.NewEquipments) != 0 || len(env.cat.Equipments) != 1 {
		t.Error("nome inválido não pode ser cadastrado")
	}
	img := env.img.Images[sha]
	if fv(img.Fields, "equipment") != "" || fv(img.Fields, "day_time") != "night" {
		t.Errorf("equipamento vazio e o resto aplicado: %+v", img)
	}
}

func TestImportNomeOriginalRepetido(t *testing.T) {
	env := newTestEnv(t)
	env.withProfile(t)
	env.seedNamed(t, "a.jpg", "foto 1")
	env.seedNamed(t, "a.jpg", "foto 2") // outro conteúdo, mesmo nome (pastas diferentes)

	rep := decodeImport(t, env.postFile(t, "/packages/"+testPkg+"/import?profile=csv-teste&dry_run=false", "file", importCSV))
	if len(rep.Ambiguous) != 1 || rep.Ambiguous[0] != "a.jpg" {
		t.Errorf("ambiguous = %v", rep.Ambiguous)
	}
	for _, img := range env.img.Images {
		if img.OriginalName == "a.jpg" && fv(img.Fields, "day_time") != "" {
			t.Error("com nome repetido, não dá para saber qual é: nenhuma pode ser alterada")
		}
	}
}

func TestImportPorSHA1(t *testing.T) {
	env := newTestEnv(t)
	sha := env.seedNamed(t, "qualquer.jpg", "conteudo")

	profile := `{"name":"por-hash","reader":{"type":"csv"},"match":{"column":"hash","by":"sha1"},
	             "fields":{"weather":{"column":"clima","map":{"chuva":"rain"}}}}`
	assertStatus(t, env.doJSON("POST", "/import-profiles", profile), http.StatusCreated)

	csv := "hash,clima\n" + strings.ToUpper(sha) + ",chuva\n"
	rep := decodeImport(t, env.postFile(t, "/packages/"+testPkg+"/import?profile=por-hash&dry_run=false", "file", csv))
	if rep.Applied != 1 || fv(env.img.Images[sha].Fields, "weather") != "rain" {
		t.Errorf("applied=%d weather=%q", rep.Applied, fv(env.img.Images[sha].Fields, "weather"))
	}
}

func TestImportErros(t *testing.T) {
	env := newTestEnv(t)
	env.withProfile(t)

	cases := map[string]struct {
		path   string
		csv    string
		status int
		want   string
	}{
		"sem perfil":         {"/packages/" + testPkg + "/import", importCSV, http.StatusBadRequest, "profile"},
		"perfil inexistente": {"/packages/" + testPkg + "/import?profile=nao-existe", importCSV, http.StatusNotFound, "perfil"},
		"pacote inexistente": {"/packages/nao-existe/import?profile=csv-teste", importCSV, http.StatusNotFound, "pacote"},
		"CSV sem a coluna de match": {"/packages/" + testPkg + "/import?profile=csv-teste", "Nome;Turno\na.jpg;N\n",
			http.StatusBadRequest, "Arquivo"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			rec := env.postFile(t, tc.path, "file", tc.csv)
			assertStatus(t, rec, tc.status)
			if msg := errorMsg(t, rec); !strings.Contains(msg, tc.want) {
				t.Errorf("mensagem %q não menciona %q", msg, tc.want)
			}
		})
	}
}

func TestMergeAnnotation(t *testing.T) {
	when := time.Date(2026, 9, 3, 11, 0, 0, 0, time.UTC)
	v := annotation.Values{Fields: map[string]any{"day_time": "night", "weather": "rain", "captured_at": when, "conditions": []string{"glare"}}}

	img := images.Image{Fields: map[string]any{"weather": "fog"}}
	kept := img.Fields
	if !images.MergeAnnotation(&img, v, false) {
		t.Fatal("deveria ter mudado")
	}
	if fv(img.Fields, "weather") != "fog" || fv(img.Fields, "day_time") != "night" || img.Fields["captured_at"] != when {
		t.Errorf("sem overwrite = %+v", img.Fields)
	}
	if images.MergeAnnotation(&img, v, false) {
		t.Error("aplicar a mesma coisa de novo não deveria mudar nada")
	}
	// o mapa de antes não muda (quem chamou guarda a imagem de antes para o histórico)
	if len(kept) != 1 {
		t.Errorf("o mapa antigo foi alterado: %v", kept)
	}
	if !images.MergeAnnotation(&img, v, true) || fv(img.Fields, "weather") != "rain" {
		t.Errorf("com overwrite = %+v", img.Fields)
	}
}
