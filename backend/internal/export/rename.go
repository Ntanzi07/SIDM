package export

import (
	"path"
	"regexp"
	"strings"
)

// Nome dos arquivos de saída. Profile.FileName é um modelo com campos entre
// chaves; vazio = o nome guardado no SIDM (o v2). Exemplo, o padrão antigo:
//
//	{sha1}{direction}{resolution}{capture_angle}{day_time}{vehicle_type}{image_range}{plate_type}{plate_category}{weather}{condition}{plate}
//	-> 02d9314a6112e8b95f97415dc96e5bf0682f3a3fF01LDBEB1S01KOY7386.jpg
//
// Os campos são os do perfil de anotação (ver MetaFields). Um campo de lista
// com código sai com o código (no perfil veiculos, o do nome v2: direção
// "F", resolução "01", …). Campo sem valor vira "_" repetido na largura dele
// (a do maior código; numa data, 16; num texto, o max_len), e o nome
// continua com tamanho fixo. Os de objeto vêm do primeiro objeto do tipo; de
// uma lista, o primeiro valor. A extensão é a do arquivo guardado.

// TemplateLegacy é o padrão antigo: hash + características + placa.
const TemplateLegacy = "{sha1}{direction}{resolution}{capture_angle}{day_time}{vehicle_type}{image_range}{plate_type}{plate_category}{weather}{condition}{plate}"

// MissingChar preenche um campo sem valor.
const MissingChar = "_"

var tokenRe = regexp.MustCompile(`\{([^{}]*)\}`)

// FileNameOf monta o nome de saída da imagem (com a extensão de ext), depois
// do Bind. Meta tem os VALORES guardados (ex.: "rear"); um campo de lista
// com código sai com o código (ex.: "T").
func (p *Profile) FileNameOf(m Meta, ext string) string {
	if p.FileName == "" {
		return m["name"] + ext
	}
	name := tokenRe.ReplaceAllStringFunc(p.FileName, func(tok string) string {
		field := tok[1 : len(tok)-1]
		spec := p.specs[field]
		value := m[field]
		if value != "" && spec.codes != nil {
			if code, ok := spec.codes[value]; ok {
				value = code
			}
		}
		if field == "plate" || field == "vehicle_plate" {
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
		if spec, ok := p.specs[field]; !ok || !spec.Name {
			add("file_name: campo desconhecido {%s} no perfil de anotação %s (use: %s)", field, p.annotationName(),
				strings.Join(keysWhere(p.specs, func(m MetaField) bool { return m.Name }), ", "))
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
