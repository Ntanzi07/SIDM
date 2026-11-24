package annotation

// A ligação entre um objeto da imagem (ex.: o Veículo 1) e a caixa que ele
// é (a caixa motorcycle). O tipo de objeto diz quais labels de caixa o
// representam (BoxLabels) e o que cada label já diz dele (motorcycle →
// vehicle_type = moto).

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// validateLinks confere as BoxLabels no perfil já resolvido (os campos
// podem ser herdados).
func (p *Profile) validateLinks() error {
	var ps problems
	for i, t := range p.Objects {
		path := fmt.Sprintf("objects[%d].box_labels", i)
		if len(t.BoxLabels) > 0 && t.Box {
			ps.add(path, "o tipo %q já é uma caixa: as labels de caixa são para os tipos de lista (ex.: veículo)", t.Key)
			continue
		}
		seen := map[string]bool{}
		for j, l := range t.BoxLabels {
			lp := fmt.Sprintf("%s[%d]", path, j)
			switch {
			case l.Label == "" || len(l.Label) > 64:
				ps.add(lp+".label", "a label da caixa (ex.: motorcycle), até 64 caracteres")
			case seen[l.Label]:
				ps.add(lp+".label", "label %q repetida", l.Label)
			}
			seen[l.Label] = true
			for _, key := range sortedKeys(l.Fields) {
				f, ok := t.Field(key)
				if !ok {
					ps.add(lp+".fields."+key, "o tipo %q não tem o campo %q", t.Key, key)
					continue
				}
				if _, err := f.Clean(l.Fields[key]); err != nil {
					ps.add(lp+".fields."+key, "%v", err)
				}
			}
		}
	}
	if len(ps) > 0 {
		return &InvalidError{Msg: summary(ps), Problems: ps}
	}
	return nil
}

// BoxLabel devolve a label de caixa do tipo (se ela representa o tipo).
func (t *ObjectType) BoxLabel(label string) (*BoxLabel, bool) {
	i := slices.IndexFunc(t.BoxLabels, func(l BoxLabel) bool { return l.Label == label })
	if i < 0 {
		return nil, false
	}
	return &t.BoxLabels[i], true
}

// implied: os campos do objeto com o que a caixa dele já diz, onde estão
// vazios (a caixa motorcycle preenche o tipo de veículo).
func implied(t *ObjectType, o Object) map[string]any {
	if o.Box == nil {
		return o.Fields
	}
	l, ok := t.BoxLabel(strings.TrimSpace(o.Box.Label))
	if !ok || len(l.Fields) == 0 {
		return o.Fields
	}
	out := make(map[string]any, len(o.Fields)+len(l.Fields))
	for k, v := range o.Fields {
		out[k] = v
	}
	for k, v := range l.Fields {
		if cur, ok := out[k]; !ok || cur == nil || cur == "" {
			out[k] = v
		}
	}
	return out
}

// checkLink confere a caixa de um objeto: a label tem que representar o
// tipo, e o que ela diz vale (campo vazio é preenchido; diferente é erro:
// a caixa diz moto e o tipo está carro).
func checkLink(ps *problems, path string, t *ObjectType, o Object, fields map[string]any) *BoxRef {
	if o.Box == nil {
		return nil
	}
	ref := *o.Box
	ref.Task, ref.Label = strings.TrimSpace(ref.Task), strings.TrimSpace(ref.Label)
	l, ok := t.BoxLabel(ref.Label)
	switch {
	case ref.Task == "" || ref.Label == "":
		ps.add(path+".box", "a caixa precisa da tarefa e da label")
		return nil
	case !ok:
		ps.add(path+".box", "uma caixa %q não representa %q (as que representam: %s)", ref.Label, t.Key, labelList(t))
		return nil
	}
	for _, key := range sortedKeys(l.Fields) {
		f, _ := t.Field(key)
		want, err := f.Clean(l.Fields[key])
		if err != nil || want == nil {
			continue
		}
		have, ok := fields[key]
		switch {
		case !ok:
			fields[key] = want // a caixa já diz: preenche
		case !reflect.DeepEqual(have, want):
			ps.add(fmt.Sprintf("%s.%s", path, key), "a caixa é %q, então %s é %v (está %v)", ref.Label, fieldName(f), want, have)
		}
	}
	return &ref
}

func labelList(t *ObjectType) string {
	if len(t.BoxLabels) == 0 {
		return "nenhuma"
	}
	out := make([]string, len(t.BoxLabels))
	for i, l := range t.BoxLabels {
		out[i] = l.Label
	}
	return strings.Join(out, ", ")
}

func fieldName(f *Field) string {
	if f.Label != "" {
		return strings.ToLower(f.Label)
	}
	return f.Key
}

// sameBox: duas referências para a mesma caixa.
func sameBox(a, b *BoxRef) bool {
	return a.Task == b.Task && a.Label == b.Label && a.X == b.X && a.Y == b.Y && a.W == b.W && a.H == b.H
}
