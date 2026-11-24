package annotation

// A herança de perfis: um perfil pode estender outro (Extends). O perfil
// "resolvido" (o que o resto do sistema usa) tem os campos e os tipos de
// objeto do pai primeiro e os do filho depois, cada herdado marcado com
// From. No banco, cada perfil guarda só o que é dele.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"SIDM/internal/platform"
)

// MaxDepth: quantos perfis numa linha de herança (o próprio e os pais).
const MaxDepth = 5

// raw: o perfil como guardado (padrão do código ou do banco), sem herança.
func (s *Service) raw(ctx context.Context, name string) (*Profile, error) {
	if p, ok := Builtin(name); ok {
		return s.builtinWithLabels(ctx, p)
	}
	return s.repo.FindProfile(ctx, name)
}

// resolve monta o perfil com os campos dos pais.
func (s *Service) resolve(ctx context.Context, p *Profile) (*Profile, error) {
	return resolveBy(ctx, p, s.raw)
}

func resolveBy(ctx context.Context, p *Profile, lookup func(context.Context, string) (*Profile, error)) (*Profile, error) {
	chain := []*Profile{p}
	for cur := p; cur.Extends != ""; {
		if len(chain) >= MaxDepth {
			return nil, &InvalidError{Msg: fmt.Sprintf("o perfil %q passa de %d níveis de herança", p.Name, MaxDepth)}
		}
		parent, err := lookup(ctx, cur.Extends)
		if errors.Is(err, platform.ErrNotFound) {
			return nil, &InvalidError{Msg: fmt.Sprintf("o perfil %q estende %q, que não existe", cur.Name, cur.Extends)}
		}
		if err != nil {
			return nil, err
		}
		for _, seen := range chain {
			if seen.Name == parent.Name {
				return nil, &InvalidError{Msg: fmt.Sprintf("herança em círculo: %s → %s", names(chain), parent.Name)}
			}
		}
		chain = append(chain, parent)
		cur = parent
	}
	// do mais longe (a raiz) ao próprio
	out := *chain[len(chain)-1]
	out.Fields = mark(out.Fields, "")
	out.Objects = markObjects(out.Objects, "")
	for i := len(chain) - 2; i >= 0; i-- {
		var err error
		if out, err = merge(out, *chain[i]); err != nil {
			return nil, err
		}
	}
	out.Lineage = make([]string, len(chain))
	for i, c := range chain {
		out.Lineage[i] = c.Name
	}
	return &out, nil
}

func names(chain []*Profile) string {
	out := make([]string, len(chain))
	for i, c := range chain {
		out[i] = c.Name
	}
	return strings.Join(out, " → ")
}

// merge: o pai (já resolvido) com os campos do filho por cima. Uma chave
// repetida é erro (o filho não troca um campo do pai, só acrescenta).
func merge(parent, child Profile) (Profile, error) {
	out := child
	out.Fields = append(mark(parent.Fields, parent.Name), mark(child.Fields, "")...)
	var ps problems
	seen := map[string]bool{}
	for i, f := range out.Fields {
		if seen[f.Key] {
			ps.add(fmt.Sprintf("fields[%d].key", i), "o campo %q já vem do perfil %s", f.Key, parent.Name)
		}
		seen[f.Key] = true
	}
	out.Objects = markObjects(parent.Objects, parent.Name)
	for i, o := range child.Objects {
		j := slices.IndexFunc(out.Objects, func(t ObjectType) bool { return t.Key == o.Key })
		if j < 0 {
			out.Objects = append(out.Objects, markObjects([]ObjectType{o}, "")...)
			continue
		}
		// o tipo do pai: aqui só se acrescentam campos (e caixas que o representam)
		t := &out.Objects[j]
		path := fmt.Sprintf("objects[%d]", i)
		if o.Box != t.Box || (o.Min != 0 && o.Min != t.Min) || (o.Max != 0 && o.Max != t.Max) {
			ps.add(path, "o tipo %q vem do perfil %s: aqui só dá para acrescentar campos", o.Key, parent.Name)
		}
		fields := slices.Clone(t.Fields)
		for k, f := range o.Fields {
			if _, dup := t.Field(f.Key); dup {
				ps.add(fmt.Sprintf("%s.fields[%d].key", path, k), "o campo %q de %q já vem do perfil %s", f.Key, o.Key, parent.Name)
				continue
			}
			f.From = ""
			fields = append(fields, f)
		}
		t.Fields = fields
		for _, l := range o.BoxLabels {
			if slices.ContainsFunc(t.BoxLabels, func(b BoxLabel) bool { return b.Label == l.Label }) {
				ps.add(path+".box_labels", "a caixa %q de %q já vem do perfil %s", l.Label, o.Key, parent.Name)
				continue
			}
			l.From = ""
			t.BoxLabels = append(t.BoxLabels, l)
		}
	}
	if len(ps) > 0 {
		return Profile{}, &InvalidError{Msg: summary(ps), Problems: ps}
	}
	return out, nil
}

// mark: cópia dos campos com From (só onde ainda não tem: um campo do avô
// continua dizendo que veio do avô).
func mark(fs []Field, from string) []Field {
	out := slices.Clone(fs)
	for i := range out {
		if out[i].From == "" {
			out[i].From = from
		}
	}
	return out
}

func markObjects(os []ObjectType, from string) []ObjectType {
	out := slices.Clone(os)
	for i := range out {
		if out[i].From == "" {
			out[i].From = from
		}
		out[i].Fields = mark(out[i].Fields, from)
		out[i].BoxLabels = slices.Clone(out[i].BoxLabels)
		for j := range out[i].BoxLabels {
			if out[i].BoxLabels[j].From == "" {
				out[i].BoxLabels[j].From = from
			}
		}
	}
	return out
}

func ownLabels(ls []BoxLabel) []BoxLabel {
	return slices.DeleteFunc(slices.Clone(ls), func(l BoxLabel) bool { return l.From != "" })
}

// own: só o que é do perfil (tira o que veio marcado de outro), para gravar.
// A tela manda o perfil como recebeu, com os herdados.
func own(p *Profile) {
	p.Fields = slices.DeleteFunc(p.Fields, func(f Field) bool { return f.From != "" })
	var objs []ObjectType
	for _, o := range p.Objects {
		o.Fields = slices.DeleteFunc(o.Fields, func(f Field) bool { return f.From != "" })
		o.BoxLabels = ownLabels(o.BoxLabels)
		if o.From != "" && len(o.Fields) == 0 && len(o.BoxLabels) == 0 {
			continue // tipo todo herdado, sem nada acrescentado
		}
		if o.From != "" { // tipo do pai com campos acrescentados: só a chave e o acréscimo
			o = ObjectType{Key: o.Key, Fields: o.Fields, BoxLabels: ownLabels(o.BoxLabels)}
		}
		objs = append(objs, o)
	}
	p.Objects = objs
	p.Lineage = nil
}

// Family: o perfil e todos os que o estendem (filhos, netos...). Uma
// pesquisa do perfil olha os pacotes de todos eles.
func (s *Service) Family(ctx context.Context, name string) ([]string, error) {
	saved, err := s.repo.ListProfiles(ctx)
	if err != nil {
		return nil, err
	}
	out := []string{name}
	for i := 0; i < len(out); i++ {
		for _, p := range saved {
			if p.Extends == out[i] && !slices.Contains(out, p.Name) {
				out = append(out, p.Name)
			}
		}
	}
	return out, nil
}

// checkFamily: depois de mudar o perfil p, os que o estendem continuam
// fechando (um campo novo no pai pode bater com um do filho).
func (s *Service) checkFamily(ctx context.Context, p *Profile) error {
	family, err := s.Family(ctx, p.Name)
	if err != nil {
		return err
	}
	for _, name := range family[1:] {
		child, err := s.raw(ctx, name)
		if err != nil {
			return err
		}
		if _, err := s.resolveWith(ctx, child, p); err != nil {
			var invalid *InvalidError
			if errors.As(err, &invalid) {
				return &InvalidError{Msg: fmt.Sprintf("o perfil %q estende este e deixaria de fechar: %s", name, invalid.Msg)}
			}
			return err
		}
	}
	return nil
}

// resolveWith resolve p como se o perfil changed já estivesse gravado.
func (s *Service) resolveWith(ctx context.Context, p *Profile, changed *Profile) (*Profile, error) {
	return resolveBy(ctx, p, func(ctx context.Context, name string) (*Profile, error) {
		if name == changed.Name {
			c := *changed
			return &c, nil
		}
		return s.raw(ctx, name)
	})
}
