package filename

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

const validName = "02d9314a6112e8b95f97415dc96e5bf0682f3a3f_v2_8003-MF4739_hikvision-233_F-01-L-D-E-S_01_20261103T221400Z_KOY7386-B-1-B-WHT.jpg"

func TestParseValid(t *testing.T) {
	want := ImageMeta{
		SHA1:         "02d9314a6112e8b95f97415dc96e5bf0682f3a3f",
		Version:      "v2",
		Equipment:    "8003-MF4739",
		Camera:       "hikvision-233",
		Direction:    "front",
		Resolution:   "640x480",
		CaptureAngle: "lateral",
		DayTime:      "day",
		ImageRange:   "narrow",
		Weather:      "clear",
		Conditions:   []string{"no_damage"},
		CapturedAt:   time.Date(2026, 11, 3, 22, 14, 0, 0, time.UTC),
		Vehicles: []Vehicle{{
			Plate: "KOY7386", PlateType: "br", PlateCategory: "rental",
			Type: "bus", Color: "white",
		}},
	}

	got, err := Parse(validName)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("\ngot:  %+v\nwant: %+v", got, want)
	}
}

func TestParseMultipleVehiclesAndConditions(t *testing.T) {
	name := "02d9314a6112e8b95f97415dc96e5bf0682f3a3f_v2_8003-MF4739_hikvision-233_F-09-S-N-A-C_03.05_20261103T221400Z_ABC1D23-M-8-C-BLK.KOY7386-B-1-T-WHT.jpg"

	got, err := Parse(name)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(got.Vehicles) != 2 {
		t.Errorf("esperava 2 veículos, veio %d", len(got.Vehicles))
	}
	if !reflect.DeepEqual(got.Conditions, []string{"duplicate_plate", "glare"}) {
		t.Errorf("condições erradas: %v", got.Conditions)
	}
}

func TestParseInvalid(t *testing.T) {
	cases := map[string]string{
		"partes faltando":                    "02d9314a6112e8b95f97415dc96e5bf0682f3a3f_v2_8003-MF4739.jpg",
		"hash curto":                         "02d93_v2_8003-MF4739_hikvision-233_F-01-L-D-E-S_01_20261103T221400Z_KOY7386-B-1-B-WHT.jpg",
		"hash não-hex":                       "zzd9314a6112e8b95f97415dc96e5bf0682f3a3f_v2_8003-MF4739_hikvision-233_F-01-L-D-E-S_01_20261103T221400Z_KOY7386-B-1-B-WHT.jpg",
		"versão errada":                      "02d9314a6112e8b95f97415dc96e5bf0682f3a3f_v9_8003-MF4739_hikvision-233_F-01-L-D-E-S_01_20261103T221400Z_KOY7386-B-1-B-WHT.jpg",
		"código inválido":                    "02d9314a6112e8b95f97415dc96e5bf0682f3a3f_v2_8003-MF4739_hikvision-233_X-01-L-D-E-S_01_20261103T221400Z_KOY7386-B-1-B-WHT.jpg",
		"data inválida":                      "02d9314a6112e8b95f97415dc96e5bf0682f3a3f_v2_8003-MF4739_hikvision-233_F-01-L-D-E-S_01_2026-11-03_KOY7386-B-1-B-WHT.jpg",
		"equipamento com caractere inválido": "02d9314a6112e8b95f97415dc96e5bf0682f3a3f_v2_8003.MF4739_hikvision-233_F-01-L-D-E-S_01_20261103T221400Z_KOY7386-B-1-B-WHT.jpg",
		"câmera longa demais":                "02d9314a6112e8b95f97415dc96e5bf0682f3a3f_v2_8003-MF4739_hikvision-233-modelo-com-nome-gigante-demais_F-01-L-D-E-S_01_20261103T221400Z_KOY7386-B-1-B-WHT.jpg",
		"veículo incompleto":                 "02d9314a6112e8b95f97415dc96e5bf0682f3a3f_v2_8003-MF4739_hikvision-233_F-01-L-D-E-S_01_20261103T221400Z_KOY7386-B-1.jpg",
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(input); err == nil {
				t.Errorf("esperava erro para %q", input)
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	names := []string{
		validName,
		"02d9314a6112e8b95f97415dc96e5bf0682f3a3f_v2_8003-MF4739_hikvision-233_F-09-S-N-A-C_03.05_20261103T221400Z_ABC1D23-M-8-C-BLK.KOY7386-B-1-T-WHT.jpg",
	}
	for _, name := range names {
		m, err := Parse(name)
		if err != nil {
			t.Fatalf("Parse(%q): %v", name, err)
		}
		got, err := Format(m)
		if err != nil {
			t.Fatalf("Format: %v", err)
		}
		if got+".jpg" != name {
			t.Errorf("round-trip mudou o nome:\ngot:  %s\nwant: %s", got+".jpg", name)
		}
	}
}

func TestFormatInvalid(t *testing.T) {
	m, _ := Parse(validName)
	m.DayTime = "afternoon"
	if _, err := Format(m); err == nil {
		t.Error("esperava erro para período desconhecido")
	}
}

func TestSanitize(t *testing.T) {
	ok := map[string]string{
		"8003 MF4739":     "8003-MF4739",
		"  8003   MF4739": "8003-MF4739",
		"8003-MF4739":     "8003-MF4739",
	}
	for in, want := range ok {
		got, err := SanitizeEquipment(in)
		if err != nil || got != want {
			t.Errorf("SanitizeEquipment(%q) = %q, %v; want %q", in, got, err, want)
		}
	}

	bad := []string{"", "   ", "8003_MF4739", "8003/MF4739", "-8003", "8003--MF", "nome-de-equipamento-longo-demais"}
	for _, in := range bad {
		if _, err := SanitizeEquipment(in); err == nil {
			t.Errorf("SanitizeEquipment(%q): esperava erro", in)
		}
	}
}

func TestIsAllowedExt(t *testing.T) {
	for ext, want := range map[string]bool{
		".jpg": true, ".JPG": true, ".jpeg": true, ".png": true,
		".gif": false, ".txt": false, "": false,
	} {
		if got := IsAllowedExt(ext); got != want {
			t.Errorf("IsAllowedExt(%q) = %v, want %v", ext, got, want)
		}
	}
}

func TestParseSHA1Maiusculo(t *testing.T) {
	upper := strings.ToUpper(validName[:40]) + validName[40:]
	m, err := Parse(upper)
	if err != nil {
		t.Fatal(err)
	}
	if m.SHA1 != validName[:40] {
		t.Errorf("sha1 deveria ser normalizado para minúsculo: %s", m.SHA1)
	}
}

func TestParseExtensaoInvalida(t *testing.T) {
	name := strings.TrimSuffix(validName, ".jpg") + ".gif"
	if _, err := Parse(name); err == nil {
		t.Error("esperava erro para .gif")
	}
}

func TestFormatErros(t *testing.T) {
	base, err := Parse(validName)
	if err != nil {
		t.Fatal(err)
	}

	cases := map[string]func(m *ImageMeta){
		"sem condições": func(m *ImageMeta) { m.Conditions = nil },
		"sem veículos":  func(m *ImageMeta) { m.Vehicles = nil },
		"placa inválida": func(m *ImageMeta) {
			m.Vehicles = []Vehicle{{Plate: "AB-12", PlateType: "br", PlateCategory: "private", Type: "car", Color: "red"}}
		},
		"cor desconhecida":     func(m *ImageMeta) { m.Vehicles[0].Color = "roxo-neon" },
		"equipamento com _":    func(m *ImageMeta) { m.Equipment = "8003_MF" },
		"sha1 inválido":        func(m *ImageMeta) { m.SHA1 = "xyz" },
		"condição inexistente": func(m *ImageMeta) { m.Conditions = []string{"no_damage", "aliens"} },
	}
	for name, mod := range cases {
		t.Run(name, func(t *testing.T) {
			m := base
			m.Vehicles = append([]Vehicle(nil), base.Vehicles...) // cópia: não estragar o base
			mod(&m)
			if _, err := Format(m); err == nil {
				t.Error("esperava erro")
			}
		})
	}
}

func TestFormatConverteParaUTC(t *testing.T) {
	m, _ := Parse(validName)
	saoPaulo := time.FixedZone("BRT", -3*60*60)
	m.CapturedAt = time.Date(2026, 11, 3, 19, 14, 0, 0, saoPaulo) // = 22:14 UTC

	got, err := Format(m)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "_20261103T221400Z_") {
		t.Errorf("data não convertida para UTC: %s", got)
	}
}

func TestCodesRetornaCopia(t *testing.T) {
	c := Codes()
	c["day_time"]["X"] = "estragado"
	delete(c["direction"], "F")

	if _, ok := dayTimeCodes["X"]; ok {
		t.Error("alterar o retorno de Codes() alterou a tabela original")
	}
	if _, err := Parse(validName); err != nil {
		t.Errorf("parser quebrou depois de mexerem no retorno de Codes(): %v", err)
	}
}

func TestTabelasSemValoresRepetidos(t *testing.T) {
	// Se dois códigos tivessem o mesmo valor, o Format (que usa a tabela
	// invertida) não saberia qual código escrever.
	for field, table := range Codes() {
		seen := map[string]string{}
		for code, value := range table {
			if other, ok := seen[value]; ok {
				t.Errorf("%s: códigos %q e %q têm o mesmo valor %q", field, code, other, value)
			}
			seen[value] = code
		}
	}
}

func TestCodigosSemSeparadores(t *testing.T) {
	// Um código com "_", "-" ou "." quebraria o split do Parse.
	for field, table := range Codes() {
		for code := range table {
			if strings.ContainsAny(code, "_-.") {
				t.Errorf("%s: código %q contém separador", field, code)
			}
		}
	}
}
