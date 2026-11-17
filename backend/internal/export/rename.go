package export

import (
	"path"
	"regexp"
	"sort"
	"strings"

	"SIDM/internal/filename"
)

// Nome dos arquivos de saída. Profile.FileName é um modelo com campos entre
// chaves; vazio = o nome guardado no SIDM (o v2). Exemplo, o padrão antigo:
//
//	{sha1}{direction}{resolution}{capture_angle}{day_time}{vehicle_type}{image_range}{plate_type}{plate_category}{weather}{condition}{plate}
//	-> 02d9314a6112e8b95f97415dc96e5bf0682f3a3fF01LDBEB1S01KOY7386.jpg
//
// Os campos de código saem com o código do nome v2 (direção "F", resolução
// "01", …). Campo sem valor vira "_" repetido na largura dele (o nome
// continua com tamanho fixo). Os do veículo vêm do primeiro veículo; a
// condição é a primeira da imagem. A extensão é a do arquivo guardado.

// TemplateLegacy é o padrão antigo: hash + características + placa.
const TemplateLegacy = "{sha1}{direction}{resolution}{capture_angle}{day_time}{vehicle_type}{image_range}{plate_type}{plate_category}{weather}{condition}{plate}"

// MissingChar preenche um campo sem valor.
const MissingChar = "_"

type nameField struct {
	table string // tabela de códigos (filename.Codes); "" = valor direto
	width int    // largura fixa (0 = variável: sem valor fica vazio)
}

var nameFields = map[string]nameField{
	"sha1":           {width: 40},
	"name":           {}, // o nome guardado (v2), sem extensão
	"original":       {}, // o nome original do upload, sem extensão
	"package":        {},
	"equipment":      {},
	"camera":         {},
	"captured_at":    {width: 16}, // 20260903T112523Z
	"date":           {width: 8},  // 20260903
	"plate":          {width: 7},
	"direction":      {table: "direction", width: 1},
	"resolution":     {table: "resolution", width: 2},
	"capture_angle":  {table: "capture_angle", width: 1},
	"day_time":       {table: "day_time", width: 1},
	"vehicle_type":   {table: "vehicle_type", width: 1},
	"image_range":    {table: "image_range", width: 1},
	"plate_type":     {table: "plate_type", width: 1},
	"plate_category": {table: "plate_category", width: 1},
	"weather":        {table: "weather", width: 1},
	"condition":      {table: "conditions", width: 2},
	"vehicle_color":  {table: "vehicle_color", width: 3},
}

// NameFields: os campos aceitos no modelo, em ordem alfabética.
func NameFields() []string {
	out := make([]string, 0, len(nameFields))
	for f := range nameFields {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

// valor -> código, por tabela
var valueToCode = func() map[string]map[string]string {
	out := map[string]map[string]string{}
	for table, codes := range filename.Codes() {
		rev := make(map[string]string, len(codes))
		for code, value := range codes {
			rev[value] = code
		}
		out[table] = rev
	}
	return out
}()

var tokenRe = regexp.MustCompile(`\{([^{}]*)\}`)

// FileNameOf monta o nome de saída da imagem (com a extensão de ext). Meta
// tem os campos de nameFields com os VALORES guardados (ex.: "rear").
func (p *Profile) FileNameOf(m Meta, ext string) string {
	if p.FileName == "" {
		return m["name"] + ext
	}
	name := tokenRe.ReplaceAllStringFunc(p.FileName, func(tok string) string {
		field := tok[1 : len(tok)-1]
		spec := nameFields[field]
		value := m[field]
		if spec.table != "" && value != "" {
			value = valueToCode[spec.table][value]
		}
		if field == "plate" {
			value = strings.ToUpper(value)
		}
		if value == "" && spec.width > 0 {
			return strings.Repeat(MissingChar, spec.width)
		}
		return sanitizeFolder(value)
	})
	return name + ext
}

// validateFileName: só campos conhecidos, e o nome tem que ser único por
// imagem (tem {sha1} ou {name}, que começa pelo sha1).
func (p *Profile) validateFileName(add func(string, ...any)) {
	if p.FileName == "" {
		return
	}
	if len(p.FileName) > 200 {
		add("file_name: no máximo 200 caracteres")
	}
	unique := false
	for _, m := range tokenRe.FindAllStringSubmatch(p.FileName, -1) {
		field := m[1]
		if _, ok := nameFields[field]; !ok {
			add("file_name: campo desconhecido {%s} (use: %s)", field, strings.Join(NameFields(), ", "))
			continue
		}
		if field == "sha1" || field == "name" {
			unique = true
		}
	}
	literal := tokenRe.ReplaceAllString(p.FileName, "")
	if strings.ContainsAny(literal, `{}/\:*?"<>|`) {
		add(`file_name: o texto fixo não pode ter { } / \ : * ? " < > |`)
	}
	if !unique {
		add("file_name: use {sha1} (ou {name}) no modelo, para cada imagem ter um nome diferente")
	}
}

// NameMeta junta os dados da imagem que o modelo de nome usa.
func NameMeta(sha1, storedName, originalName string, m Meta) Meta {
	out := Meta{
		"sha1":     sha1,
		"name":     strings.TrimSuffix(storedName, path.Ext(storedName)),
		"original": strings.TrimSuffix(originalName, path.Ext(originalName)),
	}
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (p *Profile) normalizeFileName() {
	p.FileName = strings.TrimSpace(p.FileName)
}
