package api

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"SIDM/internal/crypt"
	"SIDM/internal/importer"
	"SIDM/internal/store"
)

// jpegWithINI monta um "JPEG" mínimo com um INI depois do fim da imagem,
// do jeito que os equipamentos gravam. O conteúdo varia com `seed` para cada
// imagem ter um sha1 diferente.
func jpegWithINI(seed, plate string) string {
	img := "\xFF\xD8\xFF\xC0" + seed + "\xFF\xD9"
	return img + `
[Metrologico]
Data=18/09/2026
Hora=10:32:28
Milisegundo=321
Assinatura=wsBcBAABCAAQBQ

[NMetrologico]
Equip = MFR1 0001
Placa = ` + plate + `
TipoVeiculo = AUTOMOVEL
`
}

const iniProfileJSON = `{
  "name": "ini-mfr1",
  "reader": { "type": "jpeg-ini" },
  "fields": {
    "equipment":   { "column": "NMetrologico.Equip" },
    "captured_at": { "template": "{Metrologico.Data} {Metrologico.Hora}.{Metrologico.Milisegundo}",
                     "date_format": "02/01/2006 15:04:05.000", "timezone": "America/Sao_Paulo" }
  },
  "vehicles": { "mode": "single", "fields": {
    "plate": { "column": "NMetrologico.Placa" },
    "type":  { "column": "NMetrologico.TipoVeiculo", "map": { "AUTOMOVEL": "car" } }
  }}
}`

func (e *testEnv) withINIProfile(t *testing.T) {
	t.Helper()
	assertStatus(t, e.doJSON("POST", "/import-profiles", iniProfileJSON), http.StatusCreated)
}

func TestUploadGuardaINI(t *testing.T) {
	env := newTestEnv(t)
	res := env.upload(t,
		upFile{"com-ini.jpg", jpegWithINI("a", "GGB5H24")},
		upFile{"sem-ini.jpg", "\xFF\xD8\xFF\xC0b\xFF\xD9"},
	)

	if !res[0].INI || res[1].INI {
		t.Errorf("ini: com=%v sem=%v", res[0].INI, res[1].INI)
	}
	if res[0].Prefilled {
		t.Error("sem ?profile=, o INI é só guardado: nada é pré-preenchido")
	}

	img := env.db.images[res[0].SHA1]
	got := map[string]string{}
	for _, e := range img.INI {
		got[e.Section+"."+e.Key] = e.Value
	}
	if got["NMetrologico.Placa"] != "GGB5H24" || got["Metrologico.Data"] != "18/09/2026" {
		t.Errorf("INI guardado = %v", got)
	}
	if _, ok := got["Metrologico.Assinatura"]; ok {
		t.Error("a assinatura não pode ir para o banco")
	}
	if img.DayTime != "" || len(img.Vehicles) != 0 {
		t.Error("sem perfil, nenhum campo de anotação deveria ser preenchido")
	}
}

func TestUploadComPerfilDeINI(t *testing.T) {
	env := newTestEnv(t)
	env.withINIProfile(t)

	body := jpegWithINI("a", "GGB5H24")
	res := env.uploadToWithQuery(t, testPkg, "&profile=ini-mfr1", upFile{"foto.jpg", body})
	if res[0].Status != "created" || !res[0].Prefilled {
		t.Fatalf("resultado = %+v", res[0])
	}

	img := env.db.images[res[0].SHA1]
	if img.Status != store.StatusPending {
		t.Error("o perfil pré-preenche, mas a imagem continua pending")
	}
	if img.Equipment != "MFR1-0001" {
		t.Errorf("equipamento deveria vir sanitizado: %q", img.Equipment)
	}
	if !env.db.equipments["MFR1-0001"] {
		t.Error("o equipamento do INI deveria ser cadastrado automaticamente")
	}
	want := time.Date(2026, 9, 18, 13, 32, 28, 321e6, time.UTC)
	if img.CapturedAt == nil || !img.CapturedAt.Equal(want) {
		t.Errorf("captured_at = %v, want %v", img.CapturedAt, want)
	}
	if len(img.Vehicles) != 1 || img.Vehicles[0].Plate != "GGB5H24" || img.Vehicles[0].Type != "car" {
		t.Errorf("veículo = %+v", img.Vehicles)
	}
}

func TestUploadPerfilDeINIComAvisos(t *testing.T) {
	env := newTestEnv(t)
	env.withINIProfile(t)

	body := strings.Replace(jpegWithINI("a", "ABC1234"), "AUTOMOVEL", "TRICICLO", 1)
	body = strings.Replace(body, "Data=18/09/2026", "Data=99/99/2026", 1)
	res := env.uploadToWithQuery(t, testPkg, "&profile=ini-mfr1", upFile{"foto.jpg", body})

	joined := strings.Join(res[0].Warnings, " | ")
	if !strings.Contains(joined, "TRICICLO") || !strings.Contains(joined, "captured_at") {
		t.Errorf("avisos deveriam citar o valor sem tradução e a data ruim: %v", res[0].Warnings)
	}
	if res[0].Status != "created" {
		t.Error("aviso não pode impedir o upload")
	}
}

func TestUploadPerfilErrado(t *testing.T) {
	env := newTestEnv(t)
	env.withProfile(t) // perfil de CSV

	rec := env.do("POST", "/images?package="+testPkg+"&profile=csv-teste", strings.NewReader(""),
		map[string]string{"Content-Type": "multipart/form-data; boundary=x"})
	assertStatus(t, rec, http.StatusBadRequest)
	if !strings.Contains(errorMsg(t, rec), "jpeg-ini") {
		t.Errorf("a mensagem deveria explicar o tipo de perfil: %q", errorMsg(t, rec))
	}

	rec = env.do("POST", "/images?package="+testPkg+"&profile=nao-existe", strings.NewReader(""),
		map[string]string{"Content-Type": "multipart/form-data; boundary=x"})
	assertStatus(t, rec, http.StatusNotFound)
}

func TestImportComPerfilDeINISemArquivo(t *testing.T) {
	env := newTestEnv(t)

	// Upload SEM perfil: o INI fica guardado, nada preenchido.
	res := env.upload(t,
		upFile{"a.jpg", jpegWithINI("a", "AAA1111")},
		upFile{"b.jpg", jpegWithINI("b", "BBB2222")},
		upFile{"sem-ini.jpg", "\xFF\xD8x\xFF\xD9"},
	)
	// O perfil é criado DEPOIS do upload.
	env.withINIProfile(t)

	path := "/packages/" + testPkg + "/import?profile=ini-mfr1"
	rep := decodeImport(t, env.do("POST", path, nil, nil)) // sem arquivo!
	if !rep.DryRun || rep.Records != 2 || rep.WithoutINI != 1 || rep.Applied != 2 {
		t.Errorf("ensaio: dry_run=%v records=%d without_ini=%d applied=%d", rep.DryRun, rep.Records, rep.WithoutINI, rep.Applied)
	}
	if len(rep.NewEquipments) != 1 || rep.NewEquipments[0] != "MFR1-0001" {
		t.Errorf("new_equipments = %v", rep.NewEquipments)
	}
	if env.db.images[res[0].SHA1].Equipment != "" {
		t.Fatal("o ensaio gravou!")
	}

	rep = decodeImport(t, env.do("POST", path+"&dry_run=false", nil, nil))
	if rep.Applied != 2 {
		t.Fatalf("applied = %d", rep.Applied)
	}
	a := env.db.images[res[0].SHA1]
	if a.Equipment != "MFR1-0001" || len(a.Vehicles) != 1 || a.Vehicles[0].Plate != "AAA1111" {
		t.Errorf("imagem a = %+v", a)
	}
	if !env.db.equipments["MFR1-0001"] {
		t.Error("equipamento deveria ter sido cadastrado")
	}
}

func TestImportINIRelatorioPorImagem(t *testing.T) {
	env := newTestEnv(t)
	body := strings.Replace(jpegWithINI("a", "AAA1111"), "AUTOMOVEL", "TRICICLO", 1)
	env.upload(t, upFile{"a.jpg", body})
	env.withINIProfile(t)

	rep := decodeImport(t, env.do("POST", "/packages/"+testPkg+"/import?profile=ini-mfr1", nil, nil))
	if rep.Unmapped["vehicles.type"]["TRICICLO"] != 1 {
		t.Errorf("unmapped = %v", rep.Unmapped)
	}
}

// postFiles manda vários arquivos no campo "file".
func (e *testEnv) postFiles(t *testing.T, path string, files ...upFile) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for _, f := range files {
		w, _ := mw.CreateFormFile("file", f.name)
		w.Write([]byte(f.content))
	}
	mw.Close()
	return e.do("POST", path, &body, map[string]string{"Content-Type": mw.FormDataContentType()})
}

func TestInspectINI(t *testing.T) {
	env := newTestEnv(t)
	rec := env.postFiles(t, "/imports/inspect?type=jpeg-ini",
		upFile{"a.jpg", jpegWithINI("a", "ABC1D23")},
		upFile{"b.jpg", string(crypt.EncryptRC4Legacy([]byte(jpegWithINI("b", "XYZ9W87"))))}, // criptografada
		upFile{"sem-ini.jpg", "\xFF\xD8\xFF\xC0x\xFF\xD9"},
	)
	assertStatus(t, rec, http.StatusOK)
	var ins importer.Inspection
	decode(t, rec, &ins)

	if ins.Rows != 2 {
		t.Errorf("rows = %d, want 2 (a imagem sem INI fica de fora)", ins.Rows)
	}
	want := []string{"Metrologico.Data", "Metrologico.Hora", "Metrologico.Milisegundo", "NMetrologico.Equip", "NMetrologico.Placa", "NMetrologico.TipoVeiculo"}
	if strings.Join(ins.Columns, ",") != strings.Join(want, ",") {
		t.Errorf("colunas = %v\nwant %v (sem a Assinatura)", ins.Columns, want)
	}
	if ins.Distinct["NMetrologico.Placa"].Total != 2 || ins.Distinct["NMetrologico.TipoVeiculo"].Values[0].Value != "AUTOMOVEL" {
		t.Errorf("distintos = %+v", ins.Distinct)
	}
}

func TestInspectINIErros(t *testing.T) {
	env := newTestEnv(t)
	rec := env.postFiles(t, "/imports/inspect?type=jpeg-ini", upFile{"sem-ini.jpg", "\xFF\xD8\xFF\xC0x\xFF\xD9"})
	assertStatus(t, rec, http.StatusBadRequest)
	if !strings.Contains(errorMsg(t, rec), "sem-ini.jpg") {
		t.Errorf("a mensagem deveria citar a imagem: %q", errorMsg(t, rec))
	}
	assertStatus(t, env.postFiles(t, "/imports/inspect?type=jpeg-ini"), http.StatusBadRequest)

	many := make([]upFile, maxInspectINIFiles+1)
	for i := range many {
		many[i] = upFile{"x.jpg", jpegWithINI("x", "AAA1A11")}
	}
	assertStatus(t, env.postFiles(t, "/imports/inspect?type=jpeg-ini", many...), http.StatusBadRequest)
}
