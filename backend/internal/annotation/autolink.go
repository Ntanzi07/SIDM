package annotation

// A ligação automática no import das caixas: a caixa motorcycle já é um
// veículo, então não precisa ligar imagem por imagem. As regras:
//
//  1. Objeto que já aponta uma caixa desta tarefa: acha a caixa de novo (a
//     mesma label, a mais parecida, IoU >= 0,5) e atualiza a posição.
//  2. Sobrou exatamente uma caixa e um objeto sem caixa do tipo, e o que a
//     label diz não contradiz o objeto: liga os dois.
//  3. Nenhum objeto do tipo sem caixa nem ligado a outra tarefa, e a imagem
//     ainda não foi verificada:
//     cada caixa vira um objeto (o que a label diz, os atributos que são um
//     campo e os valores iniciais), até o máximo do tipo.
//  4. O resto fica solto (mais de um par possível, verificada, máximo):
//     liga na tela. Numa imagem verificada só se liga caixa (passos 1 e 2,
//     sem mudar valor nenhum): o nome v2 dela não muda.

import (
	"maps"
	"reflect"
)

// SeenBox é uma caixa da imagem numa tarefa, como o import a conhece.
type SeenBox struct {
	Label      string
	X, Y, W, H float64
	Attributes map[string]any
}

// LinkResult conta o que a ligação fez numa imagem (ou somado no import).
type LinkResult struct {
	Linked  int `json:"linked"`  // caixas ligadas a um objeto que já existia
	Created int `json:"created"` // objetos novos, um por caixa
	Moved   int `json:"moved"`   // ligações que já existiam e a caixa mexeu
	Loose   int `json:"loose"`   // caixas que podiam ser um objeto e ficaram soltas
	Images  int `json:"images"`  // imagens que mudaram
	Busy    int `json:"busy"`    // imagens puladas: alguém está anotando
}

// Add soma outro resultado.
func (r *LinkResult) Add(o LinkResult) {
	r.Linked += o.Linked
	r.Created += o.Created
	r.Moved += o.Moved
	r.Loose += o.Loose
	r.Images += o.Images
	r.Busy += o.Busy
}

// Changed: algum objeto mudou.
func (r LinkResult) Changed() bool { return r.Linked+r.Created+r.Moved > 0 }

func iou(a BoxRef, b SeenBox) float64 {
	ix := max(0, min(a.X+a.W, b.X+b.W)-max(a.X, b.X))
	iy := max(0, min(a.Y+a.H, b.Y+b.H)-max(a.Y, b.Y))
	inter := ix * iy
	union := a.W*a.H + b.W*b.H - inter
	if union <= 0 {
		return 0
	}
	return inter / union
}

func refOf(task string, b SeenBox) *BoxRef {
	return &BoxRef{Task: task, Label: b.Label, X: b.X, Y: b.Y, W: b.W, H: b.H}
}

// LinkBoxes liga as caixas de uma tarefa aos objetos (ver as regras no topo).
// create: pode criar objetos e preencher campos (imagem não verificada).
// Devolve os objetos (uma cópia; os de entrada não mudam).
func (p *Profile) LinkBoxes(objects []Object, task string, boxes []SeenBox, create bool) ([]Object, LinkResult) {
	var res LinkResult
	out := make([]Object, len(objects))
	for i, o := range objects {
		out[i] = Object{Type: o.Type, Fields: maps.Clone(o.Fields)}
		if o.Box != nil {
			b := *o.Box
			out[i].Box = &b
		}
	}
	used := make([]bool, len(boxes))

	// 1. as ligações que já existem nesta tarefa
	for i := range out {
		ref := out[i].Box
		if ref == nil || ref.Task != task {
			continue
		}
		best, score := -1, 0.5
		for j, b := range boxes {
			if used[j] || b.Label != ref.Label {
				continue
			}
			if s := iou(*ref, b); s >= score {
				best, score = j, s
			}
		}
		if best < 0 {
			continue // a caixa sumiu: a tela mostra "não achada"
		}
		used[best] = true
		if now := refOf(task, boxes[best]); *now != *ref {
			out[i].Box = now
			res.Moved++
		}
	}

	for ti := range p.Objects {
		t := &p.Objects[ti]
		if t.Box || len(t.BoxLabels) == 0 {
			continue
		}
		var cands []int
		for j, b := range boxes {
			if _, ok := t.BoxLabel(b.Label); ok && !used[j] {
				cands = append(cands, j)
			}
		}
		if len(cands) == 0 {
			continue
		}
		var free []int
		count, elsewhere := 0, 0
		for i, o := range out {
			if o.Type != t.Key {
				continue
			}
			count++
			switch {
			case o.Box == nil:
				free = append(free, i)
			case o.Box.Task != task:
				elsewhere++ // a mesma moto pode estar em duas tarefas
			}
		}

		switch {
		// 2. um par só: liga
		case len(free) == 1 && len(cands) == 1:
			o, b := &out[free[0]], boxes[cands[0]]
			fields, ok := t.boxFields(b, o.Fields, create, false)
			if !ok {
				res.Loose++
				continue
			}
			o.Fields, o.Box = fields, refOf(task, b)
			used[cands[0]] = true
			res.Linked++

		// 3. nenhum objeto do tipo sem caixa (nem ligado a outra tarefa: seria
		// a mesma moto de novo): cada caixa vira um
		case len(free) == 0 && elsewhere == 0 && create:
			for _, j := range cands {
				if t.Max > 0 && count >= t.Max {
					res.Loose++
					continue
				}
				fields, _ := t.boxFields(boxes[j], t.defaults(), true, true)
				out = append(out, Object{Type: t.Key, Fields: fields, Box: refOf(task, boxes[j])})
				used[j] = true
				count++
				res.Created++
			}

		// 4. ambíguo ou verificada: fica para a tela
		default:
			res.Loose += len(cands)
		}
	}
	return out, res
}

// defaults: os valores iniciais dos campos do tipo (como a tela faz num
// objeto novo).
func (t *ObjectType) defaults() map[string]any {
	out := map[string]any{}
	for _, f := range t.Fields {
		if f.Default == nil || f.Type == TypeRef {
			continue
		}
		if v, err := f.Clean(f.Default); err == nil && v != nil {
			out[f.Key] = v
		}
	}
	return out
}

func empty(v any) bool {
	if v == nil || v == "" {
		return true
	}
	rv := reflect.ValueOf(v)
	return (rv.Kind() == reflect.Slice || rv.Kind() == reflect.Map) && rv.Len() == 0
}

// boxFields: os campos do objeto ligado à caixa b. O que a label diz vale
// (ok=false se o objeto já tem outro valor: a caixa diz moto e ele é carro).
// Com fill, o que a label diz entra nos vazios, e os atributos que são um
// campo também (nos vazios; num objeto novo, fresh, por cima do valor
// inicial); sem fill, nada
// muda: ok=false se a label diria algo num campo vazio.
func (t *ObjectType) boxFields(b SeenBox, fields map[string]any, fill, fresh bool) (map[string]any, bool) {
	l, _ := t.BoxLabel(b.Label)
	out := maps.Clone(fields)
	if out == nil {
		out = map[string]any{}
	}
	for key, raw := range l.Fields {
		f, ok := t.Field(key)
		if !ok {
			continue
		}
		want, err := f.Clean(raw)
		if err != nil || want == nil {
			continue
		}
		have, has := out[key]
		switch {
		case fresh: // objeto novo: a label vale mais que o valor inicial
			out[key] = want
		case has && !empty(have) && !reflect.DeepEqual(have, want):
			return nil, false
		case !has || empty(have):
			if !fill {
				return nil, false
			}
			out[key] = want
		}
	}
	if !fill {
		return out, true
	}
	for attr, key := range l.Attributes {
		f, ok := t.Field(key)
		raw, has := b.Attributes[attr]
		if !ok || !has || empty(raw) {
			continue
		}
		v, err := f.Clean(raw)
		if err != nil || v == nil {
			continue // valor do CVAT que o campo não aceita: fica vazio
		}
		if cur, set := fields[key]; !set || empty(cur) || fresh {
			out[key] = v
		}
	}
	return out, true
}
