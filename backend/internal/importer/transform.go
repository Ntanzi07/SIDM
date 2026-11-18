package importer

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"SIDM/internal/annotation"
)

// Record é uma imagem descrita pelo arquivo: a chave (nome original ou sha1)
// e a anotação, nos campos do perfil de anotação (ainda não conferida: o
// Fit do perfil é que limpa e avisa do que não serve). Line é a primeira
// linha do arquivo que fala dela.
type Record struct {
	Key    string            `json:"key"`
	Line   int               `json:"line"`
	Values annotation.Values `json:"values"`
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

// Transform aplica o perfil em todas as linhas. ap é o perfil de anotação
// do pacote: diz o tipo de cada campo (lista, número, data...) e os valores
// aceitos. Um campo que ap não tem vai como texto, e o Fit do perfil avisa.
func Transform(p *Profile, ap *annotation.Profile, t *Table) ([]Record, Report) {
	if p.Vehicles != nil { // formato antigo, ainda não normalizado
		cp := *p
		cp.Normalize()
		p = &cp
	}
	rep := Report{Rows: len(t.Rows), Unmapped: map[string]map[string]int{}, Errors: []RowError{}}
	tr := &transformer{p: p, ap: ap, rep: &rep}

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
			rec = &Record{Key: key, Line: row.Line, Values: annotation.Values{Fields: map[string]any{}, Objects: []annotation.Object{}}}
			byKey[key] = rec
			order = append(order, key)
			// Campos da imagem vêm da PRIMEIRA linha da imagem.
			rec.Values.Fields = tr.imageFields(row)
		}
		rec.Values.Objects = append(rec.Values.Objects, tr.objects(row)...)
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
	ap  *annotation.Profile
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

// translate passa o valor pelo Map. Valor sem tradução: devolve "" e anota
// no relatório (o ensaio mostra em "valores sem tradução").
//
// Sem Map, um campo de texto livre (placa, equipamento) vai como está. Um
// campo de LISTA (choice/multi: tipo, cor, período...) só aceita, sem
// diferenciar maiúscula, o próprio valor ("car"), o código do nome do
// arquivo ("C") ou o nome da tela ("Carro"). Antes, sem Map, qualquer texto
// passava ("AUTOMOVEL") e quebrava a verificação depois.
func (tr *transformer) translate(where string, f *annotation.Field, value string, rule FieldRule) string {
	if value == "" {
		return value
	}
	if rule.Map == nil {
		if f == nil || (f.Type != annotation.TypeChoice && f.Type != annotation.TypeMulti) {
			return value
		}
		if v, ok := f.OptionOf(value); ok {
			return v
		}
		tr.unmapped(where, value)
		return ""
	}
	for from, to := range rule.Map {
		if strings.EqualFold(strings.TrimSpace(from), value) {
			return to
		}
	}
	tr.unmapped(where, value)
	return ""
}

// patternSeparators: o que sai de um texto com formato (ex.: a placa
// "ABC-1234") quando ele só passa no formato sem os separadores.
var patternSeparators = strings.NewReplacer("-", "", " ", "", ".", "")

// fitPattern: um texto com formato que só não passa por causa de
// separadores ("ABC-1234" numa placa [A-Z0-9]{7}) vai sem eles.
func fitPattern(f *annotation.Field, s string) string {
	if f.Type != annotation.TypeText || f.Pattern == "" {
		return s
	}
	re, err := regexp.Compile(`^(?:` + f.Pattern + `)$`)
	if err != nil {
		return s
	}
	check := s
	if f.Upper {
		check = strings.ToUpper(s)
	}
	if re.MatchString(check) {
		return s
	}
	if bare := patternSeparators.Replace(check); re.MatchString(bare) {
		return bare
	}
	return s
}

// value lê e converte UM campo: o valor bruto da regra, traduzido e no tipo
// do campo. nil = vazio (ou sem tradução, ou inválido: já anotado no
// relatório). f nil: o perfil de anotação não tem o campo, e o texto vai
// como veio (o Fit avisa).
func (tr *transformer) value(row Row, where string, f *annotation.Field, rule FieldRule, n int) any {
	raw := tr.raw(row, rule, n)
	if raw == "" {
		return nil
	}
	if f == nil {
		return raw
	}
	switch f.Type {
	case annotation.TypeDateTime:
		t, err := parseDate(raw, rule)
		if err != nil {
			tr.addError(row.Line, where, err.Error())
			return nil
		}
		return t

	case annotation.TypeMulti:
		parts := []string{raw}
		if rule.Split != "" {
			parts = strings.Split(raw, rule.Split)
		}
		var out []string
		for _, part := range parts {
			if v := tr.translate(where, f, strings.TrimSpace(part), rule); v != "" && !contains(out, v) {
				out = append(out, v)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	}

	s := tr.translate(where, f, raw, rule)
	if s == "" {
		return nil
	}
	v, err := f.Typed(s)
	if err != nil {
		tr.addError(row.Line, where, err.Error())
		return nil
	}
	if str, ok := v.(string); ok {
		return fitPattern(f, str)
	}
	return v
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

func (tr *transformer) imageFields(row Row) map[string]any {
	out := map[string]any{}
	for _, name := range sortedRules(tr.p.Fields) { // ordem fixa: o relatório sai igual em toda execução
		var f *annotation.Field
		if tr.ap != nil {
			f, _ = tr.ap.Field(name)
		}
		if v := tr.value(row, name, f, tr.p.Fields[name], 0); v != nil {
			out[name] = v
		}
	}
	return out
}

// objects monta os objetos da linha, tipo por tipo (em ordem de nome).
func (tr *transformer) objects(row Row) []annotation.Object {
	types := make([]string, 0, len(tr.p.Objects))
	for t := range tr.p.Objects {
		types = append(types, t)
	}
	sort.Strings(types)

	var out []annotation.Object
	for _, t := range types {
		rules := tr.p.Objects[t]
		if rules.Mode != ObjectsPerColumns {
			if o, ok := tr.object(row, t, rules, 0); ok {
				out = append(out, o)
			}
			continue
		}
		for n := 1; n <= rules.Max; n++ {
			if o, ok := tr.object(row, t, rules, n); ok {
				out = append(out, o)
			}
		}
	}
	return out
}

// object monta o objeto n da linha (n=0 fora do modo "columns"). Sem valor
// no campo Key (ex: Placa2 vazia), ou, sem Key, em nenhum campo que vem de
// coluna, não há objeto ali.
func (tr *transformer) object(row Row, typ string, rules *ObjectRules, n int) (annotation.Object, bool) {
	var def *annotation.ObjectType
	if tr.ap != nil {
		def, _ = tr.ap.Object(typ)
	}
	if rules.Key != "" {
		if tr.raw(row, rules.Fields[rules.Key], n) == "" {
			return annotation.Object{}, false
		}
	} else {
		present := false
		for _, rule := range rules.Fields {
			if rule.Value == "" && tr.raw(row, rule, n) != "" {
				present = true
				break
			}
		}
		if !present {
			return annotation.Object{}, false
		}
	}

	fields := map[string]any{}
	for _, name := range sortedRules(rules.Fields) {
		var f *annotation.Field
		if def != nil {
			f, _ = def.Field(name)
		}
		if v := tr.value(row, typ+"."+name, f, rules.Fields[name], n); v != nil {
			fields[name] = v
		}
	}
	return annotation.Object{Type: typ, Fields: fields}, true
}

func sortedRules(m map[string]FieldRule) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
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
