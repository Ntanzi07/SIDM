package importer

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"SIDM/internal/filename"
)

// Annotation é o que o import descobriu sobre UMA imagem.
// Campo vazio = o arquivo não disse nada (ou o valor não tinha tradução).
type Annotation struct {
	Equipment    string     `json:"equipment,omitempty"`
	Camera       string     `json:"camera,omitempty"`
	Direction    string     `json:"direction,omitempty"`
	Resolution   string     `json:"resolution,omitempty"`
	CaptureAngle string     `json:"capture_angle,omitempty"`
	DayTime      string     `json:"day_time,omitempty"`
	ImageRange   string     `json:"image_range,omitempty"`
	Weather      string     `json:"weather,omitempty"`
	Conditions   []string   `json:"conditions,omitempty"`
	CapturedAt   *time.Time `json:"captured_at,omitempty"`
	Vehicles     []Vehicle  `json:"vehicles,omitempty"`
}

type Vehicle struct {
	Plate         string `json:"plate"`
	PlateType     string `json:"plate_type,omitempty"`
	PlateCategory string `json:"plate_category,omitempty"`
	Type          string `json:"type,omitempty"`
	Color         string `json:"color,omitempty"`
}

// Record é uma imagem descrita pelo arquivo: a chave (nome original ou sha1)
// e a anotação. Line é a primeira linha do arquivo que fala dela.
type Record struct {
	Key        string     `json:"key"`
	Line       int        `json:"line"`
	Annotation Annotation `json:"annotation"`
}

// Report diz o que NÃO deu certo na transformação. Nada aqui é fatal: o
// campo problemático fica vazio e o resto da linha é aproveitado.
type Report struct {
	Rows    int `json:"rows"`    // linhas lidas
	Records int `json:"records"` // imagens descritas (depois de agrupar)

	// Unmapped: valores sem tradução no "map", por campo, com a contagem.
	// É a lista do que falta adicionar no perfil.
	Unmapped map[string]map[string]int `json:"unmapped"`

	MissingKey int        `json:"missing_key"` // linhas sem a coluna de identificação
	Errors     []RowError `json:"errors"`      // datas inválidas etc. (até maxErrors)
	ErrorCount int        `json:"error_count"`
}

type RowError struct {
	Key   string `json:"key,omitempty"` // no import de INI: o sha1 da imagem
	Line  int    `json:"line"`
	Field string `json:"field"`
	Error string `json:"error"`
}

const maxErrors = 100

// Transform aplica o perfil em todas as linhas.
func Transform(p *Profile, t *Table) ([]Record, Report) {
	rep := Report{Rows: len(t.Rows), Unmapped: map[string]map[string]int{}, Errors: []RowError{}}
	tr := &transformer{p: p, rep: &rep}

	byKey := map[string]*Record{}
	var order []string

	for _, row := range t.Rows {
		key := strings.TrimSpace(row.Values[p.Match.Column])
		if p.Match.By == MatchBySHA1 {
			key = strings.ToLower(key)
		}
		if key == "" {
			rep.MissingKey++
			continue
		}

		rec, seen := byKey[key]
		if !seen {
			rec = &Record{Key: key, Line: row.Line}
			byKey[key] = rec
			order = append(order, key)
			// Campos da imagem vêm da PRIMEIRA linha da imagem.
			tr.imageFields(row, &rec.Annotation)
		}
		rec.Annotation.Vehicles = append(rec.Annotation.Vehicles, tr.vehicles(row)...)
	}

	records := make([]Record, 0, len(order))
	for _, k := range order {
		records = append(records, *byKey[k])
	}
	rep.Records = len(records)
	return records, rep
}

type transformer struct {
	p   *Profile
	rep *Report
}

// raw lê o valor bruto de uma regra: o valor fixo, a coluna, ou o template
// (sempre com {n} trocado pelo número do veículo, no modo "columns").
func (tr *transformer) raw(row Row, rule FieldRule, n int) string {
	if rule.Value != "" {
		return rule.Value
	}
	num := func(s string) string {
		if n > 0 {
			return strings.ReplaceAll(s, "{n}", strconv.Itoa(n))
		}
		return s
	}
	if rule.Template != "" {
		return fillTemplate(num(rule.Template), row.Values)
	}
	return strings.TrimSpace(row.Values[num(rule.Column)])
}

var placeholderRe = regexp.MustCompile(`\{([^{}]+)\}`)

// fillTemplate troca cada {coluna} pelo valor. Coluna vazia: resultado vazio.
func fillTemplate(tpl string, values map[string]string) string {
	missing := false
	out := placeholderRe.ReplaceAllStringFunc(tpl, func(m string) string {
		v := strings.TrimSpace(values[m[1:len(m)-1]])
		if v == "" {
			missing = true
		}
		return v
	})
	if missing {
		return ""
	}
	return strings.TrimSpace(out)
}

// codeLookup: para cada tabela do padrão, o que o CSV pode trazer sem um
// "map" (o valor, "car", ou a letra do nome do arquivo, "C"), em minúsculas,
// apontando para o valor certo. Montado uma vez só.
var codeLookup = func() map[string]map[string]string {
	out := map[string]map[string]string{}
	for table, codes := range filename.Codes() {
		m := map[string]string{}
		for code, value := range codes {
			m[strings.ToLower(code)] = value
			m[strings.ToLower(value)] = value
		}
		out[table] = m
	}
	return out
}()

// translate passa o valor pelo Map. Valor sem tradução: devolve "" e anota
// no relatório (o ensaio mostra em "valores sem tradução").
//
// Sem Map, um campo de texto livre (placa, equipamento) vai como está. Um
// campo CODIFICADO (tipo, cor, período...) só aceita o próprio valor ("car")
// ou a letra do padrão ("C"), sem diferenciar maiúscula. Antes, sem Map,
// qualquer texto passava ("AUTOMOVEL") e quebrava a verificação depois.
func (tr *transformer) translate(field, value string, rule FieldRule) string {
	if value == "" {
		return value
	}
	if rule.Map == nil {
		table, coded := codeTable[strings.TrimPrefix(field, "vehicles.")]
		if !coded {
			return value
		}
		if v, ok := codeLookup[table][strings.ToLower(value)]; ok {
			return v
		}
		tr.unmapped(field, value)
		return ""
	}
	for from, to := range rule.Map {
		if strings.EqualFold(strings.TrimSpace(from), value) {
			return to
		}
	}
	tr.unmapped(field, value)
	return ""
}

func (tr *transformer) unmapped(field, value string) {
	if tr.rep.Unmapped[field] == nil {
		tr.rep.Unmapped[field] = map[string]int{}
	}
	tr.rep.Unmapped[field][value]++
}

func (tr *transformer) addError(line int, field, msg string) {
	tr.rep.ErrorCount++
	if len(tr.rep.Errors) < maxErrors {
		tr.rep.Errors = append(tr.rep.Errors, RowError{Line: line, Field: field, Error: msg})
	}
}

func (tr *transformer) imageFields(row Row, a *Annotation) {
	targets := map[string]*string{
		"equipment": &a.Equipment, "camera": &a.Camera, "direction": &a.Direction,
		"resolution": &a.Resolution, "capture_angle": &a.CaptureAngle, "day_time": &a.DayTime,
		"image_range": &a.ImageRange, "weather": &a.Weather,
	}

	// Ordem fixa: o relatório sai igual em toda execução.
	names := make([]string, 0, len(tr.p.Fields))
	for name := range tr.p.Fields {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		rule := tr.p.Fields[name]
		value := tr.raw(row, rule, 0)
		if value == "" {
			continue
		}

		switch name {
		case "captured_at":
			t, err := parseDate(value, rule)
			if err != nil {
				tr.addError(row.Line, name, err.Error())
				continue
			}
			a.CapturedAt = &t

		case "conditions":
			parts := []string{value}
			if rule.Split != "" {
				parts = strings.Split(value, rule.Split)
			}
			for _, part := range parts {
				if v := tr.translate(name, strings.TrimSpace(part), rule); v != "" && !contains(a.Conditions, v) {
					a.Conditions = append(a.Conditions, v)
				}
			}

		default:
			*targets[name] = tr.translate(name, value, rule)
		}
	}
}

func (tr *transformer) vehicles(row Row) []Vehicle {
	v := tr.p.Vehicles
	if v == nil {
		return nil
	}
	if v.Mode == VehiclesPerRow || v.Mode == VehiclesSingle {
		if veh, ok := tr.vehicle(row, 0); ok {
			return []Vehicle{veh}
		}
		return nil
	}

	var out []Vehicle
	for n := 1; n <= v.Max; n++ {
		if veh, ok := tr.vehicle(row, n); ok {
			out = append(out, veh)
		}
	}
	return out
}

// vehicle monta o veículo n da linha (n=0 no modo "row").
// Sem placa = não há veículo ali (ex: Placa2 vazia).
func (tr *transformer) vehicle(row Row, n int) (Vehicle, bool) {
	rules := tr.p.Vehicles.Fields
	plate := strings.ToUpper(strings.ReplaceAll(tr.raw(row, rules["plate"], n), "-", ""))
	if plate == "" {
		return Vehicle{}, false
	}

	get := func(field string) string {
		rule, ok := rules[field]
		if !ok {
			return ""
		}
		return tr.translate("vehicles."+field, tr.raw(row, rule, n), rule)
	}
	return Vehicle{
		Plate:         plate,
		PlateType:     get("plate_type"),
		PlateCategory: get("plate_category"),
		Type:          get("type"),
		Color:         get("color"),
	}, true
}

// parseDate lê a data no formato do perfil, no fuso do perfil, e devolve em UTC.
func parseDate(value string, rule FieldRule) (time.Time, error) {
	loc := time.UTC
	if rule.Timezone != "" {
		l, err := time.LoadLocation(rule.Timezone)
		if err != nil {
			return time.Time{}, fmt.Errorf("fuso %q desconhecido", rule.Timezone)
		}
		loc = l
	}
	t, err := time.ParseInLocation(rule.DateFormat, value, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("data %q não está no formato %q", value, rule.DateFormat)
	}
	return t.UTC(), nil
}
