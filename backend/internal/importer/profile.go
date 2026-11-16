// Package importer lê arquivos de metadados (CSV hoje; INI/EXIF no futuro)
// e transforma cada linha numa anotação, seguindo um PERFIL.
//
// Um CSV novo não exige código novo: exige um perfil novo. O perfil é só
// configuração (JSON, guardado no Mongo) e diz:
//   - como ler o arquivo (separador, encoding);
//   - qual coluna identifica a imagem;
//   - de qual coluna vem cada campo nosso, e como traduzir os valores.
//
// Este pacote não sabe nada de banco nem de HTTP: recebe linhas, devolve
// anotações e um relatório do que não deu para traduzir.
package importer

import (
	"fmt"
	"sort"
	"strings"
	"time"
	_ "time/tzdata" // fusos embutidos no binário: no Docker (alpine) não existe /usr/share/zoneinfo

	"SIDM/internal/filename"
)

// Profile descreve um formato de arquivo de metadados.
type Profile struct {
	Name        string `json:"name" bson:"name"`
	Description string `json:"description,omitempty" bson:"description,omitempty"`

	Reader ReaderConfig         `json:"reader" bson:"reader"`
	Match  MatchConfig          `json:"match" bson:"match"`
	Fields map[string]FieldRule `json:"fields" bson:"fields"`

	// Vehicles é opcional: há CSVs sem dados de veículo.
	Vehicles *VehicleRules `json:"vehicles,omitempty" bson:"vehicles,omitempty"`

	CreatedBy string    `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`
}

// ReaderConfig: como ler o arquivo.
type ReaderConfig struct {
	Type      string `json:"type" bson:"type"`                               // "csv"
	Delimiter string `json:"delimiter,omitempty" bson:"delimiter,omitempty"` // padrão ","
	Encoding  string `json:"encoding,omitempty" bson:"encoding,omitempty"`   // "utf-8" (padrão) ou "latin1"
}

// MatchConfig: qual coluna identifica a imagem, e comparada com o quê.
type MatchConfig struct {
	Column string `json:"column" bson:"column"`
	By     string `json:"by" bson:"by"` // "original_name" ou "sha1"
}

const (
	MatchByOriginalName = "original_name"
	MatchBySHA1         = "sha1"
)

// FieldRule: de onde vem um campo e como transformar o valor.
// Exatamente um de Column, Value ou Template.
type FieldRule struct {
	Column string `json:"column,omitempty" bson:"column,omitempty"` // valor vem desta coluna
	Value  string `json:"value,omitempty" bson:"value,omitempty"`   // valor fixo para o arquivo todo

	// Template junta várias colunas: "{Metrologico.Data} {Metrologico.Hora}".
	// Se alguma coluna citada estiver vazia, o resultado é vazio (campo não
	// preenchido), em vez de um valor pela metade.
	Template string `json:"template,omitempty" bson:"template,omitempty"`

	// Map traduz o valor de origem para o nosso ("N" -> "night").
	// A comparação ignora maiúscula e espaços nas pontas.
	Map map[string]string `json:"map,omitempty" bson:"map,omitempty"`

	// Split separa uma lista numa célula só ("03,05"). Só para conditions.
	Split string `json:"split,omitempty" bson:"split,omitempty"`

	// Só para captured_at: layout no formato do Go (ex: "02/01/2006 15:04:05")
	// e o fuso em que a data foi escrita (ex: "America/Sao_Paulo").
	DateFormat string `json:"date_format,omitempty" bson:"date_format,omitempty"`
	Timezone   string `json:"timezone,omitempty" bson:"timezone,omitempty"`
}

// VehicleRules: como achar os veículos no arquivo. Três jeitos:
//
//	"single":  um veículo só por imagem (é o caso do INI: "Placa = ...").
//	"row":     cada LINHA é um veículo; linhas da mesma imagem são agrupadas.
//	"columns": colunas numeradas na mesma linha (Placa1, Placa2...). Os nomes
//	           de coluna usam "{n}" no lugar do número: "Placa{n}".
type VehicleRules struct {
	Mode   string               `json:"mode" bson:"mode"`
	Max    int                  `json:"max,omitempty" bson:"max,omitempty"` // só em "columns"
	Fields map[string]FieldRule `json:"fields" bson:"fields"`
}

const (
	VehiclesSingle     = "single"
	VehiclesPerRow     = "row"
	VehiclesPerColumns = "columns"
)

// Campos nossos que um perfil pode preencher.
var (
	imageFields   = []string{"equipment", "camera", "direction", "resolution", "capture_angle", "day_time", "image_range", "weather", "conditions", "captured_at"}
	vehicleFields = []string{"plate", "plate_type", "plate_category", "type", "color"}

	// Campo nosso -> tabela de códigos (para validar os valores do Map).
	// Campos fora daqui (equipment, camera, plate) aceitam texto livre.
	codeTable = map[string]string{
		"direction": "direction", "resolution": "resolution", "capture_angle": "capture_angle",
		"day_time": "day_time", "image_range": "image_range", "weather": "weather",
		"conditions": "conditions",
		"plate_type": "plate_type", "plate_category": "plate_category",
		"type": "vehicle_type", "color": "vehicle_color",
	}
)

// ValidDateLayout diz se o texto é um layout do Go que tem, pelo menos,
// dia, mês e ano. O jeito de saber: formatar uma data conhecida com ele e
// ler de volta. Um layout errado (por exemplo uma data de exemplo, como
// "2026-08-12 09:23:23") não devolve a mesma data.
func ValidDateLayout(layout string) bool {
	ref := time.Date(2017, 11, 23, 8, 49, 37, 987_000_000, time.UTC)
	got, err := time.Parse(layout, ref.Format(layout))
	return err == nil && got.Year() == ref.Year() && got.Month() == ref.Month() && got.Day() == ref.Day()
}

// ValidProfileName diz se o nome serve para um perfil: só letras, números e "-".
func ValidProfileName(name string) bool {
	_, err := filename.SanitizePackage(name)
	return err == nil && !strings.Contains(name, " ")
}

// Validate confere o perfil ANTES de salvar, para um erro de digitação no
// JSON aparecer na hora, e não no meio de um import de 5000 linhas.
func (p *Profile) Validate() error {
	var errs []string
	add := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }

	if !ValidProfileName(p.Name) {
		add("name: use só letras, números e \"-\" (ex: csv-equipamento-8003)")
	}

	switch p.Reader.Type {
	case "csv":
		if p.Match.Column == "" {
			add("match.column: obrigatório (a coluna que identifica a imagem)")
		}
		if p.Match.By != MatchByOriginalName && p.Match.By != MatchBySHA1 {
			add("match.by: use %q ou %q", MatchByOriginalName, MatchBySHA1)
		}
	case ReaderJPEGINI:
		// O INI está DENTRO da imagem: não há o que casar.
		if p.Match != (MatchConfig{}) {
			add("match: não se usa com %q (o INI já está dentro da própria imagem)", ReaderJPEGINI)
		}
		if p.Vehicles != nil && p.Vehicles.Mode == VehiclesPerRow {
			add("vehicles.mode: com %q use \"single\" (o INI é uma linha só por imagem)", ReaderJPEGINI)
		}
	default:
		add("reader.type: %q não suportado (use \"csv\" ou %q)", p.Reader.Type, ReaderJPEGINI)
	}
	if d := p.Reader.Delimiter; d != "" && len([]rune(d)) != 1 {
		add("reader.delimiter: deve ser um caractere só (ex: \";\")")
	}
	switch strings.ToLower(p.Reader.Encoding) {
	case "", "utf-8", "utf8", "latin1", "iso-8859-1":
	default:
		add("reader.encoding: %q não suportado (use \"utf-8\" ou \"latin1\")", p.Reader.Encoding)
	}

	if len(p.Fields) == 0 && p.Vehicles == nil {
		add("fields: o perfil não preenche nada")
	}
	for name, rule := range p.Fields {
		if !contains(imageFields, name) {
			add("fields.%s: campo desconhecido (válidos: %s)", name, strings.Join(imageFields, ", "))
			continue
		}
		validateRule(add, "fields."+name, name, rule)
	}

	if v := p.Vehicles; v != nil {
		switch v.Mode {
		case VehiclesSingle, VehiclesPerRow:
		case VehiclesPerColumns:
			if v.Max < 1 || v.Max > 20 {
				add("vehicles.max: entre 1 e 20 no modo \"columns\"")
			}
		default:
			add("vehicles.mode: use %q, %q ou %q", VehiclesSingle, VehiclesPerRow, VehiclesPerColumns)
		}
		if _, ok := v.Fields["plate"]; !ok {
			add("vehicles.fields.plate: obrigatório (é o que diz que o veículo existe)")
		}
		for name, rule := range v.Fields {
			if !contains(vehicleFields, name) {
				add("vehicles.fields.%s: campo desconhecido (válidos: %s)", name, strings.Join(vehicleFields, ", "))
				continue
			}
			if v.Mode == VehiclesPerColumns && rule.Column+rule.Template != "" && !strings.Contains(rule.Column+rule.Template, "{n}") {
				add("vehicles.fields.%s.column: no modo \"columns\" use \"{n}\" no nome (ex: \"Placa{n}\")", name)
			}
			validateRule(add, "vehicles.fields."+name, name, rule)
		}
	}

	if len(errs) > 0 {
		sort.Strings(errs)
		return fmt.Errorf("perfil inválido:\n- %s", strings.Join(errs, "\n- "))
	}
	return nil
}

func validateRule(add func(string, ...any), path, field string, r FieldRule) {
	set := 0
	for _, v := range []string{r.Column, r.Value, r.Template} {
		if v != "" {
			set++
		}
	}
	if set != 1 {
		add("%s: use \"column\", \"value\" OU \"template\" (exatamente um)", path)
	}
	if r.Template != "" && !strings.Contains(r.Template, "{") {
		add("%s.template: cite as colunas entre chaves, ex: \"{Secao.Data} {Secao.Hora}\"", path)
	}
	if r.Split != "" && field != "conditions" {
		add("%s.split: só faz sentido em conditions", path)
	}

	if field == "captured_at" {
		if r.DateFormat == "" {
			add("%s.date_format: obrigatório (layout do Go, ex: \"02/01/2006 15:04:05\")", path)
		} else if !ValidDateLayout(r.DateFormat) {
			add("%s.date_format: %q não é um layout do Go. Escreva a data de referência "+
				"(2006-01-02 15:04:05) no mesmo formato do CSV, ex: \"2006-01-02 15:04:05.000\" ou \"02/01/2006 15:04:05\"",
				path, r.DateFormat)
		}
		if r.Timezone != "" {
			if _, err := time.LoadLocation(r.Timezone); err != nil {
				add("%s.timezone: fuso desconhecido %q (ex: \"America/Sao_Paulo\")", path, r.Timezone)
			}
		}
	} else if r.DateFormat != "" || r.Timezone != "" {
		add("%s: date_format/timezone só valem para captured_at", path)
	}

	// Os valores de destino do Map precisam ser códigos que existem no padrão.
	if table, ok := codeTable[field]; ok {
		valid := validValues(table)
		for from, to := range r.Map {
			if !valid[to] {
				add("%s.map[%q]: %q não é um valor válido (válidos: %s)", path, from, to, strings.Join(sortedKeys(valid), ", "))
			}
		}
		if r.Value != "" && r.Map == nil && !valid[r.Value] {
			add("%s.value: %q não é um valor válido", path, r.Value)
		}
	}
}

// validValues: os valores ("night", "rain"...) de uma tabela do padrão.
func validValues(table string) map[string]bool {
	out := map[string]bool{}
	for _, v := range filename.Codes()[table] {
		out[v] = true
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
