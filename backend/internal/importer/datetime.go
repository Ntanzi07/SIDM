package importer

// Leitura de data e hora SEM formato fixo. Antes o perfil exigia o layout
// exato do Go ("02/01/2006 15:04:05") e qualquer diferença (um ";" a mais,
// milissegundo com 2 dígitos) descartava a data. Agora a data, a hora e o
// milissegundo são lidos cada um sozinho, reconhecendo os formatos comuns,
// sempre com a mesma convenção:
//
//	data:  DD/MM/AAAA, DD-MM-AAAA, DD.MM.AAAA, DD/MM/AA, AAAA-MM-DD, AAAAMMDD
//	       (dia antes do mês; "mdy" no perfil inverte; começando pelo ano é
//	       sempre ano-mês-dia)
//	hora:  HH:MM, HH:MM:SS, HH:MM:SS.fff (qualquer número de casas), HHMM,
//	       HHMMSS, com AM/PM opcional
//	ms:    número inteiro de milissegundos ("12" = 12 ms, não 120)
//	junto: "data hora", "dataThora", ISO com fuso (o fuso do texto vale),
//	       AAAAMMDDHHMMSS, epoch em segundos (10 dígitos) ou ms (13)
//
// Vários valores no mesmo campo ("10:03:24;10:03:24", um por câmera): vale
// o primeiro. Sem fuso no texto, vale o do perfil (vazio = UTC). A data
// também tem que ser plausível (de 2000 até amanhã): um relógio desacertado
// vira erro claro, em vez de uma data errada gravada sem ninguém ver.

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Ordem de dia e mês numa data que não começa pelo ano.
const (
	DateOrderDMY = "dmy" // padrão: 03/04/2026 = 3 de abril
	DateOrderMDY = "mdy" // 03/04/2026 = 4 de março
)

// minDate: antes disso, a data é tida como relógio errado.
var minDate = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// now é trocado nos testes.
var now = time.Now

var (
	reDateYMD    = regexp.MustCompile(`^(\d{4})[-/.](\d{1,2})[-/.](\d{1,2})`)
	reDateDMY    = regexp.MustCompile(`^(\d{1,2})[-/.](\d{1,2})[-/.](\d{4}|\d{2})`)
	reDateYMDRaw = regexp.MustCompile(`^(\d{4})(\d{2})(\d{2})`)
	reClock      = regexp.MustCompile(`^(\d{1,2}):(\d{2})(?::(\d{2})(?:[.,](\d{1,9}))?)?$`)
	reClockRaw   = regexp.MustCompile(`^(\d{2})(\d{2})(\d{2})?$`)
	reAMPM       = regexp.MustCompile(`(?i)\s*([ap])\.?\s*m\.?$`)
	reZone       = regexp.MustCompile(`(?i)\s*(z|utc|gmt|[+-]\d{2}:?\d{2})$`)
	reDigits     = regexp.MustCompile(`^\d+$`)
	reSpaces     = regexp.MustCompile(`\s+`)
)

// firstValue limpa o texto: espaços sobrando e, com vários valores
// separados por ";" ou "|", fica só o primeiro que não for vazio.
func firstValue(s string) string {
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == '|' }) {
		if part = strings.TrimSpace(part); part != "" {
			return reSpaces.ReplaceAllString(part, " ")
		}
	}
	return ""
}

// ParseDateTime lê data, hora (opcional, se não vier junto da data) e
// milissegundo (opcional) e devolve o instante em UTC. order é DateOrderDMY
// (ou vazio) ou DateOrderMDY; loc é o fuso de quando o texto não tem um.
func ParseDateTime(date, clock, ms, order string, loc *time.Location) (time.Time, error) {
	date, clock, ms = firstValue(date), firstValue(clock), firstValue(ms)
	if date == "" {
		return time.Time{}, fmt.Errorf("sem data")
	}
	if loc == nil {
		loc = time.UTC
	}
	t, err := parseDateTime(date, clock, ms, order, loc)
	if err != nil {
		return time.Time{}, err
	}
	return plausible(t, loc)
}

// plausible: de 2000 até amanhã. Fora disso é relógio errado, não uma data.
func plausible(t time.Time, loc *time.Location) (time.Time, error) {
	if t.Before(minDate) || t.After(now().Add(24*time.Hour)) {
		return time.Time{}, fmt.Errorf("data %s fora do esperado (de 2000 até hoje): relógio do equipamento errado?", t.In(loc).Format("02/01/2006 15:04:05"))
	}
	return t.UTC(), nil
}

func parseDateTime(date, clock, ms, order string, loc *time.Location) (time.Time, error) {
	shown := strings.TrimSpace(date + " " + clock)

	// Tudo num número: epoch (10 ou 13 dígitos) ou AAAAMMDDHHMMSS.
	if clock == "" && reDigits.MatchString(date) {
		switch len(date) {
		case 10, 13:
			n, _ := strconv.ParseInt(date, 10, 64)
			if len(date) == 13 {
				return time.UnixMilli(n), nil
			}
			return time.Unix(n, 0), nil
		case 12, 14:
			date, clock = date[:8], date[8:]
		}
	}

	y, m, d, rest, err := splitDate(date, order)
	if err != nil {
		return time.Time{}, fmt.Errorf("data %q: %v", shown, err)
	}
	if rest != "" {
		if clock != "" {
			return time.Time{}, fmt.Errorf("data %q: a data já tem hora, e veio outra hora separada", shown)
		}
		clock = rest
	}

	// O fuso, se o texto tiver um (no fim da hora: "08:35:42-03:00", "Z").
	zone := loc
	if z := reZone.FindStringSubmatch(clock); z != nil && clock != "" {
		fixed, err := zoneOf(z[1])
		if err != nil {
			return time.Time{}, fmt.Errorf("data %q: %v", shown, err)
		}
		zone = fixed
		clock = strings.TrimSpace(clock[:len(clock)-len(z[0])])
	}

	hh, mm, ss, nanos, err := splitClock(clock)
	if err != nil {
		return time.Time{}, fmt.Errorf("hora %q: %v", clock, err)
	}
	if ms != "" && nanos == 0 { // a hora já com fração ganha do milissegundo separado
		n, err := parseMillis(ms)
		if err != nil {
			return time.Time{}, fmt.Errorf("milissegundo %q: %v", ms, err)
		}
		nanos = n
	}

	t := time.Date(y, time.Month(m), d, hh, mm, ss, nanos, zone)
	if t.Day() != d || int(t.Month()) != m { // 31/02 "vira" 03/03 no Go: aqui é erro
		return time.Time{}, fmt.Errorf("data %q: o dia %d não existe em %02d/%d", shown, d, m, y)
	}
	return t, nil
}

// splitDate acha a data no começo do texto e devolve o que sobra (a hora,
// se veio junto, separada por espaço ou "T").
func splitDate(s, order string) (y, m, d int, rest string, err error) {
	var a, b, c string // na ordem do texto
	ymd := false
	if g := reDateYMD.FindStringSubmatch(s); g != nil {
		a, b, c, ymd = g[1], g[2], g[3], true
		rest = s[len(g[0]):]
	} else if g := reDateDMY.FindStringSubmatch(s); g != nil {
		a, b, c = g[1], g[2], g[3]
		rest = s[len(g[0]):]
	} else if g := reDateYMDRaw.FindStringSubmatch(s); g != nil {
		a, b, c, ymd = g[1], g[2], g[3], true
		rest = s[len(g[0]):]
	} else {
		return 0, 0, 0, "", fmt.Errorf("não é uma data (use dia/mês/ano ou ano-mês-dia)")
	}
	if rest != "" {
		if rest[0] != ' ' && rest[0] != 'T' && rest[0] != 't' {
			return 0, 0, 0, "", fmt.Errorf("texto sobrando depois da data")
		}
		rest = strings.TrimSpace(rest[1:])
	}

	if ymd {
		y, m, d = atoi(a), atoi(b), atoi(c)
	} else {
		first, second := atoi(a), atoi(b)
		d, m = first, second
		if order == DateOrderMDY {
			d, m = second, first
		}
		y = atoi(c)
		if len(c) == 2 {
			y += 2000
		}
	}
	if m < 1 || m > 12 {
		if !ymd && order != DateOrderMDY && d >= 1 && d <= 12 && m <= 31 {
			return 0, 0, 0, "", fmt.Errorf("mês %d não existe (a data parece estar como mês/dia: use a ordem \"mdy\" no perfil)", m)
		}
		return 0, 0, 0, "", fmt.Errorf("mês %d não existe", m)
	}
	if d < 1 || d > 31 {
		return 0, 0, 0, "", fmt.Errorf("dia %d não existe", d)
	}
	return y, m, d, rest, nil
}

// splitClock lê a hora (vazia = meia-noite).
func splitClock(s string) (hh, mm, ss, nanos int, err error) {
	if s == "" {
		return 0, 0, 0, 0, nil
	}
	pm, ampm := false, false
	if g := reAMPM.FindStringSubmatch(s); g != nil {
		ampm, pm = true, strings.EqualFold(g[1], "p")
		s = strings.TrimSpace(s[:len(s)-len(g[0])])
	}
	var frac string
	if g := reClock.FindStringSubmatch(s); g != nil {
		hh, mm, ss, frac = atoi(g[1]), atoi(g[2]), atoi(g[3]), g[4]
	} else if g := reClockRaw.FindStringSubmatch(s); g != nil {
		hh, mm, ss = atoi(g[1]), atoi(g[2]), atoi(g[3])
	} else {
		return 0, 0, 0, 0, fmt.Errorf("não é uma hora (use HH:MM:SS)")
	}
	if ampm {
		if hh < 1 || hh > 12 {
			return 0, 0, 0, 0, fmt.Errorf("com AM/PM a hora vai de 1 a 12")
		}
		hh %= 12
		if pm {
			hh += 12
		}
	}
	if hh > 23 || mm > 59 || ss > 59 {
		return 0, 0, 0, 0, fmt.Errorf("hora, minuto ou segundo fora do limite")
	}
	if frac != "" { // fração de segundo: ".5" = 500 ms
		nanos = atoi((frac + "000000000")[:9])
	}
	return hh, mm, ss, nanos, nil
}

// parseMillis: o milissegundo numa coluna própria é um número de ms (até 3
// dígitos: "12" = 12 ms). Com mais dígitos é a fração do segundo
// ("123456" = 0,123456 s).
func parseMillis(s string) (int, error) {
	if !reDigits.MatchString(s) {
		return 0, fmt.Errorf("não é um número")
	}
	if len(s) <= 3 {
		return atoi(s) * int(time.Millisecond), nil
	}
	if len(s) > 9 {
		return 0, fmt.Errorf("casas demais")
	}
	return atoi((s + "000000000")[:9]), nil
}

// zoneOf: "Z", "UTC", "-03:00", "+0530".
func zoneOf(s string) (*time.Location, error) {
	switch strings.ToUpper(s) {
	case "Z", "UTC", "GMT":
		return time.UTC, nil
	}
	s = strings.ReplaceAll(s, ":", "")
	h, m := atoi(s[1:3]), atoi(s[3:5])
	if h > 14 || m > 59 {
		return nil, fmt.Errorf("fuso %q inválido", s)
	}
	off := (h*60 + m) * 60
	if s[0] == '-' {
		off = -off
	}
	return time.FixedZone(s, off), nil
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
