package export

import (
	"sort"

	"SIDM/internal/annotation"
)

// MetaField é um metadado da imagem que pode virar nível de pasta ou parte
// do nome do arquivo de saída. Os campos vêm do perfil de anotação (ver
// MetaFields); a chave é a mesma que imageMeta (no módulo exports) usa.
type MetaField struct {
	Key   string `json:"field"`
	Label string `json:"label"`
	// Group: "file" (sha1, nome, pacote, divisão), "image" (um campo da
	// imagem) ou o tipo do objeto (ex.: "vehicle").
	Group string `json:"group"`
	// Values: valor guardado -> nome padrão da pasta. nil = campo aberto
	// (equipamento, número, texto: a pasta é o próprio valor).
	Values map[string]string `json:"values"`
	// Labels: valor guardado -> nome na tela (para a tabela de nomes).
	Labels map[string]string `json:"labels,omitempty"`
	Folder bool              `json:"folder"` // pode ser nível de pasta
	Name   bool              `json:"name"`   // pode ir no modelo de nome
	// Alias: nome antigo ou atalho de outro campo (ex.: "plate" para
	// "vehicle_plate"). Aceito, mas a tela não oferece.
	Alias bool `json:"alias,omitempty"`

	codes map[string]string // valor -> código (no nome do arquivo)
	width int               // largura fixa no nome (0 = variável)
}

// Os metadados que não vêm do perfil de anotação.
var fileFields = []MetaField{
	{Key: "sha1", Label: "Hash (sha1)", Group: "file", Name: true, width: 40},
	{Key: "name", Label: "Nome do SIDM (v2)", Group: "file", Name: true},
	{Key: "original", Label: "Nome original", Group: "file", Name: true},
	{Key: "package", Label: "Pacote", Group: "file", Name: true, Folder: true},
	{Key: "split", Label: "Divisão (train/valid/test)", Group: "file", Folder: true,
		Values: map[string]string{SplitTrain: SplitTrain, SplitValid: SplitValid, SplitTest: SplitTest}},
}

// dateWidth: uma data no nome sai como 20260903T112523Z.
const dateWidth = 16

// MetaFields lista os metadados que um perfil de anotação dá às imagens:
// os de arquivo (sha1, pacote, divisão...), cada campo da imagem, e cada
// campo de cada tipo de objeto sem caixa como "<tipo>_<campo>" (do primeiro
// objeto do tipo). O campo de objeto também vale sem o tipo ("plate") se a
// imagem não tem um campo com esse nome (do primeiro objeto da imagem).
//
// No perfil veiculos isso dá os nomes de sempre: direction, day_time,
// vehicle_type, vehicle_color, plate, plate_type...
func MetaFields(ap *annotation.Profile) []MetaField {
	out := append([]MetaField(nil), fileFields...)
	taken := map[string]bool{}
	for _, f := range out {
		taken[f.Key] = true
	}
	add := func(m MetaField) {
		if !taken[m.Key] {
			taken[m.Key] = true
			out = append(out, m)
		}
	}

	for i := range ap.Fields {
		f := &ap.Fields[i]
		add(metaField(f.Key, labelOf(f.Label, f.Key), "image", f))
		switch {
		case f.Key == "conditions" && f.Type == annotation.TypeMulti:
			m := metaField("condition", "Condição (a primeira)", "image", f)
			m.Alias = true
			add(m)
		case f.Key == "captured_at" && f.Type == annotation.TypeDateTime:
			// só a data, AAAAMMDD
			add(MetaField{Key: "date", Label: "Data", Group: "image", Name: true, width: 8})
		}
	}

	var aliases []MetaField
	for _, t := range ap.Objects {
		if t.Box {
			continue // os objetos com caixa ficam nas caixas, não na imagem
		}
		for i := range t.Fields {
			f := &t.Fields[i]
			label := labelOf(t.Label, t.Key) + ": " + labelOf(f.Label, f.Key)
			add(metaField(t.Key+"_"+f.Key, label, t.Key, f))
			alias := metaField(f.Key, label, t.Key, f)
			alias.Alias = true
			aliases = append(aliases, alias)
		}
	}
	for _, a := range aliases {
		add(a)
	}
	return out
}

func labelOf(label, key string) string {
	if label != "" {
		return label
	}
	return key
}

// metaField descreve um campo do perfil de anotação como metadado.
func metaField(key, label, group string, f *annotation.Field) MetaField {
	m := MetaField{Key: key, Label: label, Group: group, Name: true}
	switch f.Type {
	case annotation.TypeChoice, annotation.TypeMulti:
		m.Folder = true
		m.Values, m.Labels = map[string]string{}, map[string]string{}
		for _, o := range f.Options {
			m.Values[o.Value] = o.Value
			m.Labels[o.Value] = labelOf(o.Label, o.Value)
			if o.Code != "" {
				if m.codes == nil {
					m.codes = map[string]string{}
				}
				m.codes[o.Value] = o.Code
				m.width = max(m.width, len(o.Code))
			}
		}
		// A resolução vira "<código>-<valor>" ("01-640x480"): as pastas
		// ficam em ordem.
		if f.Key == "resolution" && m.codes != nil {
			for v, c := range m.codes {
				m.Values[v] = c + "-" + v
			}
		}
	case annotation.TypeBool:
		m.Folder = true
		m.Values = map[string]string{"true": "true", "false": "false"}
		m.Labels = map[string]string{"true": "Sim", "false": "Não"}
	case annotation.TypeRef, annotation.TypeNumber:
		m.Folder = true
	case annotation.TypeText:
		m.Folder = true // a pasta é o próprio texto (ex.: a marca)
		m.width = f.MaxLen
	case annotation.TypeDateTime:
		m.width = dateWidth
	}
	return m
}

// fieldSpecs indexa os metadados por chave.
func fieldSpecs(ap *annotation.Profile) map[string]MetaField {
	out := map[string]MetaField{}
	for _, m := range MetaFields(ap) {
		out[m.Key] = m
	}
	return out
}

// folderKeys / nameKeys: as chaves aceitas (sem os atalhos), para as
// mensagens de erro.
func keysWhere(specs map[string]MetaField, ok func(MetaField) bool) []string {
	var out []string
	for k, m := range specs {
		if ok(m) && !m.Alias {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
