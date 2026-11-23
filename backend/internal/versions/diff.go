package versions

// O diff entre duas versões de um dataset.
//
// A lista compara só o sha1 e o hash de cada item (não precisa abrir a
// anotação de milhares de imagens): sha1 só na nova = adicionada; só na velha
// = removida; nas duas com hash diferente = alterada. O detalhe de UMA imagem
// (o que mudou: campos, objetos, caixas por tarefa) compara os dois itens
// inteiros, só quando a tela abre a imagem.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"

	"go.mongodb.org/mongo-driver/v2/bson"

	"SIDM/internal/annotation"
	"SIDM/internal/boxes"
	"SIDM/internal/platform"
)

// ItemRef: o mínimo de um item para a lista do diff.
type ItemRef struct {
	SHA1     string `bson:"sha1" json:"sha1"`
	Package  string `bson:"package" json:"package"`
	FileName string `bson:"file_name" json:"file_name"`
	Hash     string `bson:"hash" json:"-"`
}

// Diff: o que mudou da versão From para a To. From é nil quando a To é a
// primeira (tudo entra como adicionado).
type Diff struct {
	Dataset   string       `json:"dataset"`
	From      *VersionInfo `json:"from"`
	To        *VersionInfo `json:"to"`
	Added     []ItemRef    `json:"added"`
	Removed   []ItemRef    `json:"removed"`
	Changed   []ItemRef    `json:"changed"`
	Unchanged int          `json:"unchanged"`
}

// Os estados de uma imagem no diff.
const (
	StatusAdded     = "added"
	StatusRemoved   = "removed"
	StatusChanged   = "changed"
	StatusUnchanged = "unchanged"
)

// ItemDiff: o detalhe de uma imagem entre as duas versões. Só o que mudou.
type ItemDiff struct {
	SHA1     string `json:"sha1"`
	Package  string `json:"package"`
	FileName string `json:"file_name"`
	Status   string `json:"status"`
	// Fields e Objects: a chave no formato dos filtros da Pesquisa
	// ("fields.cor", "objects.veiculo.cor"; com mais de um objeto do mesmo
	// tipo, "objects.veiculo[2].cor"). nil = não informado.
	Fields  []Change     `json:"fields"`
	Objects []Change     `json:"objects"`
	Tasks   []TaskChange `json:"tasks"`
}

// Change: um valor que mudou.
type Change struct {
	Key  string `json:"key"`
	From any    `json:"from"`
	To   any    `json:"to"`
}

// TaskChange: as caixas de uma tarefa que mudaram. Labels traz só as labels
// cuja quantidade mudou; Edited = as caixas mudaram de outro jeito (posição,
// tamanho, atributos) mesmo com as mesmas quantidades.
type TaskChange struct {
	Task   string        `json:"task"`
	From   int           `json:"from"` // caixas na versão velha
	To     int           `json:"to"`
	Labels []LabelChange `json:"labels"`
	Edited bool          `json:"edited"`
}

type LabelChange struct {
	Label string `json:"label"`
	From  int    `json:"from"`
	To    int    `json:"to"`
}

// pair acha as duas versões. to = 0: a mais nova; from = 0: a anterior à to
// (nenhuma, se a to é a primeira).
func (s *Service) pair(ctx context.Context, name string, from, to int) (*Version, *Version, error) {
	if _, err := s.repo.FindDataset(ctx, name); err != nil {
		return nil, nil, err
	}
	var vt *Version
	var err error
	if to == 0 {
		if vt, err = s.repo.LastVersion(ctx, name); err != nil {
			return nil, nil, err
		}
		if vt == nil {
			return nil, nil, &InvalidError{fmt.Sprintf("o dataset %q ainda não tem versões", name)}
		}
	} else if vt, err = s.repo.FindVersion(ctx, name, to); err != nil {
		return nil, nil, err
	}
	if from == 0 {
		from = vt.Number - 1
		if from == 0 {
			return nil, vt, nil
		}
	}
	if from == vt.Number {
		return nil, nil, &InvalidError{"escolha duas versões diferentes"}
	}
	vf, err := s.repo.FindVersion(ctx, name, from)
	if err != nil {
		return nil, nil, err
	}
	return vf, vt, nil
}

// Diff compara as listas de itens das duas versões.
func (s *Service) Diff(ctx context.Context, name string, from, to int) (*Diff, error) {
	vf, vt, err := s.pair(ctx, name, from, to)
	if err != nil {
		return nil, err
	}
	d := &Diff{Dataset: name, To: vt.info(), Added: []ItemRef{}, Removed: []ItemRef{}, Changed: []ItemRef{}}
	var old []ItemRef
	if vf != nil {
		d.From = vf.info()
		if old, err = s.repo.ItemRefs(ctx, vf.ID); err != nil {
			return nil, err
		}
	}
	cur, err := s.repo.ItemRefs(ctx, vt.ID)
	if err != nil {
		return nil, err
	}
	// as duas em ordem de sha1: anda nas duas ao mesmo tempo
	i, j := 0, 0
	for i < len(old) || j < len(cur) {
		switch {
		case j == len(cur) || (i < len(old) && old[i].SHA1 < cur[j].SHA1):
			d.Removed = append(d.Removed, old[i])
			i++
		case i == len(old) || cur[j].SHA1 < old[i].SHA1:
			d.Added = append(d.Added, cur[j])
			j++
		default:
			if old[i].Hash != cur[j].Hash {
				d.Changed = append(d.Changed, cur[j])
			} else {
				d.Unchanged++
			}
			i++
			j++
		}
	}
	return d, nil
}

// DiffItem: o que mudou numa imagem entre as duas versões.
func (s *Service) DiffItem(ctx context.Context, name string, from, to int, sha1 string) (*ItemDiff, error) {
	vf, vt, err := s.pair(ctx, name, from, to)
	if err != nil {
		return nil, err
	}
	var a, b *Item
	if vf != nil {
		if a, err = s.findItem(ctx, vf.ID, sha1); err != nil {
			return nil, err
		}
	}
	if b, err = s.findItem(ctx, vt.ID, sha1); err != nil {
		return nil, err
	}
	if a == nil && b == nil {
		return nil, platform.ErrNotFound
	}
	d := &ItemDiff{SHA1: sha1, Fields: []Change{}, Objects: []Change{}, Tasks: []TaskChange{}}
	switch {
	case a == nil:
		d.Status = StatusAdded
		a = &Item{}
	case b == nil:
		d.Status = StatusRemoved
		b = &Item{}
	case a.Hash == b.Hash:
		d.Status = StatusUnchanged
	default:
		d.Status = StatusChanged
	}
	for _, it := range []*Item{a, b} {
		if it.SHA1 != "" {
			d.Package, d.FileName = it.Package, it.FileName
		}
	}
	d.Fields = diffFields("fields.", a.Fields, b.Fields)
	d.Objects = diffObjects(a.Objects, b.Objects)
	d.Tasks = diffTasks(a.Boxes, b.Boxes)
	return d, nil
}

func (s *Service) findItem(ctx context.Context, version, sha1 string) (*Item, error) {
	it, err := s.repo.FindItem(ctx, version, sha1)
	if errors.Is(err, platform.ErrNotFound) {
		return nil, nil
	}
	return it, err
}

// diffFields: os campos que mudaram, em ordem de chave.
func diffFields(prefix string, a, b map[string]any) []Change {
	keys := slices.Sorted(maps.Keys(a))
	for k := range b {
		if _, ok := a[k]; !ok {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	out := []Change{}
	for _, k := range keys {
		va, vb := plain(a[k]), plain(b[k])
		if !same(va, vb) {
			out = append(out, Change{Key: prefix + k, From: va, To: vb})
		}
	}
	return out
}

// diffObjects: os objetos comparados por tipo e posição (o 1º veículo da
// velha com o 1º da nova).
func diffObjects(a, b []annotation.Object) []Change {
	byType := func(objs []annotation.Object) map[string][]annotation.Object {
		m := map[string][]annotation.Object{}
		for _, o := range objs {
			m[o.Type] = append(m[o.Type], o)
		}
		return m
	}
	ta, tb := byType(a), byType(b)
	types := slices.Sorted(maps.Keys(ta))
	for t := range tb {
		if _, ok := ta[t]; !ok {
			types = append(types, t)
		}
	}
	slices.Sort(types)
	out := []Change{}
	for _, t := range types {
		n := max(len(ta[t]), len(tb[t]))
		for i := range n {
			var fa, fb map[string]any
			if i < len(ta[t]) {
				fa = ta[t][i].Fields
			}
			if i < len(tb[t]) {
				fb = tb[t][i].Fields
			}
			prefix := "objects." + t + "."
			if n > 1 {
				prefix = fmt.Sprintf("objects.%s[%d].", t, i+1)
			}
			out = append(out, diffFields(prefix, fa, fb)...)
		}
	}
	return out
}

// diffTasks: as tarefas cujas caixas mudaram, em ordem de nome.
func diffTasks(a, b map[string]TaskSet) []TaskChange {
	names := slices.Sorted(maps.Keys(a))
	for t := range b {
		if _, ok := a[t]; !ok {
			names = append(names, t)
		}
	}
	slices.Sort(names)
	out := []TaskChange{}
	for _, t := range names {
		sa, sb := a[t], b[t]
		if same(sa, sb) {
			continue
		}
		ca, cb := countLabels(sa.Boxes), countLabels(sb.Boxes)
		tc := TaskChange{Task: t, From: len(sa.Boxes), To: len(sb.Boxes), Labels: []LabelChange{}}
		labels := slices.Sorted(maps.Keys(ca))
		for l := range cb {
			if _, ok := ca[l]; !ok {
				labels = append(labels, l)
			}
		}
		slices.Sort(labels)
		for _, l := range labels {
			if ca[l] != cb[l] {
				tc.Labels = append(tc.Labels, LabelChange{Label: l, From: ca[l], To: cb[l]})
			}
		}
		tc.Edited = len(tc.Labels) == 0
		out = append(out, tc)
	}
	return out
}

func countLabels(bs []boxes.Box) map[string]int {
	m := map[string]int{}
	for _, b := range bs {
		m[b.Label]++
	}
	return m
}

// plain: o valor como o JSON mostra (o Mongo devolve listas como bson.A e
// documentos como bson.D).
func plain(v any) any {
	switch x := v.(type) {
	case bson.A:
		return plain([]any(x))
	case []any:
		out := make([]any, len(x))
		for i := range x {
			out[i] = plain(x[i])
		}
		return out
	case bson.D:
		out := make(map[string]any, len(x))
		for _, e := range x {
			out[e.Key] = plain(e.Value)
		}
		return out
	case bson.M:
		return plain(map[string]any(x))
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = plain(e)
		}
		return out
	}
	return v
}

// same: iguais como JSON (o mesmo critério do hash: 3 e 3.0 são iguais).
func same(a, b any) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}
