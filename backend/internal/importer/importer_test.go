package importer

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// Um perfil completo, no estilo de um CSV real de equipamento.
func sampleProfile() *Profile {
	return &Profile{
		Name:   "csv-teste",
		Reader: ReaderConfig{Type: "csv", Delimiter: ";"},
		Match:  MatchConfig{Column: "Arquivo", By: MatchByOriginalName},
		Fields: map[string]FieldRule{
			"equipment":   {Value: "8003-MF4739"},
			"day_time":    {Column: "Turno", Map: map[string]string{"D": "day", "N": "night"}},
			"weather":     {Column: "Clima", Map: map[string]string{"Limpo": "clear", "Chuva": "rain"}},
			"conditions":  {Column: "Obs", Split: ",", Map: map[string]string{"ofusc": "glare", "contraste": "low_contrast"}},
			"captured_at": {Column: "DataHora", DateFormat: "02/01/2006 15:04:05", Timezone: "America/Sao_Paulo"},
		},
		Vehicles: &VehicleRules{
			Mode: VehiclesPerRow,
			Fields: map[string]FieldRule{
				"plate": {Column: "Placa"},
				"type":  {Column: "Tipo", Map: map[string]string{"Carro": "car", "Caminhão": "truck"}},
			},
		},
	}
}

func read(t *testing.T, cfg ReaderConfig, content string) *Table {
	t.Helper()
	tbl, err := ReadCSV(strings.NewReader(content), cfg)
	if err != nil {
		t.Fatal(err)
	}
	return tbl
}

// ---------- Validate ----------

func TestValidateOK(t *testing.T) {
	if err := sampleProfile().Validate(); err != nil {
		t.Fatalf("perfil válido recusado: %v", err)
	}
}

func TestValidateErros(t *testing.T) {
	cases := map[string]struct {
		mod  func(p *Profile)
		want string
	}{
		"nome com espaço":        {func(p *Profile) { p.Name = "meu csv" }, "name"},
		"leitor desconhecido":    {func(p *Profile) { p.Reader.Type = "xlsx" }, "reader.type"},
		"delimitador longo":      {func(p *Profile) { p.Reader.Delimiter = ";;" }, "reader.delimiter"},
		"encoding desconhecido":  {func(p *Profile) { p.Reader.Encoding = "utf-16" }, "reader.encoding"},
		"sem coluna de match":    {func(p *Profile) { p.Match.Column = "" }, "match.column"},
		"match.by errado":        {func(p *Profile) { p.Match.By = "placa" }, "match.by"},
		"campo desconhecido":     {func(p *Profile) { p.Fields["velocidade"] = FieldRule{Column: "Vel"} }, "fields.velocidade"},
		"column e value juntos":  {func(p *Profile) { p.Fields["camera"] = FieldRule{Column: "Cam", Value: "x"} }, "fields.camera"},
		"nem column nem value":   {func(p *Profile) { p.Fields["camera"] = FieldRule{} }, "fields.camera"},
		"map com valor inválido": {func(p *Profile) { p.Fields["weather"].Map["Garoa"] = "garoa" }, `map["Garoa"]`},
		"data sem formato":       {func(p *Profile) { p.Fields["captured_at"] = FieldRule{Column: "D"} }, "date_format"},
		"data de exemplo no lugar do layout": {func(p *Profile) {
			p.Fields["captured_at"] = FieldRule{Column: "D", DateFormat: "2026-08-12 09:23:23.000"}
		}, "não é um layout do Go"},
		"fuso inválido": {func(p *Profile) {
			r := p.Fields["captured_at"]
			r.Timezone = "Marte/Olympus"
			p.Fields["captured_at"] = r
		}, "timezone"},
		"split fora de conditions": {func(p *Profile) { r := p.Fields["weather"]; r.Split = ","; p.Fields["weather"] = r }, "split"},
		"veículo sem placa":        {func(p *Profile) { delete(p.Vehicles.Fields, "plate") }, "vehicles.fields.plate"},
		"modo de veículo errado":   {func(p *Profile) { p.Vehicles.Mode = "tabela" }, "vehicles.mode"},
		"columns sem {n}": {func(p *Profile) {
			p.Vehicles.Mode, p.Vehicles.Max = VehiclesPerColumns, 2
		}, "{n}"},
		"columns sem max": {func(p *Profile) {
			p.Vehicles = &VehicleRules{Mode: VehiclesPerColumns, Fields: map[string]FieldRule{"plate": {Column: "Placa{n}"}}}
		}, "vehicles.max"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p := sampleProfile()
			tc.mod(p)
			err := p.Validate()
			if err == nil {
				t.Fatal("esperava erro")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("erro %q não menciona %q", err, tc.want)
			}
		})
	}
}

func TestValidateJuntaTodosOsErros(t *testing.T) {
	p := sampleProfile()
	p.Match.By = "x"
	p.Reader.Encoding = "utf-16"
	err := p.Validate()
	if err == nil || !strings.Contains(err.Error(), "reader.encoding") || !strings.Contains(err.Error(), "match.by") {
		t.Errorf("deveria listar os dois erros de uma vez: %v", err)
	}
}

// ---------- ReadCSV ----------

func TestReadCSV(t *testing.T) {
	tbl := read(t, ReaderConfig{Delimiter: ";"}, "Arquivo; Turno ;Clima\nimg1.jpg;N; Chuva \n\n;;\nimg2.jpg;D;Limpo\n")

	if fmt.Sprint(tbl.Columns) != "[Arquivo Turno Clima]" {
		t.Errorf("colunas = %v (espaços do cabeçalho deveriam sair)", tbl.Columns)
	}
	if len(tbl.Rows) != 2 {
		t.Fatalf("esperava 2 linhas (vazias ignoradas), veio %d", len(tbl.Rows))
	}
	if tbl.Rows[0].Values["Clima"] != "Chuva" {
		t.Errorf("valor deveria vir sem espaços: %q", tbl.Rows[0].Values["Clima"])
	}
	if tbl.Rows[1].Line != 5 {
		t.Errorf("número da linha = %d, want 5 (para o relatório apontar a linha certa)", tbl.Rows[1].Line)
	}
}

func TestReadCSVBOMDoExcel(t *testing.T) {
	tbl := read(t, ReaderConfig{}, "\xEF\xBB\xBFArquivo,Turno\nimg1.jpg,N\n")
	if tbl.Columns[0] != "Arquivo" {
		t.Errorf("o BOM grudou no nome da coluna: %q", tbl.Columns[0])
	}
}

func TestReadCSVLatin1(t *testing.T) {
	// "Caminhão" em Latin-1: o "ã" é o byte 0xE3.
	tbl := read(t, ReaderConfig{Encoding: "latin1"}, "Tipo\nCaminh\xE3o\n")
	if tbl.Rows[0].Values["Tipo"] != "Caminhão" {
		t.Errorf("latin1 não convertido: %q", tbl.Rows[0].Values["Tipo"])
	}
}

func TestReadCSVLinhaCurta(t *testing.T) {
	tbl := read(t, ReaderConfig{}, "A,B,C\n1,2\n")
	if tbl.Rows[0].Values["C"] != "" || tbl.Rows[0].Values["A"] != "1" {
		t.Errorf("linha com menos colunas: %v", tbl.Rows[0].Values)
	}
}

func TestReadCSVErros(t *testing.T) {
	cases := map[string]struct {
		cfg     ReaderConfig
		content string
		want    string
	}{
		"vazio":                 {ReaderConfig{}, "", "vazio"},
		"coluna repetida":       {ReaderConfig{}, "A,A\n1,2\n", "duas vezes"},
		"separador errado":      {ReaderConfig{}, "Arquivo;Turno\nimg;N\n", "separador"},
		"encoding desconhecido": {ReaderConfig{Encoding: "ebcdic"}, "A\n1\n", "encoding"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ReadCSV(strings.NewReader(tc.content), tc.cfg)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want algo com %q", err, tc.want)
			}
		})
	}
}

// ---------- Inspect ----------

func TestInspect(t *testing.T) {
	tbl := read(t, ReaderConfig{Delimiter: ";"}, "Arquivo;Clima\na.jpg;Chuva\nb.jpg;Limpo\nc.jpg;Chuva\nd.jpg;\n")
	ins := Inspect(tbl, 2, 50)

	if ins.Rows != 4 || len(ins.Sample) != 2 {
		t.Errorf("rows=%d sample=%d", ins.Rows, len(ins.Sample))
	}
	clima := ins.Distinct["Clima"]
	if clima.Total != 2 || clima.Values[0] != (ValueCount{"Chuva", 2}) || clima.Values[1] != (ValueCount{"Limpo", 1}) {
		t.Errorf("distintos de Clima = %+v (mais frequentes primeiro, vazios fora)", clima)
	}

	limited := Inspect(tbl, 2, 1)
	if !limited.Distinct["Arquivo"].Truncated || len(limited.Distinct["Arquivo"].Values) != 1 || limited.Distinct["Arquivo"].Total != 4 {
		t.Errorf("limite de distintos não aplicado: %+v", limited.Distinct["Arquivo"])
	}
}

// ---------- Transform ----------

const sampleCSV = `Arquivo;Turno;Clima;Obs;DataHora;Placa;Tipo
img1.jpg;N;chuva;ofusc, contraste;03/09/2026 08:25:23;abc-1d23;Caminhão
img1.jpg;N;chuva;;03/09/2026 08:25:23;KOY7386;Carro
img2.jpg;d;Garoa;;31/02/2026 10:00:00;;
img3.jpg;X;Limpo;ofusc;03/09/2026 23:59:59;XYZ9999;Van
;N;Limpo;;;;
`

func TestTransform(t *testing.T) {
	p := sampleProfile()
	records, rep := Transform(p, read(t, p.Reader, sampleCSV))

	if rep.Rows != 5 || rep.Records != 3 || len(records) != 3 {
		t.Fatalf("rows=%d records=%d; want 5 linhas e 3 imagens", rep.Rows, rep.Records)
	}
	if rep.MissingKey != 1 {
		t.Errorf("missing_key = %d, want 1", rep.MissingKey)
	}

	t.Run("img1: duas linhas agrupadas, dois veículos", func(t *testing.T) {
		a := records[0].Annotation
		if records[0].Key != "img1.jpg" || records[0].Line != 2 {
			t.Errorf("key=%q line=%d", records[0].Key, records[0].Line)
		}
		if a.Equipment != "8003-MF4739" {
			t.Errorf("valor fixo não aplicado: %q", a.Equipment)
		}
		if a.DayTime != "night" || a.Weather != "rain" {
			t.Errorf("map (ignorando maiúscula): day_time=%q weather=%q", a.DayTime, a.Weather)
		}
		if fmt.Sprint(a.Conditions) != "[glare low_contrast]" {
			t.Errorf("split + map: %v", a.Conditions)
		}
		want := time.Date(2026, 9, 3, 11, 25, 23, 0, time.UTC) // 08:25 em São Paulo = 11:25 UTC
		if a.CapturedAt == nil || !a.CapturedAt.Equal(want) {
			t.Errorf("captured_at = %v, want %v", a.CapturedAt, want)
		}
		if len(a.Vehicles) != 2 {
			t.Fatalf("veículos = %+v", a.Vehicles)
		}
		if a.Vehicles[0] != (Vehicle{Plate: "ABC1D23", Type: "truck"}) {
			t.Errorf("placa deveria vir maiúscula e sem hífen: %+v", a.Vehicles[0])
		}
	})

	t.Run("img2: valores ruins ficam vazios", func(t *testing.T) {
		a := records[1].Annotation
		if a.DayTime != "day" || a.Weather != "" || a.CapturedAt != nil || len(a.Vehicles) != 0 {
			t.Errorf("img2 = %+v", a)
		}
	})

	t.Run("relatório", func(t *testing.T) {
		if rep.Unmapped["weather"]["Garoa"] != 1 {
			t.Errorf("Garoa deveria estar em unmapped: %v", rep.Unmapped)
		}
		if rep.Unmapped["day_time"]["X"] != 1 {
			t.Errorf("X deveria estar em unmapped: %v", rep.Unmapped)
		}
		if rep.Unmapped["vehicles.type"]["Van"] != 1 {
			t.Errorf("Van deveria estar em unmapped: %v", rep.Unmapped)
		}
		if rep.ErrorCount != 1 || rep.Errors[0].Line != 4 || rep.Errors[0].Field != "captured_at" {
			t.Errorf("31/02 deveria ser erro na linha 4: %+v", rep.Errors)
		}
	})
}

func TestTransformVeiculosEmColunas(t *testing.T) {
	p := &Profile{
		Name:   "colunas",
		Reader: ReaderConfig{Type: "csv"},
		Match:  MatchConfig{Column: "hash", By: MatchBySHA1},
		Vehicles: &VehicleRules{
			Mode: VehiclesPerColumns,
			Max:  3,
			Fields: map[string]FieldRule{
				"plate": {Column: "Placa{n}"},
				"color": {Column: "Cor{n}", Map: map[string]string{"branco": "white", "preto": "black"}},
			},
		},
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}

	csv := "hash,Placa1,Cor1,Placa2,Cor2,Placa3,Cor3\nABCDEF,AAA1111,branco,,,CCC3333,preto\n"
	records, _ := Transform(p, read(t, p.Reader, csv))

	if records[0].Key != "abcdef" {
		t.Errorf("sha1 deveria ser normalizado para minúsculo: %q", records[0].Key)
	}
	got := records[0].Annotation.Vehicles
	want := []Vehicle{{Plate: "AAA1111", Color: "white"}, {Plate: "CCC3333", Color: "black"}}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("veículos = %+v, want %+v (a coluna 2 vazia deveria ser pulada)", got, want)
	}
}

func TestErrosLimitados(t *testing.T) {
	p := sampleProfile()
	var b strings.Builder
	b.WriteString("Arquivo;DataHora\n")
	for i := 0; i < maxErrors+50; i++ {
		fmt.Fprintf(&b, "img%d.jpg;data-ruim\n", i)
	}
	_, rep := Transform(p, read(t, p.Reader, b.String()))
	if rep.ErrorCount != maxErrors+50 || len(rep.Errors) != maxErrors {
		t.Errorf("error_count=%d errors=%d; want o total contado e só %d listados", rep.ErrorCount, len(rep.Errors), maxErrors)
	}
}

func TestValidDateLayout(t *testing.T) {
	validos := []string{
		"02/01/2006 15:04:05",
		"2006-01-02 15:04:05.000",
		"2006-01-02T15:04:05Z07:00",
		"02/01/2006",
		"20060102150405",
		"2 Jan 06 15:04",
	}
	for _, l := range validos {
		if !ValidDateLayout(l) {
			t.Errorf("%q deveria ser um layout válido", l)
		}
	}
	invalidos := []string{
		"2026-08-12 09:23:23.000", // uma data de exemplo, não um layout
		"12/08/2026 09:23",
		"dd/mm/yyyy hh:mm:ss", // padrão de outras linguagens
		"YYYY-MM-DD",
		"15:04:05", // só hora: falta o dia
		"texto",
	}
	for _, l := range invalidos {
		if ValidDateLayout(l) {
			t.Errorf("%q não deveria ser aceito como layout", l)
		}
	}
}

func TestTransformCampoCodificadoSemMap(t *testing.T) {
	p := &Profile{
		Name:   "sem-map",
		Reader: ReaderConfig{Type: "csv", Delimiter: ";"},
		Match:  MatchConfig{Column: "Arquivo", By: MatchByOriginalName},
		Fields: map[string]FieldRule{
			"day_time":  {Column: "Periodo"},
			"equipment": {Column: "Equip"}, // texto livre: vai como está
		},
		Vehicles: &VehicleRules{Mode: VehiclesPerRow, Fields: map[string]FieldRule{
			"plate": {Column: "Placa"},
			"type":  {Column: "Tipo"},
		}},
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	csv := "Arquivo;Periodo;Equip;Placa;Tipo\n" +
		"a.jpg;night;8003 X;AAA1A11;car\n" + // o próprio valor
		"b.jpg;N;8003 X;BBB1B11;T\n" + // a letra do padrão
		"c.jpg;Noite;8003 X;CCC1C11;AUTOMOVEL\n" // nenhum dos dois: fica vazio e vai para o relatório
	records, rep := Transform(p, read(t, p.Reader, csv))
	if len(records) != 3 {
		t.Fatalf("records = %d", len(records))
	}
	got := func(i int) string {
		a := records[i].Annotation
		return a.DayTime + "/" + a.Vehicles[0].Type + "/" + a.Equipment
	}
	for i, want := range []string{"night/car/8003 X", "night/truck/8003 X", "/" + "/8003 X"} {
		if got(i) != want {
			t.Errorf("linha %d: %q, want %q", i+2, got(i), want)
		}
	}
	if rep.Unmapped["vehicles.type"]["AUTOMOVEL"] != 1 || rep.Unmapped["day_time"]["Noite"] != 1 {
		t.Errorf("unmapped = %v", rep.Unmapped)
	}
}
