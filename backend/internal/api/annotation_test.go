package api

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"SIDM/internal/images"
	"SIDM/internal/platform"
)

// Um perfil diferente do de veículos: dia/noite obrigatório, quantas
// pessoas na moto, capacete com caixa (vem do CVAT) e placa sem caixa.
const capaceteProfile = `{
  "name": "capacete-moto",
  "fields": [
    {"key": "day_time", "type": "choice", "required": true, "options": [{"value": "day"}, {"value": "night"}]},
    {"key": "riders", "type": "number", "integer": true, "min": 1, "max": 3}
  ],
  "objects": [
    {"key": "helmet", "box": true, "fields": [{"key": "color", "type": "choice", "options": [{"value": "black"}]}]},
    {"key": "plate", "max": 1, "fields": [{"key": "text", "type": "text", "pattern": "[A-Z0-9]{7}", "upper": true}]}
  ]
}`

// withCapacete cadastra o perfil e um pacote "motos" que usa ele.
func (e *testEnv) withCapacete(t *testing.T) {
	t.Helper()
	assertStatus(t, e.doJSON("POST", "/annotation-profiles", capaceteProfile), http.StatusCreated)
	rec := e.doJSON("POST", "/packages", `{"name": "motos", "profile": "capacete-moto"}`)
	assertStatus(t, rec, http.StatusCreated)
	var resp struct{ Name, Profile string }
	decode(t, rec, &resp)
	if resp.Profile != "capacete-moto" {
		t.Fatalf("pacote criado com o perfil %q", resp.Profile)
	}
}

func TestPacoteComPerfilProprio(t *testing.T) {
	env := newTestEnv(t)
	env.withCapacete(t)
	sha := env.uploadTo(t, "motos", upFile{"moto.jpg", "uma moto"})[0].SHA1

	// verificar sem o obrigatório: 400, com o caminho do problema
	rec := env.doJSON("PUT", "/images/"+sha, `{"fields": {"riders": 2}, "complete": true}`)
	assertStatus(t, rec, http.StatusBadRequest)
	var prob struct {
		Problems []struct{ Path string } `json:"problems"`
	}
	decode(t, rec, &prob)
	if len(prob.Problems) != 1 || prob.Problems[0].Path != "fields.day_time" {
		t.Errorf("problemas = %+v", prob.Problems)
	}
	// campo de veículos não existe neste perfil
	assertStatus(t, env.doJSON("PUT", "/images/"+sha, `{"fields": {"weather": "rain"}}`), http.StatusBadRequest)

	// verificar: sem modelo de nome, o arquivo continua <sha1>.jpg
	rec = env.doJSON("PUT", "/images/"+sha,
		`{"fields": {"day_time": "night", "riders": 2}, "objects": [{"type": "plate", "fields": {"text": "abc1d23"}}], "complete": true}`)
	assertStatus(t, rec, http.StatusOK)
	img := env.img.Images[sha]
	if img.Status != images.StatusVerified || img.FileName != sha+".jpg" || img.PatternVersion != "" {
		t.Errorf("status=%s nome=%q versão=%q", img.Status, img.FileName, img.PatternVersion)
	}
	if img.Fields["riders"] != 2.0 || img.Objects[0].Fields["text"] != "ABC1D23" {
		t.Errorf("valores = %+v %+v", img.Fields, img.Objects)
	}
	if _, err := os.Stat(env.pkgFile2("motos", sha+".jpg")); err != nil {
		t.Error("o arquivo sumiu:", err)
	}

	// busca pelos campos do perfil
	var page platform.Page[images.Image]
	decode(t, env.do("GET", "/images?package=motos&fields.day_time=night", nil, nil), &page)
	if page.Total != 1 {
		t.Errorf("busca por fields.day_time = %d", page.Total)
	}

	// com uma verificada, o perfil do pacote não muda mais
	rec = env.doJSON("PATCH", "/packages/motos", `{"profile": "veiculos"}`)
	assertStatus(t, rec, http.StatusBadRequest)
	if !strings.Contains(errorMsg(t, rec), "verificadas") {
		t.Errorf("mensagem = %q", errorMsg(t, rec))
	}

	// reabrir: volta a pendente sem mover o arquivo (o nome já é o sha1)
	assertStatus(t, env.do("POST", "/images/"+sha+"/reopen", nil, map[string]string{"X-User": "nathan"}), http.StatusOK)
	img = env.img.Images[sha]
	if img.Status != images.StatusPending || img.Path != "dataset/motos/"+sha+".jpg" {
		t.Errorf("reaberta: %s %s", img.Status, img.Path)
	}
	if _, err := os.Stat(env.pkgFile2("motos", sha+".jpg")); err != nil {
		t.Error("o arquivo sumiu ao reabrir:", err)
	}
}

func TestPerfilDoPacoteNaAPI(t *testing.T) {
	env := newTestEnv(t)
	// pacote sem perfil: o padrão
	rec := env.doJSON("POST", "/packages", `{"name": "sem-perfil"}`)
	assertStatus(t, rec, http.StatusCreated)
	if !strings.Contains(rec.Body.String(), `"profile":"veiculos"`) {
		t.Errorf("resposta = %s", rec.Body.String())
	}
	assertStatus(t, env.doJSON("POST", "/packages", `{"name": "x", "profile": "nao-existe"}`), http.StatusBadRequest)

	// trocar sem verificadas: pode
	assertStatus(t, env.doJSON("POST", "/annotation-profiles", capaceteProfile), http.StatusCreated)
	rec = env.doJSON("PATCH", "/packages/sem-perfil", `{"profile": "capacete-moto"}`)
	assertStatus(t, rec, http.StatusOK)
	if env.cat.Packages["sem-perfil"].Profile != "capacete-moto" {
		t.Error("perfil não trocado")
	}
	// renomear e trocar juntos
	rec = env.doJSON("PATCH", "/packages/sem-perfil", `{"name": "com-perfil", "profile": "veiculos"}`)
	assertStatus(t, rec, http.StatusOK)
	if env.cat.Packages["com-perfil"].Profile != "veiculos" {
		t.Errorf("pacotes = %+v", env.cat.Packages)
	}
	assertStatus(t, env.doJSON("PATCH", "/packages/com-perfil", `{}`), http.StatusBadRequest)
}

// O nome v2 só existe no perfil veiculos: num pacote de outro perfil, um
// arquivo com nome v2 não pré-preenche nada.
func TestUploadV2EmPerfilProprio(t *testing.T) {
	env := newTestEnv(t)
	env.withCapacete(t)
	content := "imagem com nome v2"
	v2 := sha1Of(content) + "_v2_8003-MF4739_hikvision-233_F-01-L-D-E-S_01_20261103T221400Z_KOY7386-B-1-B-WHT.jpg"
	res := env.uploadTo(t, "motos", upFile{v2, content})
	if res[0].Prefilled || len(env.img.Images[res[0].SHA1].Fields) != 0 {
		t.Errorf("não deveria pré-preencher: %+v", env.img.Images[res[0].SHA1].Fields)
	}
}

// O import só preenche o que o perfil do pacote tem; o resto é listado.
func TestImportForaDoPerfil(t *testing.T) {
	env := newTestEnv(t)
	env.withCapacete(t)
	env.withProfile(t) // csv-teste: equipment, day_time, weather, captured_at e veículos
	sha := env.uploadTo(t, "motos", upFile{"a.jpg", "a"})[0].SHA1

	csv := "Arquivo;Turno;Clima;Placa;Tipo\na.jpg;N;Chuva;ABC1D23;Carro\n"
	rec := env.postFile(t, "/packages/motos/import?profile=csv-teste&dry_run=false", "file", csv)
	assertStatus(t, rec, http.StatusOK)
	rep := decodeImport(t, rec)
	if rep.Applied != 1 || fv(env.img.Images[sha].Fields, "day_time") != "night" {
		t.Errorf("applied=%d fields=%v", rep.Applied, env.img.Images[sha].Fields)
	}
	if _, ok := env.img.Images[sha].Fields["weather"]; ok {
		t.Error("clima não existe no perfil e entrou")
	}
	joined := strings.Join(rep.OutsideProfile, " | ")
	if rep.OutsideProfileCount < 3 || !strings.Contains(joined, "weather") || !strings.Contains(joined, "vehicle") {
		t.Errorf("fora do perfil (%d) = %s", rep.OutsideProfileCount, joined)
	}
}
