package filename

import (
	"reflect"
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
