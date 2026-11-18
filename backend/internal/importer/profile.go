// Package importer lê arquivos de metadados (CSV hoje; INI/EXIF no futuro)
// e transforma cada linha numa anotação, seguindo um PERFIL.
//
// Um CSV novo não exige código novo: exige um perfil novo. O perfil é só
// configuração (JSON, guardado no Mongo) e diz:
//   - qual perfil de anotação ele preenche (os campos de destino são os dele);
//   - como ler o arquivo (separador, encoding);
//   - qual coluna identifica a imagem;
//   - de qual coluna vem cada campo da imagem e de cada tipo de objeto, e
//     como traduzir os valores.
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

	"SIDM/internal/annotation"
	"SIDM/internal/filename"
)

// Profile descreve um formato de arquivo de metadados.
type Profile struct {
	Name        string `json:"name" bson:"name"`
	Description string `json:"description,omitempty" bson:"description,omitempty"`

	// AnnotationProfile é o perfil de anotação cujos campos este perfil
	// preenche (vazio = o padrão, "veiculos"). As chaves de Fields e de
	// Objects são as chaves desse perfil, e o import só aceita pacotes que
	// usam ele.
	AnnotationProfile string `json:"annotation_profile,omitempty" bson:"annotation_profile,omitempty"`

	Reader ReaderConfig         `json:"reader" bson:"reader"`
	Match  MatchConfig          `json:"match" bson:"match"`
	Fields map[string]FieldRule `json:"fields" bson:"fields"`

	// Objects: tipo de objeto (sem caixa) do perfil de anotação -> como
	// achar os objetos dele no arquivo. Opcional: há CSVs só com dados da
	// imagem.
	Objects map[string]*ObjectRules `json:"objects,omitempty" bson:"objects,omitempty"`

	// Vehicles é o formato de antes dos perfis de anotação (= objects.vehicle,
	// com key "plate"). Ainda é aceito na entrada; o Normalize passa para
	// Objects, e ele nunca é gravado de novo.
	Vehicles *ObjectRules `json:"vehicles,omitempty" bson:"vehicles,omitempty"`

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

	// Split separa uma lista numa célula só ("03,05"). Só em campo de
	// vários valores (multi), como conditions.
	Split string `json:"split,omitempty" bson:"split,omitempty"`

	// Só em campo de data e hora (como captured_at): layout no formato do Go
	// (ex: "02/01/2006 15:04:05") e o fuso em que a data foi escrita (ex:
	// "America/Sao_Paulo").
	DateFormat string `json:"date_format,omitempty" bson:"date_format,omitempty"`
	Timezone   string `json:"timezone,omitempty" bson:"timezone,omitempty"`
}

// ObjectRules: como achar os objetos de um tipo no arquivo. Três jeitos:
//
//	"single":  um objeto só por imagem (é o caso do INI: "Placa = ...").
//	"row":     cada LINHA é um objeto; linhas da mesma imagem são agrupadas.
//	"columns": colunas numeradas na mesma linha (Placa1, Placa2...). Os nomes
//	           de coluna usam "{n}" no lugar do número: "Placa{n}".
type ObjectRules struct {
	Mode string `json:"mode" bson:"mode"`
	Max  int    `json:"max,omitempty" bson:"max,omitempty"` // só em "columns"
	// Key é o campo que diz que o objeto existe (ex.: a placa): sem valor
	// nele, não há objeto ali. Vazio: o objeto existe se algum campo que
	// vem de coluna tem valor.
	Key    string               `json:"key,omitempty" bson:"key,omitempty"`
	Fields map[string]FieldRule `json:"fields" bson:"fields"`
}

// VehicleRules é o nome antigo de ObjectRules.
type VehicleRules = ObjectRules

const (
	ObjectsSingle     = "single"
	ObjectsPerRow     = "row"
	ObjectsPerColumns = "columns"

	// Nomes antigos.
	VehiclesSingle     = ObjectsSingle
	VehiclesPerRow     = ObjectsPerRow
	VehiclesPerColumns = ObjectsPerColumns
)

// legacyVehicle é o tipo de objeto que o formato antigo (vehicles) preenche.
const legacyVehicle = "vehicle"

// Normalize limpa o perfil antes de validar ou usar: o formato antigo
// (vehicles) vira objects.vehicle, e o perfil de anotação vazio vira o
// padrão. Pode ser chamado mais de uma vez.
func (p *Profile) Normalize() {
	p.AnnotationProfile = strings.TrimSpace(p.AnnotationProfile)
	if p.AnnotationProfile == "" {
		p.AnnotationProfile = annotation.DefaultProfile
	}
	if v := p.Vehicles; v != nil {
		if p.Objects == nil {
			p.Objects = map[string]*ObjectRules{}
		}
		if _, ok := p.Objects[legacyVehicle]; !ok {
			if _, hasPlate := v.Fields["plate"]; hasPlate && v.Key == "" {
				v.Key = "plate" // antes, a placa era o que dizia que o veículo existe
			}
			p.Objects[legacyVehicle] = v
		}
		p.Vehicles = nil
	}
	for t, o := range p.Objects {
		if o == nil {
			delete(p.Objects, t)
		}
	}
	if len(p.Objects) == 0 {
		p.Objects = nil
	}
}

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
// JSON aparecer na hora, e não no meio de um import de 5000 linhas. ap é o
// perfil de anotação que ele preenche (o de AnnotationProfile): os campos de
// destino e os valores dos map são conferidos contra ele. Chame depois do
// Normalize.
func (p *Profile) Validate(ap *annotation.Profile) error {
	var errs []string
	add := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }

	if !ValidProfileName(p.Name) {
		add("name: use só letras, números e \"-\" (ex: csv-equipamento-8003)")
	}
	if p.Vehicles != nil {
		add("vehicles: formato antigo; chame o Normalize (vira objects.vehicle)")
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
		for t, o := range p.Objects {
			if o.Mode == ObjectsPerRow {
				add("objects.%s.mode: com %q use \"single\" ou \"columns\" (o INI é uma linha só por imagem)", t, ReaderJPEGINI)
			}
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

	if ap == nil {
		add("annotation_profile: perfil de anotação %q não encontrado", p.AnnotationProfile)
	} else {
		if p.AnnotationProfile != "" && p.AnnotationProfile != ap.Name {
			add("annotation_profile: %q, mas os campos foram conferidos contra %q", p.AnnotationProfile, ap.Name)
		}
		p.validateTargets(add, ap)
	}

	if len(errs) > 0 {
		sort.Strings(errs)
		return fmt.Errorf("perfil inválido:\n- %s", strings.Join(errs, "\n- "))
	}
	return nil
}

// validateTargets confere os campos de destino contra o perfil de anotação.
func (p *Profile) validateTargets(add func(string, ...any), ap *annotation.Profile) {
	if len(p.Fields) == 0 && len(p.Objects) == 0 {
		add("fields: o perfil não preenche nada")
	}
	for name, rule := range p.Fields {
		f, ok := ap.Field(name)
		if !ok {
			add("fields.%s: o perfil de anotação %s não tem este campo (válidos: %s)", name, ap.Name, strings.Join(fieldKeys(ap.Fields), ", "))
			continue
		}
		validateRule(add, "fields."+name, f, rule)
	}

	for typ, o := range p.Objects {
		base := "objects." + typ
		t, ok := ap.Object(typ)
		switch {
		case !ok:
			add("%s: o perfil de anotação %s não tem este tipo de objeto (válidos: %s)", base, ap.Name, strings.Join(listObjects(ap), ", "))
			continue
		case t.Box:
			add("%s: %q tem caixa (vem do CVAT): o import não preenche", base, typ)
			continue
		}
		switch o.Mode {
		case ObjectsSingle, ObjectsPerRow:
		case ObjectsPerColumns:
			if o.Max < 1 || o.Max > 20 {
				add("%s.max: entre 1 e 20 no modo \"columns\"", base)
			}
		default:
			add("%s.mode: use %q, %q ou %q", base, ObjectsSingle, ObjectsPerRow, ObjectsPerColumns)
		}
		if len(o.Fields) == 0 {
			add("%s.fields: mapeie pelo menos um campo", base)
		}
		if o.Key != "" {
			if _, mapped := o.Fields[o.Key]; !mapped {
				add("%s.key: %q não está em fields (o campo que diz que o objeto existe tem que vir do arquivo)", base, o.Key)
			}
		}
		for name, rule := range o.Fields {
			f, ok := t.Field(name)
			if !ok {
				add("%s.fields.%s: %q não tem este campo (válidos: %s)", base, name, typ, strings.Join(fieldKeys(t.Fields), ", "))
				continue
			}
			if o.Mode == ObjectsPerColumns && rule.Column+rule.Template != "" && !strings.Contains(rule.Column+rule.Template, "{n}") {
				add("%s.fields.%s.column: no modo \"columns\" use \"{n}\" no nome (ex: \"Placa{n}\")", base, name)
			}
			validateRule(add, base+".fields."+name, f, rule)
		}
	}
}

func validateRule(add func(string, ...any), path string, f *annotation.Field, r FieldRule) {
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
	if r.Split != "" && f.Type != annotation.TypeMulti {
		add("%s.split: só faz sentido em campo de vários valores (multi), como conditions", path)
	}

	if f.Type == annotation.TypeDateTime {
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
		if len(r.Map) > 0 {
			add("%s.map: não se usa em data e hora (use date_format)", path)
		}
		return
	}
	if r.DateFormat != "" || r.Timezone != "" {
		add("%s: date_format/timezone só valem para campos de data e hora", path)
	}

	// Os valores de destino do Map (e o valor fixo) têm que ser valores
	// aceitos pelo campo: uma opção da lista, um número, true/false...
	froms := make([]string, 0, len(r.Map))
	for from := range r.Map {
		froms = append(froms, from)
	}
	sort.Strings(froms)
	for _, from := range froms {
		if err := checkTarget(f, r.Map[from]); err != nil {
			add("%s.map[%q]: %v", path, from, err)
		}
	}
	if r.Value != "" && r.Map == nil {
		if err := checkTarget(f, r.Value); err != nil {
			add("%s.value: %v", path, err)
		}
	}
}

// checkTarget: o texto é um valor válido do campo (já traduzido)?
func checkTarget(f *annotation.Field, s string) error {
	v, err := f.Typed(s)
	if err != nil {
		return err
	}
	if f.Type == annotation.TypeMulti {
		v = []string{s}
	}
	if _, err := f.Clean(v); err != nil {
		return err
	}
	return nil
}

func fieldKeys(fs []annotation.Field) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.Key
	}
	return out
}

func listObjects(ap *annotation.Profile) []string {
	var out []string
	for _, o := range ap.Objects {
		if !o.Box {
			out = append(out, o.Key)
		}
	}
	if len(out) == 0 {
		return []string{"nenhum sem caixa"}
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
