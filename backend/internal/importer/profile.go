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
// Exatamente um de Column ou Value.
type FieldRule struct {
	Column string `json:"column,omitempty" bson:"column,omitempty"` // valor vem desta coluna
	Value  string `json:"value,omitempty" bson:"value,omitempty"`   // valor fixo para o arquivo todo

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

// VehicleRules: como achar os veículos no arquivo. Dois jeitos:
//
//	"row":     cada LINHA é um veículo; linhas da mesma imagem são agrupadas.
//	"columns": colunas numeradas na mesma linha (Placa1, Placa2...). Os nomes
//	           de coluna usam "{n}" no lugar do número: "Placa{n}".
type VehicleRules struct {
	Mode   string               `json:"mode" bson:"mode"`
	Max    int                  `json:"max,omitempty" bson:"max,omitempty"` // só em "columns"
	Fields map[string]FieldRule `json:"fields" bson:"fields"`
}

const (
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

// Validate confere o perfil ANTES de salvar, para um erro de digitação no
// JSON aparecer na hora, e não no meio de um import de 5000 linhas.
func (p *Profile) Validate() error {
	var errs []string
	add := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }

	if _, err := filename.SanitizePackage(p.Name); err != nil || strings.Contains(p.Name, " ") {
		add("name: use só letras, números e \"-\" (ex: csv-equipamento-8003)")
	}

	switch p.Reader.Type {
	case "csv":
	default:
		add("reader.type: %q não suportado (use \"csv\")", p.Reader.Type)
	}
	if d := p.Reader.Delimiter; d != "" && len([]rune(d)) != 1 {
		add("reader.delimiter: deve ser um caractere só (ex: \";\")")
	}
	switch strings.ToLower(p.Reader.Encoding) {
	case "", "utf-8", "utf8", "latin1", "iso-8859-1":
	default:
		add("reader.encoding: %q não suportado (use \"utf-8\" ou \"latin1\")", p.Reader.Encoding)
	}

	if p.Match.Column == "" {
		add("match.column: obrigatório (a coluna que identifica a imagem)")
	}
	if p.Match.By != MatchByOriginalName && p.Match.By != MatchBySHA1 {
		add("match.by: use %q ou %q", MatchByOriginalName, MatchBySHA1)
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
		case VehiclesPerRow:
		case VehiclesPerColumns:
			if v.Max < 1 || v.Max > 20 {
				add("vehicles.max: entre 1 e 20 no modo \"columns\"")
			}
		default:
			add("vehicles.mode: use %q ou %q", VehiclesPerRow, VehiclesPerColumns)
		}
		if _, ok := v.Fields["plate"]; !ok {
			add("vehicles.fields.plate: obrigatório (é o que diz que o veículo existe)")
		}
		for name, rule := range v.Fields {
			if !contains(vehicleFields, name) {
				add("vehicles.fields.%s: campo desconhecido (válidos: %s)", name, strings.Join(vehicleFields, ", "))
				continue
			}
			if v.Mode == VehiclesPerColumns && rule.Column != "" && !strings.Contains(rule.Column, "{n}") {
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
	if (r.Column == "") == (r.Value == "") {
		add("%s: use \"column\" OU \"value\" (exatamente um)", path)
	}
	if r.Split != "" && field != "conditions" {
		add("%s.split: só faz sentido em conditions", path)
	}

	if field == "captured_at" {
		if r.DateFormat == "" {
			add("%s.date_format: obrigatório (layout do Go, ex: \"02/01/2006 15:04:05\")", path)
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
