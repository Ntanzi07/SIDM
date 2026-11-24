package importer

import (
	"strings"
	"testing"
	"time"

	"SIDM/internal/annotation"
)

// fakeJPEG monta um "JPEG" mínimo com um INI grudado depois do fim,
// igual ao que os equipamentos gravam. (Os bytes da imagem não importam:
// só os marcadores de início FF D8 e de fim FF D9.)
func fakeJPEG(trailer string) []byte {
	img := []byte{0xFF, 0xD8, 0xFF, 0xC0, 0x00, 0x11, 0x08, 0x02, 0x58, 0x03, 0x20, 0xFF, 0xD9}
	return append(img, []byte(trailer)...)
}

const sampleINI = `
[Metrologico]
Data=18/09/2026
Hora=10:32:28
Milisegundo=321
Faixa=1
Assinatura=wsBcBAABCAAQBQJqrT1sCRC/HWlTA6

[NMetrologico]
Equip = MFR1-0001
Placa = GGB5H24
TipoVeiculo = AUTOMOVEL
; um comentário
linha sem igual
Hora = 10:32:28
`

func TestExtractJPEGTrailer(t *testing.T) {
	got := ExtractJPEGTrailer(fakeJPEG(sampleINI))
	if !strings.HasPrefix(string(got), "[Metrologico]") {
		t.Errorf("trailer = %.40q", got)
	}

	cases := map[string][]byte{
		"sem nada depois do fim": fakeJPEG(""),
		"só espaços depois":      fakeJPEG("\n\n  \n"),
		"não é JPEG":             []byte("PNG...[Secao]\nA=1"),
		"lixo que não é INI":     fakeJPEG("xxxxxxxx"),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if got := ExtractJPEGTrailer(data); got != nil {
				t.Errorf("esperava nil, veio %.30q", got)
			}
		})
	}
}

// Equipamento que grava duas imagens no arquivo, cada uma com o seu INI:
// JPEG 1, [Metrologico], [Imagem2], JPEG 2 (com uma miniatura EXIF dentro,
// que também termina em FF D9), [NMetrologico]. Antes só o último bloco era
// lido e o Metrologico sumia.
func TestExtractJPEGTrailerDuasImagens(t *testing.T) {
	var data []byte
	data = append(data, fakeJPEG("\r\n[Metrologico]\r\nData=18/09/2026\r\nAssinatura=abc\r\n[Imagem2]")...)
	// JPEG 2: APP1 com uma miniatura (FF D8 ... FF D9) seguida de binário, e o fim.
	data = append(data, 0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x10, 0xFF, 0xD8, 0x01, 0xFF, 0xD9, 0x00, 0x3D, 0x5B, 0x01, 0xFF, 0xC0, 0x00, 0x05, 0xFF, 0xD9)
	data = append(data, []byte("\n[NMetrologico]\nPlaca = ABC1D23\n")...)

	entries := ParseINI(ExtractJPEGTrailer(data))
	got := INIValues(entries)
	want := map[string]string{"Metrologico.Data": "18/09/2026", "NMetrologico.Placa": "ABC1D23"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, esperava %q (tudo: %v)", k, got[k], v, got)
		}
	}
	if len(got) != len(want) { // a Assinatura fica de fora; o binário da miniatura também
		t.Errorf("chaves a mais: %v", got)
	}
}

func TestExtractJPEGTrailerLatin1(t *testing.T) {
	got := ExtractJPEGTrailer(fakeJPEG("[A]\nEndereco=Av. Mar\xe7o\n"))
	if !strings.Contains(string(got), "Março") {
		t.Errorf("latin1 não convertido: %q", got)
	}
}

func TestParseINI(t *testing.T) {
	entries := ParseINI(ExtractJPEGTrailer(fakeJPEG(sampleINI)))
	values := INIValues(entries)

	want := map[string]string{
		"Metrologico.Data":         "18/09/2026",
		"Metrologico.Milisegundo":  "321",
		"NMetrologico.Equip":       "MFR1-0001", // espaços em volta do "=" removidos
		"NMetrologico.TipoVeiculo": "AUTOMOVEL", // espaços sobrando removidos
		"NMetrologico.Hora":        "10:32:28",  // mesma chave em outra seção: não colide
		"Metrologico.Hora":         "10:32:28",
	}
	for k, v := range want {
		if values[k] != v {
			t.Errorf("%s = %q, want %q", k, values[k], v)
		}
	}
	if _, ok := values["Metrologico.Assinatura"]; ok {
		t.Error("a assinatura não deveria ir para o banco")
	}
	for k := range values {
		if strings.Contains(k, "comentário") || strings.Contains(k, "linha sem") {
			t.Errorf("comentário ou linha inválida virou chave: %q", k)
		}
	}
	if entries[0].Section != "Metrologico" || entries[0].Key != "Data" {
		t.Errorf("a ordem do arquivo deveria ser mantida: %+v", entries[0])
	}
}

func TestINITableETemplateDeData(t *testing.T) {
	p := &Profile{
		Name:   "ini-mfr1",
		Reader: ReaderConfig{Type: ReaderJPEGINI},
		Fields: map[string]FieldRule{
			"equipment": {Column: "NMetrologico.Equip"},
			"captured_at": {
				Template:   "{Metrologico.Data} {Metrologico.Hora}.{Metrologico.Milisegundo}",
				DateFormat: "02/01/2006 15:04:05.000",
				Timezone:   "America/Sao_Paulo",
			},
		},
		Vehicles: &VehicleRules{
			Mode: VehiclesSingle,
			Fields: map[string]FieldRule{
				"plate": {Column: "NMetrologico.Placa"},
				"type":  {Column: "NMetrologico.TipoVeiculo", Map: map[string]string{"AUTOMOVEL": "car"}},
			},
		},
	}
	if err := validate(p); err != nil {
		t.Fatal(err)
	}

	entries := ParseINI(ExtractJPEGTrailer(fakeJPEG(sampleINI)))
	run := *p
	run.Match = MatchConfig{Column: "_sha1", By: MatchBySHA1}
	records, rep := transform(&run, INITable("_sha1", "abc", entries))

	if len(records) != 1 || records[0].Key != "abc" {
		t.Fatalf("records = %+v", records)
	}
	a := records[0].Values
	want := time.Date(2026, 9, 18, 13, 32, 28, 321e6, time.UTC) // 10:32:28.321 em São Paulo
	if got, _ := a.Fields["captured_at"].(time.Time); !got.Equal(want) {
		t.Errorf("captured_at = %v, want %v (template juntando data, hora e ms)", a.Fields["captured_at"], want)
	}
	if a.Fields["equipment"] != "MFR1-0001" || rep.ErrorCount != 0 {
		t.Errorf("equipment=%v erros=%+v", a.Fields["equipment"], rep.Errors)
	}
	if got := vehicleObjs(a); got != "vehiclemap[plate:GGB5H24 type:car]" {
		t.Errorf("veículo (modo single) = %s", got)
	}
}

func TestTemplateComCampoFaltando(t *testing.T) {
	values := map[string]string{"A": "1", "B": ""}
	if got := fillTemplate("{A} {B}", values); got != "" {
		t.Errorf("com B vazio o resultado deveria ser vazio, veio %q", got)
	}
	if got := fillTemplate("{A}-x", values); got != "1-x" {
		t.Errorf("fillTemplate = %q", got)
	}
}

func TestValidateINI(t *testing.T) {
	base := func() *Profile {
		return &Profile{
			Name:   "ini",
			Reader: ReaderConfig{Type: ReaderJPEGINI},
			Fields: map[string]FieldRule{"equipment": {Column: "NMetrologico.Equip"}},
		}
	}
	if err := validate(base()); err != nil {
		t.Fatalf("perfil INI válido recusado: %v", err)
	}

	cases := map[string]struct {
		mod  func(p *Profile)
		want string
	}{
		"com match": {func(p *Profile) { p.Match = MatchConfig{Column: "x", By: MatchBySHA1} }, "match"},
		"veículos por linha": {func(p *Profile) {
			p.Vehicles = &VehicleRules{Mode: VehiclesPerRow, Fields: map[string]FieldRule{"plate": {Column: "P"}}}
		}, "objects.vehicle.mode"},
		"template sem chaves": {func(p *Profile) { p.Fields["camera"] = FieldRule{Template: "sem colunas"} }, "template"},
		"column e template":   {func(p *Profile) { p.Fields["camera"] = FieldRule{Column: "A", Template: "{B}"} }, "exatamente um"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p := base()
			tc.mod(p)
			err := validate(p)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want algo com %q", err, tc.want)
			}
		})
	}
}

// Os perfis padrão têm que passar na validação contra o perfil de anotação
// deles (um erro aqui quebraria o upload de todo mundo).
func TestBuiltinsValidos(t *testing.T) {
	for _, p := range Builtins() {
		ap, ok := annotation.Builtin(p.AnnotationProfile)
		if !ok {
			t.Fatalf("%s: perfil de anotação %q não é padrão", p.Name, p.AnnotationProfile)
		}
		if err := p.Validate(&ap); err != nil {
			t.Errorf("%s: %v", p.Name, err)
		}
	}
	p := SpliceINI()
	p.Builtin = false // alguém tentando salvar com o nome do padrão
	if err := p.Validate(nil); err == nil || !strings.Contains(err.Error(), "perfil padrão") {
		t.Errorf("err = %v", err)
	}
}
