package annotation

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Values é a anotação de uma imagem: os campos e os objetos sem caixa.
// (Os objetos com caixa ficam nas caixas, no módulo boxes.)
type Values struct {
	Fields  map[string]any `json:"fields"`
	Objects []Object       `json:"objects"`
}

// Check confere os valores contra o perfil e devolve uma cópia LIMPA
// (texto sem espaços, maiúsculas onde o campo pede, data em UTC, números
// como float64, listas como []string). Campo vazio é o mesmo que ausente e
// some da cópia.
//
// complete=false é o rascunho: só confere o que veio preenchido.
// complete=true é a verificação: também cobra os obrigatórios e o mínimo de
// objetos de cada tipo.
//
// Os nomes de cadastro (campos ref) só têm o formato conferido aqui: se o
// equipamento existe é com o catálogo.
func (p *Profile) Check(v Values, complete bool) (Values, error) {
	var ps problems
	out := Values{Fields: checkFields(&ps, "fields", p.Fields, v.Fields, complete), Objects: []Object{}}

	types := map[string]*ObjectType{}
	for i := range p.Objects {
		types[p.Objects[i].Key] = &p.Objects[i]
	}
	count := map[string]int{}
	for i, o := range v.Objects {
		path := fmt.Sprintf("objects[%d]", i)
		t, ok := types[o.Type]
		switch {
		case !ok:
			ps.add(path+".type", "tipo de objeto %q não existe no perfil %s", o.Type, p.Name)
			continue
		case t.Box:
			ps.add(path+".type", "%q tem caixa: os objetos dele vêm das caixas (CVAT), não da lista da imagem", o.Type)
			continue
		}
		count[o.Type]++
		fields := checkFields(&ps, path, t.Fields, implied(t, o), complete)
		box := checkLink(&ps, path, t, o, fields)
		if box != nil {
			for _, prev := range out.Objects {
				if prev.Box != nil && sameBox(prev.Box, box) {
					ps.add(path+".box", "esta caixa já é de outro objeto")
					box = nil
					break
				}
			}
		}
		out.Objects = append(out.Objects, Object{Type: o.Type, Fields: fields, Box: box})
	}
	for _, t := range p.Objects {
		if t.Box {
			continue
		}
		n := count[t.Key]
		if t.Max > 0 && n > t.Max {
			ps.add("objects", "no máximo %d de %q por imagem (vieram %d)", t.Max, t.Key, n)
		}
		if complete && n < t.Min {
			ps.add("objects", "pelo menos %d de %q por imagem para verificar (vieram %d)", t.Min, t.Key, n)
		}
	}
	if len(ps) > 0 {
		return Values{}, &InvalidError{Msg: summary(ps), Problems: ps}
	}
	return out, nil
}

func checkFields(ps *problems, base string, defs []Field, in map[string]any, complete bool) map[string]any {
	out := map[string]any{}
	known := map[string]bool{}
	for _, f := range defs {
		known[f.Key] = true
		path := base + "." + f.Key
		v, err := f.Clean(in[f.Key])
		switch {
		case err != nil:
			ps.add(path, "%s", err)
		case v != nil:
			out[f.Key] = v
		case complete && f.Required:
			ps.add(path, "obrigatório para verificar")
		}
	}
	for k := range in {
		if !known[k] {
			ps.add(base+"."+k, "campo %q não existe no perfil", k)
		}
	}
	return out
}

// Clean confere e limpa UM valor. Devolve nil quando está vazio.
func (f *Field) Clean(v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	switch f.Type {
	case TypeChoice:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("esperado um texto")
		}
		if s = strings.TrimSpace(s); s == "" {
			return nil, nil
		}
		// o tamanho medido do arquivo vale mesmo fora das opções (FillFromImage)
		if !f.hasOption(s) && !(f.FromImage == FromImageSize && sizeRe.MatchString(s)) {
			return nil, fmt.Errorf("%q não é uma opção (%s)", s, f.optionList())
		}
		return s, nil

	case TypeMulti:
		list, ok := stringList(v)
		if !ok {
			return nil, fmt.Errorf("esperada uma lista de textos")
		}
		out, seen := []string{}, map[string]bool{}
		for _, s := range list {
			if s = strings.TrimSpace(s); s == "" || seen[s] {
				continue // repetido não é erro: só entra uma vez
			}
			if !f.hasOption(s) {
				return nil, fmt.Errorf("%q não é uma opção (%s)", s, f.optionList())
			}
			seen[s] = true
			out = append(out, s)
		}
		out = f.dropExclusive(out)
		if len(out) == 0 {
			return nil, nil
		}
		return out, nil

	case TypeText, TypeRef:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("esperado um texto")
		}
		if s = strings.TrimSpace(s); s == "" {
			return nil, nil
		}
		if f.Upper {
			s = strings.ToUpper(s)
		}
		if f.MaxLen > 0 && len([]rune(s)) > f.MaxLen {
			return nil, fmt.Errorf("no máximo %d caracteres", f.MaxLen)
		}
		if f.Pattern != "" {
			// Inteiro: o padrão vale para o valor todo, não para um pedaço.
			if re, err := regexp.Compile(`^(?:` + f.Pattern + `)$`); err != nil || !re.MatchString(s) {
				return nil, fmt.Errorf("%q fora do formato (%s)", s, f.Pattern)
			}
		}
		return s, nil

	case TypeNumber:
		n, ok := number(v)
		if !ok {
			return nil, fmt.Errorf("esperado um número")
		}
		if f.Integer && n != math.Trunc(n) {
			return nil, fmt.Errorf("esperado um número inteiro")
		}
		if f.Min != nil && n < *f.Min {
			return nil, fmt.Errorf("menor que o mínimo (%v)", *f.Min)
		}
		if f.Max != nil && n > *f.Max {
			return nil, fmt.Errorf("maior que o máximo (%v)", *f.Max)
		}
		return n, nil

	case TypeBool:
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("esperado true ou false")
		}
		return b, nil

	case TypeDateTime:
		switch t := v.(type) {
		case time.Time:
			return t.UTC(), nil
		case interface{ Time() time.Time }: // a data do Mongo (bson.DateTime)
			return t.Time().UTC(), nil
		case string:
			if t = strings.TrimSpace(t); t == "" {
				return nil, nil
			}
			parsed, err := time.Parse(time.RFC3339, t)
			if err != nil {
				return nil, fmt.Errorf("data inválida: use RFC 3339, ex.: 2026-09-03T11:25:23Z")
			}
			return parsed.UTC(), nil
		}
		return nil, fmt.Errorf("esperada uma data")
	}
	return nil, fmt.Errorf("tipo de campo %q desconhecido", f.Type)
}

func (f *Field) hasOption(v string) bool {
	for _, o := range f.Options {
		if o.Value == v {
			return true
		}
	}
	return false
}

// dropExclusive: a opção exclusiva ("Sem avarias") sai se vier junto de
// outra (o problema marcado ganha); sozinha, fica.
func (f *Field) dropExclusive(list []string) []string {
	exclusive := map[string]bool{}
	for _, o := range f.Options {
		if o.Exclusive {
			exclusive[o.Value] = true
		}
	}
	if len(exclusive) == 0 {
		return list
	}
	kept := []string{}
	for _, v := range list {
		if !exclusive[v] {
			kept = append(kept, v)
		}
	}
	if len(kept) == 0 {
		return list // só a exclusiva
	}
	return kept
}

func (f *Field) optionList() string {
	vs := make([]string, len(f.Options))
	for i, o := range f.Options {
		vs[i] = o.Value
	}
	if len(vs) > 8 {
		return strings.Join(vs[:8], ", ") + ", ..."
	}
	return strings.Join(vs, ", ")
}

// stringList aceita qualquer lista de textos: []string, []any (JSON) ou
// bson.A (Mongo, que é um tipo com nome próprio: por isso a reflexão).
func stringList(v any) ([]string, bool) {
	if l, ok := v.([]string); ok {
		return l, true
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return nil, false
	}
	out := make([]string, rv.Len())
	for i := range out {
		s, ok := rv.Index(i).Interface().(string)
		if !ok {
			return nil, false
		}
		out[i] = s
	}
	return out, true
}

// number aceita os tipos numéricos do JSON e do Mongo.
func number(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}

// Fit encaixa no perfil uma anotação que veio de fora (import de CSV, INI,
// nome v2 do arquivo): fica só o que o perfil tem e é válido; o resto sai e
// vira aviso. Ao contrário do Check, nunca falha: um valor ruim não pode
// derrubar o import das outras imagens.
func (p *Profile) Fit(v Values) (Values, []string) {
	out := Values{Fields: map[string]any{}, Objects: []Object{}}
	var warnings []string
	out.Fields, warnings = fitFields("", "o perfil "+p.Name, p.Fields, v.Fields, false)
	for _, f := range p.Fields {
		if f.FromImage != "" {
			delete(out.Fields, f.Key) // quem dita é o arquivo (FillFromImage)
		}
	}
	for _, o := range v.Objects {
		t, ok := p.Object(o.Type)
		if !ok || t.Box {
			warnings = append(warnings, fmt.Sprintf("objeto %q: o perfil %s não tem este tipo (sem caixa)", o.Type, p.Name))
			continue
		}
		fields, w := fitFields(o.Type+".", "o perfil "+p.Name, t.Fields, o.Fields, false)
		warnings = append(warnings, w...)
		out.Objects = append(out.Objects, Object{Type: o.Type, Fields: fields})
	}
	return out, warnings
}

// FitFields encaixa nos campos do tipo os valores de UM objeto que vieram de
// fora (os atributos de uma caixa do CVAT): o nome casa pela chave do campo
// ou, se não casar, pelo nome na tela; um texto vira o tipo do campo
// (FromText). O que não serve sai e vira aviso ("color: ...").
func (t *ObjectType) FitFields(in map[string]any) (map[string]any, []string) {
	return fitFields("", fmt.Sprintf("%q", t.Key), t.Fields, in, true)
}

// CheckFields confere os valores de UM objeto do tipo, como o Check faz com a
// imagem (complete=true cobra os obrigatórios). base é o começo do caminho
// dos problemas (ex.: "boxes[2].attributes").
func (t *ObjectType) CheckFields(base string, in map[string]any, complete bool) (map[string]any, error) {
	var ps problems
	out := checkFields(&ps, base, t.Fields, in, complete)
	if len(ps) > 0 {
		return nil, &InvalidError{Msg: summary(ps), Problems: ps}
	}
	return out, nil
}

// Missing: os campos obrigatórios do tipo que estão vazios nos valores.
func (t *ObjectType) Missing(values map[string]any) []string {
	var out []string
	for _, f := range t.Fields {
		if f.Required {
			if v, err := f.Clean(values[f.Key]); err != nil || v == nil {
				out = append(out, f.Key)
			}
		}
	}
	return out
}

// BoxType devolve o tipo de objeto COM caixa dessa label do CVAT (a label é
// a chave do tipo).
func (p *Profile) BoxType(label string) (*ObjectType, bool) {
	t, ok := p.Object(label)
	if !ok || !t.Box {
		return nil, false
	}
	return t, true
}

// fitFields é o laço do Fit e do FitFields. loose: o nome também casa com
// o nome do campo na tela, e um texto passa pelo FromText.
func fitFields(where, owner string, defs []Field, in map[string]any, loose bool) (map[string]any, []string) {
	clean := map[string]any{}
	var warnings []string
	find := func(k string) *Field {
		for i := range defs {
			if defs[i].Key == k {
				return &defs[i]
			}
		}
		if loose {
			for i := range defs {
				if defs[i].Label != "" && strings.EqualFold(strings.TrimSpace(defs[i].Label), strings.TrimSpace(k)) {
					return &defs[i]
				}
			}
		}
		return nil
	}
	for _, k := range sortedKeys(in) {
		f := find(k)
		if f == nil {
			if in[k] != nil && in[k] != "" {
				warnings = append(warnings, fmt.Sprintf("%s%s: %s não tem este campo", where, k, owner))
			}
			continue
		}
		v := in[k]
		if s, ok := v.(string); ok && loose && strings.TrimSpace(s) != "" {
			conv, err := f.FromText(strings.TrimSpace(s))
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s%s: %v", where, f.Key, err))
				continue
			}
			v = conv
		}
		val, err := f.Clean(v)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s%s: %v", where, f.Key, err))
			continue
		}
		if val != nil {
			clean[f.Key] = val
		}
	}
	return clean, warnings
}

// OptionOf acha a opção do campo pelo valor, pelo código ou pelo nome da
// tela, sem diferenciar maiúscula.
func (f *Field) OptionOf(s string) (string, bool) {
	for _, o := range f.Options {
		if strings.EqualFold(o.Value, s) || (o.Code != "" && strings.EqualFold(o.Code, s)) {
			return o.Value, true
		}
	}
	for _, o := range f.Options {
		if o.Label != "" && strings.EqualFold(o.Label, s) {
			return o.Value, true
		}
	}
	return "", false
}

// Typed converte um texto no tipo do campo quando é número ou sim/não
// ("2,5" -> 2.5; "sim" -> true). Os outros tipos ficam texto.
func (f *Field) Typed(s string) (any, error) {
	switch f.Type {
	case TypeNumber:
		n, err := strconv.ParseFloat(strings.Replace(s, ",", ".", 1), 64)
		if err != nil {
			return nil, fmt.Errorf("%q não é um número", s)
		}
		return n, nil
	case TypeBool:
		switch strings.ToLower(s) {
		case "true", "1", "s", "sim", "y", "yes", "verdadeiro":
			return true, nil
		case "false", "0", "n", "nao", "não", "no", "falso":
			return false, nil
		}
		return nil, fmt.Errorf("%q não é sim/não (use true ou false)", s)
	}
	return s, nil
}

// FromText converte um texto de fora (CSV, atributo do CVAT) no valor do
// campo: numa lista, a opção pelo valor, código ou nome; número e sim/não
// pelo Typed; o resto, o próprio texto (o Clean confere).
func (f *Field) FromText(s string) (any, error) {
	switch f.Type {
	case TypeChoice, TypeMulti:
		v, ok := f.OptionOf(s)
		if !ok {
			return nil, fmt.Errorf("%q não é uma opção (%s)", s, f.optionList())
		}
		if f.Type == TypeMulti {
			return []string{v}, nil
		}
		return v, nil
	}
	return f.Typed(s)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
