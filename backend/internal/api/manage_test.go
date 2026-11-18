package api

import (
	"SIDM/internal/catalog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedVerified sobe uma imagem e verifica (equipamento 8003-MF4739, câmera hikvision-233).
func (e *testEnv) seedVerified(t *testing.T, content string) string {
	t.Helper()
	sha := e.seedPending(t, content)
	assertStatus(t, e.doJSON("PUT", "/images/"+sha, completeBody()), http.StatusOK)
	return sha
}

func decodeRegistry(t *testing.T, env *testEnv, method, path, body string) (catalog.UpdateReport, int) {
	t.Helper()
	rec := env.doJSON(method, path, body)
	var rep catalog.UpdateReport
	if rec.Code == http.StatusOK {
		decode(t, rec, &rep)
	}
	return rep, rec.Code
}

// ---------- renomear equipamento / câmera ----------

func TestRenomearEquipamentoSemImagens(t *testing.T) {
	env := newTestEnv(t)
	rep, code := decodeRegistry(t, env, "PATCH", "/equipments/8003-MF4739", `{"name":"8003 NOVO"}`)
	if code != http.StatusOK || rep.Name != "8003-NOVO" || !rep.Renamed || rep.Merged || !rep.Complete {
		t.Fatalf("code=%d rep=%+v", code, rep)
	}
	if _, ok := env.cat.Equipments["8003-MF4739"]; ok {
		t.Error("o nome antigo deveria ter sumido")
	}
	if !env.cat.Equipments["8003-NOVO"] {
		t.Error("o nome novo deveria existir e estar ativo")
	}
}

func TestRenomearEquipamentoEmCascata(t *testing.T) {
	env := newTestEnv(t)
	pending := env.seedPending(t, "pendente")
	assertStatus(t, env.doJSON("PUT", "/images/"+pending, `{"equipment":"8003-MF4739","day_time":"night"}`), http.StatusOK)
	verified := env.seedVerified(t, "verificada")
	oldFile := env.img.Images[verified].FileName

	rep, code := decodeRegistry(t, env, "PATCH", "/equipments/8003-MF4739", `{"name":"9000-XYZ"}`)
	if code != http.StatusOK || rep.PendingUpdated != 1 || rep.VerifiedRenamed != 1 || !rep.Complete {
		t.Fatalf("code=%d rep=%+v", code, rep)
	}

	if env.img.Images[pending].Equipment != "9000-XYZ" {
		t.Error("a pendente deveria usar o nome novo")
	}

	v := env.img.Images[verified]
	if v.Equipment != "9000-XYZ" || !strings.Contains(v.FileName, "_9000-XYZ_") {
		t.Errorf("verificada: equipment=%q file=%q", v.Equipment, v.FileName)
	}
	if _, err := os.Stat(env.pkgFile(v.FileName)); err != nil {
		t.Error("o arquivo com o nome novo deveria existir")
	}
	if _, err := os.Stat(env.pkgFile(oldFile)); !os.IsNotExist(err) {
		t.Error("o arquivo com o nome antigo deveria ter sumido")
	}

	var logged bool
	for _, h := range env.img.History {
		if h.ImageID == verified && h.Field == "equipment" && h.New == "9000-XYZ" && strings.Contains(h.ChangedBy, "renomear") {
			logged = true
		}
	}
	if !logged {
		t.Error("a renomeação deveria aparecer no histórico da imagem")
	}
}

func TestRenomearParaNomeExistenteFunde(t *testing.T) {
	env := newTestEnv(t)
	env.cat.Equipments["8003-DUPLICADO"] = true

	rep, code := decodeRegistry(t, env, "PATCH", "/equipments/8003-MF4739", `{"name":"8003-DUPLICADO"}`)
	if code != http.StatusOK || !rep.Merged || rep.Name != "8003-DUPLICADO" {
		t.Fatalf("code=%d rep=%+v", code, rep)
	}
	if _, ok := env.cat.Equipments["8003-MF4739"]; ok {
		t.Error("depois da fusão, o nome antigo deveria sumir")
	}
}

func TestRenomearCamera(t *testing.T) {
	env := newTestEnv(t)
	verified := env.seedVerified(t, "verificada")

	rep, code := decodeRegistry(t, env, "PATCH", "/cameras/hikvision-233", `{"name":"Intelbras 3230"}`)
	if code != http.StatusOK || rep.VerifiedRenamed != 1 {
		t.Fatalf("code=%d rep=%+v", code, rep)
	}
	v := env.img.Images[verified]
	if v.Camera != "Intelbras-3230" || !strings.Contains(v.FileName, "_Intelbras-3230_") {
		t.Errorf("camera=%q file=%q", v.Camera, v.FileName)
	}
	if v.Equipment != "8003-MF4739" {
		t.Error("renomear a câmera não pode mexer no equipamento")
	}
}

func TestRenomearComFalhaPodeSerRepetido(t *testing.T) {
	env := newTestEnv(t)
	verified := env.seedVerified(t, "verificada")
	oldFile := env.img.Images[verified].FileName

	env.img.ReplaceErr = errBoom
	rep, code := decodeRegistry(t, env, "PATCH", "/equipments/8003-MF4739", `{"name":"9000-XYZ"}`)
	if code != http.StatusOK || rep.Complete || len(rep.Failed) != 1 || rep.Failed[0] != verified {
		t.Fatalf("code=%d rep=%+v", code, rep)
	}
	if _, ok := env.cat.Equipments["8003-MF4739"]; !ok {
		t.Error("com falha, o nome antigo tem que ficar (para dar para repetir)")
	}
	if _, err := os.Stat(env.pkgFile(oldFile)); err != nil {
		t.Error("o rename do arquivo deveria ter sido desfeito")
	}

	// O banco voltou: o MESMO pedido termina o serviço.
	env.img.ReplaceErr = nil
	rep, _ = decodeRegistry(t, env, "PATCH", "/equipments/8003-MF4739", `{"name":"9000-XYZ"}`)
	if !rep.Complete || rep.VerifiedRenamed != 1 {
		t.Errorf("repetindo: %+v", rep)
	}
	if _, ok := env.cat.Equipments["8003-MF4739"]; ok {
		t.Error("agora o nome antigo deveria ter sumido")
	}
}

func TestDesativarEReativar(t *testing.T) {
	env := newTestEnv(t)

	rep, code := decodeRegistry(t, env, "PATCH", "/equipments/8003-MF4739", `{"active":false}`)
	if code != http.StatusOK || rep.Active || rep.Renamed {
		t.Fatalf("code=%d rep=%+v", code, rep)
	}

	var p namePage
	decode(t, env.do("GET", "/equipments", nil, nil), &p)
	if p.Total != 0 {
		t.Error("desativado não aparece no autocomplete")
	}

	sha := env.seedPending(t, "x")
	rec := env.doJSON("PUT", "/images/"+sha, completeBody())
	assertStatus(t, rec, http.StatusBadRequest)
	if !strings.Contains(errorMsg(t, rec), "não cadastrado") {
		t.Errorf("verificar com equipamento desativado: %q", errorMsg(t, rec))
	}

	decodeRegistry(t, env, "PATCH", "/equipments/8003-MF4739", `{"active":true}`)
	assertStatus(t, env.doJSON("PUT", "/images/"+sha, completeBody()), http.StatusOK)
}

func TestEditarCadastroErros(t *testing.T) {
	env := newTestEnv(t)
	cases := map[string]struct {
		path, body string
		status     int
	}{
		"inexistente":        {"/equipments/nao-existe", `{"name":"x"}`, http.StatusNotFound},
		"nada para mudar":    {"/equipments/8003-MF4739", `{}`, http.StatusBadRequest},
		"nome inválido":      {"/equipments/8003-MF4739", `{"name":"8003/MF"}`, http.StatusBadRequest},
		"campo errado":       {"/equipments/8003-MF4739", `{"nome":"x"}`, http.StatusBadRequest},
		"câmera inexistente": {"/cameras/nao-existe", `{"active":false}`, http.StatusNotFound},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assertStatus(t, env.doJSON("PATCH", tc.path, tc.body), tc.status)
		})
	}
}

// ---------- excluir equipamento / câmera ----------

func TestExcluirCadastroSemUsoApaga(t *testing.T) {
	env := newTestEnv(t)
	env.cat.Equipments["sem-uso"] = true

	rec := env.do("DELETE", "/equipments/sem-uso", nil, nil)
	assertStatus(t, rec, http.StatusOK)
	var resp map[string]any
	decode(t, rec, &resp)
	if resp["deleted"] != true || resp["deactivated"] != false {
		t.Errorf("resp = %v", resp)
	}
	if _, ok := env.cat.Equipments["sem-uso"]; ok {
		t.Error("deveria ter sido apagado de vez")
	}
}

func TestExcluirCadastroEmUsoDesativa(t *testing.T) {
	env := newTestEnv(t)
	sha := env.seedVerified(t, "verificada")

	rec := env.do("DELETE", "/cameras/hikvision-233", nil, nil)
	assertStatus(t, rec, http.StatusOK)
	var resp map[string]any
	decode(t, rec, &resp)
	if resp["deleted"] != false || resp["deactivated"] != true || resp["images"] != float64(1) {
		t.Errorf("resp = %v", resp)
	}
	active, exists := env.cat.Cameras["hikvision-233"]
	if !exists || active {
		t.Errorf("deveria continuar cadastrada, mas desativada: exists=%v active=%v", exists, active)
	}
	if env.img.Images[sha].Camera != "hikvision-233" {
		t.Error("as imagens que usam não podem ser alteradas")
	}

	assertStatus(t, env.do("DELETE", "/cameras/nao-existe", nil, nil), http.StatusNotFound)
}

// ---------- pacotes ----------

func TestRenomearPacote(t *testing.T) {
	env := newTestEnv(t)
	pending := env.seedPending(t, "pendente")
	verified := env.seedVerified(t, "verificada")

	rec := env.doJSON("PATCH", "/packages/"+testPkg, `{"name":"Pacote Renomeado"}`)
	assertStatus(t, rec, http.StatusOK)
	var resp map[string]any
	decode(t, rec, &resp)
	if resp["name"] != "Pacote-Renomeado" || resp["images"] != float64(2) {
		t.Errorf("resp = %v", resp)
	}

	for _, sha := range []string{pending, verified} {
		img := env.img.Images[sha]
		if img.Package != "Pacote-Renomeado" || !strings.HasPrefix(img.Path, "dataset/Pacote-Renomeado/") {
			t.Errorf("%s: package=%q path=%q", sha, img.Package, img.Path)
		}
		// o arquivo continua acessível pelo caminho novo
		assertStatus(t, env.do("GET", "/images/"+sha+"/file", nil, nil), http.StatusOK)
	}
	if _, err := os.Stat(filepath.Join(env.root, "dataset", testPkg)); !os.IsNotExist(err) {
		t.Error("a pasta antiga deveria ter sumido")
	}
	if _, ok := env.cat.Packages[testPkg]; ok {
		t.Error("o nome antigo do pacote deveria ter sumido")
	}
}

func TestRenomearPacoteErros(t *testing.T) {
	env := newTestEnv(t)
	env.cat.Packages["outro"] = catalog.Package{Name: "outro"}

	assertStatus(t, env.doJSON("PATCH", "/packages/"+testPkg, `{"name":"outro"}`), http.StatusConflict)
	assertStatus(t, env.doJSON("PATCH", "/packages/"+testPkg, `{"name":"../etc"}`), http.StatusBadRequest)
	assertStatus(t, env.doJSON("PATCH", "/packages/nao-existe", `{"name":"x"}`), http.StatusNotFound)
	assertStatus(t, env.doJSON("PATCH", "/packages/"+testPkg, `{"nome":"x"}`), http.StatusBadRequest)
}

func TestExcluirPacoteApagaTudo(t *testing.T) {
	env := newTestEnv(t)
	env.cat.Packages["manter"] = catalog.Package{Name: "manter"}
	sha := env.seedVerified(t, "verificada") // gera histórico
	env.seedPending(t, "pendente")
	other := env.uploadTo(t, "manter", upFile{"outra.jpg", "de outro pacote"})[0].SHA1

	t.Run("sem confirmação não apaga nada", func(t *testing.T) {
		for _, q := range []string{"", "?confirm=", "?confirm=outro-nome"} {
			rec := env.do("DELETE", "/packages/"+testPkg+q, nil, nil)
			assertStatus(t, rec, http.StatusBadRequest)
			if !strings.Contains(errorMsg(t, rec), "confirm="+testPkg) {
				t.Errorf("a mensagem deveria dizer como confirmar: %q", errorMsg(t, rec))
			}
		}
		if len(env.img.Images) != 3 {
			t.Fatal("apagou sem confirmação!")
		}
	})

	t.Run("com confirmação", func(t *testing.T) {
		rec := env.do("DELETE", "/packages/"+testPkg+"?confirm="+testPkg, nil, nil)
		assertStatus(t, rec, http.StatusOK)
		var resp map[string]any
		decode(t, rec, &resp)
		if resp["deleted_images"] != float64(2) {
			t.Errorf("resp = %v", resp)
		}

		if _, ok := env.img.Images[sha]; ok {
			t.Error("as imagens do pacote deveriam ter sumido")
		}
		for _, h := range env.img.History {
			if h.ImageID == sha {
				t.Error("o histórico das imagens apagadas deveria ter sumido")
			}
		}
		if _, err := os.Stat(filepath.Join(env.root, "dataset", testPkg)); !os.IsNotExist(err) {
			t.Error("a pasta do pacote deveria ter sido apagada")
		}

		if _, ok := env.img.Images[other]; !ok {
			t.Error("a imagem de OUTRO pacote não pode ser afetada")
		}
		if _, err := os.Stat(filepath.Join(env.root, "dataset", "manter")); err != nil {
			t.Error("a pasta de OUTRO pacote não pode ser afetada")
		}
	})

	t.Run("pacote inexistente", func(t *testing.T) {
		assertStatus(t, env.do("DELETE", "/packages/nao-existe?confirm=nao-existe", nil, nil), http.StatusNotFound)
	})
}
