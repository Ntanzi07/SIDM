package importer

import (
	"strings"
	"testing"
	"time"
)

func TestParseDateTime(t *testing.T) {
	sp, _ := time.LoadLocation("America/Sao_Paulo")
	now = func() time.Time { return time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { now = time.Now })

	// 17/09/2026 08:35:42 em São Paulo (UTC-3) = 11:35:42 UTC
	base := time.Date(2026, 9, 17, 11, 35, 42, 0, time.UTC)
	ms := func(n int) time.Time { return base.Add(time.Duration(n) * time.Millisecond) }

	cases := []struct {
		name               string
		date, clock, milli string
		order              string
		want               time.Time
	}{
		{"data e hora separadas (INI)", "17/09/2026", "08:35:42", "", "", base},
		{"hora repetida por câmera", "17/09/2026", "08:35:42;08:35:42", "", "", base},
		{"data repetida com |", "17/09/2026|18/09/2026", "08:35:42", "", "", base},
		{"tudo junto", "17/09/2026 08:35:42", "", "", "", base},
		{"espaços sobrando", "  17/09/2026   08:35:42 ", "", "", "", base},
		{"traço e ponto", "17-09-2026", "08:35:42", "", "", base},
		{"com ponto", "17.09.2026", "08:35:42", "", "", base},
		{"dia e mês com 1 dígito", "7/9/2026", "8:35:42", "", "", base.AddDate(0, 0, -10)},
		{"ano com 2 dígitos", "17/09/26", "08:35:42", "", "", base},
		{"ISO sem fuso (vale o do perfil)", "2026-09-17T08:35:42", "", "", "", base},
		{"ano-mês-dia com barra", "2026/09/17 08:35:42", "", "", "", base},
		{"ISO com fuso do texto", "2026-09-17T09:35:42-02:00", "", "", "", base},
		{"ISO em UTC", "2026-09-17T11:35:42Z", "", "", "", base},
		{"ISO com fração", "2026-09-17T08:35:42.250", "", "", "", ms(250)},
		{"compacto", "20260917", "083542", "", "", base},
		{"compacto junto", "20260917083542", "", "", "", base},
		{"sem segundos", "17/09/2026", "08:35", "", "", base.Add(-42 * time.Second)},
		{"só a data (meia-noite)", "17/09/2026", "", "", "", time.Date(2026, 9, 17, 3, 0, 0, 0, time.UTC)},
		{"AM/PM", "17/09/2026", "8:35:42 PM", "", "", base.Add(12 * time.Hour)},
		{"12 AM é meia-noite", "17/09/2026", "12:00:00 am", "", "", time.Date(2026, 9, 17, 3, 0, 0, 0, time.UTC)},
		{"milissegundo com 3 dígitos", "17/09/2026", "08:35:42", "123", "", ms(123)},
		{"milissegundo com 2 dígitos é ms", "17/09/2026", "08:35:42", "12", "", ms(12)},
		{"milissegundo com 1 dígito", "17/09/2026", "08:35:42", "5", "", ms(5)},
		{"fração na hora", "17/09/2026", "08:35:42.5", "", "", ms(500)},
		{"fração com vírgula", "17/09/2026", "08:35:42,123", "", "", ms(123)},
		{"fração na hora ganha do ms separado", "17/09/2026", "08:35:42.100", "999", "", ms(100)},
		{"mês/dia (americano)", "09/17/2026", "08:35:42", "", DateOrderMDY, base},
		{"começando pelo ano ignora a ordem", "2026-09-17", "08:35:42", "", DateOrderMDY, base},
		{"epoch em ms", "1789644942123", "", "", "", time.UnixMilli(1789644942123).UTC()},
		{"epoch em s", "1789644942", "", "", "", time.Unix(1789644942, 0).UTC()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseDateTime(tc.date, tc.clock, tc.milli, tc.order, sp)
			if err != nil {
				t.Fatalf("erro: %v", err)
			}
			if !got.Equal(tc.want) || got.Location() != time.UTC {
				t.Errorf("= %v, esperava %v", got, tc.want)
			}
		})
	}
}

func TestParseDateTimeErros(t *testing.T) {
	now = func() time.Time { return time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { now = time.Now })

	cases := []struct {
		name, date, clock, milli, want string
	}{
		{"vazia", "", "08:00", "", "sem data"},
		{"só separadores", ";;", "", "", "sem data"},
		{"lixo", "--/--/----", "", "", "não é uma data"},
		{"31 de fevereiro", "31/02/2026", "", "", "não existe"},
		{"mês 13", "01/13/2026", "", "", "mês 13"},
		{"americano sem a ordem", "09/17/2026", "", "", `"mdy"`},
		{"hora 25", "17/09/2026", "25:00:00", "", "fora do limite"},
		{"hora que não é hora", "17/09/2026", "manhã", "", "não é uma hora"},
		{"texto depois da data", "17/09/2026xyz", "", "", "sobrando"},
		{"hora duas vezes", "17/09/2026 08:00", "09:00", "", "já tem hora"},
		{"ms que não é número", "17/09/2026", "08:00:00", "abc", "milissegundo"},
		{"relógio zerado", "01/01/1970", "00:00:00", "", "fora do esperado"},
		{"no futuro", "01/01/2030", "", "", "fora do esperado"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseDateTime(tc.date, tc.clock, tc.milli, "", time.UTC)
			if err == nil {
				t.Fatalf("esperava erro, veio %v", got)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("erro %q não menciona %q", err, tc.want)
			}
		})
	}
}

// O perfil com as partes em colunas separadas, como o INI do MFR1 (a hora
// do NMetrologico vem repetida por câmera), e o date_format de um perfil
// antigo que não bate (vale a leitura automática).
func TestTransformDataEmPartes(t *testing.T) {
	now = func() time.Time { return time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { now = time.Now })

	entries := ParseINI([]byte("[Metrologico]\nData=12/08/2026\nMilisegundo=12\n[NMetrologico]\nHora=10:03:24;10:03:24\n"))
	want := time.Date(2026, 8, 12, 13, 3, 24, 12_000_000, time.UTC)
	rules := map[string]FieldRule{
		"partes": {Column: "Metrologico.Data", TimeColumn: "NMetrologico.Hora", MsColumn: "Metrologico.Milisegundo", Timezone: "America/Sao_Paulo"},
		"formato antigo que não bate": {Column: "Metrologico.Data", TimeColumn: "NMetrologico.Hora", MsColumn: "Metrologico.Milisegundo",
			Timezone: "America/Sao_Paulo", DateFormat: "02/01/2006 15:04:05.000"},
	}
	for name, rule := range rules {
		t.Run(name, func(t *testing.T) {
			p := &Profile{Name: "ini", Reader: ReaderConfig{Type: ReaderJPEGINI}, Fields: map[string]FieldRule{"captured_at": rule}}
			if err := validate(p); err != nil {
				t.Fatal(err)
			}
			run := *p
			run.Match = MatchConfig{Column: "_sha1", By: MatchBySHA1}
			records, rep := transform(&run, INITable("_sha1", "abc", entries))
			if rep.ErrorCount != 0 {
				t.Fatalf("erros: %+v", rep.Errors)
			}
			if got, _ := records[0].Values.Fields["captured_at"].(time.Time); !got.Equal(want) {
				t.Errorf("captured_at = %v, esperava %v", got, want)
			}
		})
	}
}
