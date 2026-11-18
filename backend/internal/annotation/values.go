package annotation

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
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
		out.Objects = append(out.Objects, Object{Type: o.Type, Fields: checkFields(&ps, path, t.Fields, o.Fields, complete)})
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
		v, err := f.clean(in[f.Key])
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

// clean confere e limpa UM valor. Devolve nil quando está vazio.
func (f *Field) clean(v any) (any, error) {
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
		if !f.hasOption(s) {
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
