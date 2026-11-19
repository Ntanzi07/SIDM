package images

// Pesquisa/dashboard: quantas imagens de um perfil de anotação têm cada valor
// de cada campo, por pacote e por status. Sempre de UM perfil por vez (o perfil
// é do pacote: as imagens do perfil são as dos pacotes que usam ele).

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"

	"SIDM/internal/annotation"
	"SIDM/internal/platform"
)

// Counts: valor → quantidade. Um sim/não vira "true"/"false"; um número, o
// número em texto. Num campo de lista (multi) a imagem conta em cada valor
// que tem, então a soma pode passar do total.
type Counts map[string]int64

// ObjectStats: um tipo de objeto. Images = imagens com pelo menos um; Count =
// objetos ao todo; Fields conta OBJETOS (160 carros, não 160 imagens).
type ObjectStats struct {
	Images int64             `json:"images"`
	Count  int64             `json:"count"`
	Fields map[string]Counts `json:"fields"`
}

// Stats é a resposta do GET /images/stats.
type Stats struct {
	Profile  string                  `json:"profile"`
	Total    int64                   `json:"total"`
	Packages Counts                  `json:"packages"`
	Status   Counts                  `json:"status"`
	Fields   map[string]Counts       `json:"fields"`
	Objects  map[string]*ObjectStats `json:"objects"`
}

// StatsSpec diz o que contar: os campos da imagem e, por tipo de objeto, os
// campos dele. Só entram campos de lista e sim/não (choice, multi, bool):
// contar texto livre, número ou data por valor não diz nada.
type StatsSpec struct {
	Fields  []string
	Objects map[string][]string
}

// countable: os tipos de campo que a pesquisa conta por valor.
func countable(f annotation.Field) bool {
	return f.Type == annotation.TypeChoice || f.Type == annotation.TypeMulti || f.Type == annotation.TypeBool
}

// SpecOf monta o spec do perfil. Os tipos de objeto COM caixa ficam de fora:
// as caixas vêm do CVAT e moram no módulo boxes, não na imagem.
func SpecOf(p *annotation.Profile) StatsSpec {
	spec := StatsSpec{Fields: []string{}, Objects: map[string][]string{}}
	for _, f := range p.Fields {
		if countable(f) {
			spec.Fields = append(spec.Fields, f.Key)
		}
	}
	for _, o := range p.Objects {
		if o.Box {
			continue
		}
		keys := []string{}
		for _, f := range o.Fields {
			if countable(f) {
				keys = append(keys, f.Key)
			}
		}
		spec.Objects[o.Key] = keys
	}
	return spec
}

// emptyStats: tudo zerado, com as chaves do spec (a tela mostra os campos
// mesmo sem imagem nenhuma).
func emptyStats(spec StatsSpec) *Stats {
	st := &Stats{Packages: Counts{}, Status: Counts{}, Fields: map[string]Counts{}, Objects: map[string]*ObjectStats{}}
	for _, k := range spec.Fields {
		st.Fields[k] = Counts{}
	}
	for t, keys := range spec.Objects {
		o := &ObjectStats{Fields: map[string]Counts{}}
		for _, k := range keys {
			o.Fields[k] = Counts{}
		}
		st.Objects[t] = o
	}
	return st
}

// countKey: o valor como chave do Counts.
func countKey(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	}
	return fmt.Sprint(v)
}

// StatsError: um pedido que não dá para atender (perfil, campo ou valor que
// não existe). Vira 400.
type StatsError struct{ Msg string }

func (e *StatsError) Error() string { return e.Msg }

// Stats conta as imagens do perfil que passam no filtro. O filtro chega sem
// pacotes do perfil: o serviço põe (e um Package de outro perfil dá zero).
func (s *Service) Stats(ctx context.Context, profile string, f ImageFilter) (*Stats, error) {
	if strings.TrimSpace(profile) == "" {
		return nil, &StatsError{"informe o perfil de anotação (?profile=)"}
	}
	p, err := s.profileNamed(ctx, profile)
	if err != nil {
		return nil, err
	}
	spec := SpecOf(p)
	pkgs, err := s.catalog.PackagesWithProfile(ctx, p.Name)
	if err != nil {
		return nil, err
	}
	if f.Package != "" {
		if !slices.Contains(pkgs, f.Package) {
			pkgs = nil // pacote de outro perfil (ou que não existe): nada a contar
		} else {
			pkgs = []string{f.Package}
		}
		f.Package = ""
	}
	if len(pkgs) == 0 {
		st := emptyStats(spec)
		st.Profile = p.Name
		return st, nil
	}
	f.Packages, f.Limit, f.Skip = pkgs, 0, 0
	st, err := s.repo.Stats(ctx, f, spec)
	if err != nil {
		return nil, err
	}
	st.Profile = p.Name
	return st, nil
}

// GET /images/stats?profile=veiculos&status=verified&package=...&fields.<campo>=...&objects.<tipo>.<campo>=...
//
// status: verified (padrão), pending ou all. fields.<campo> e
// objects.<tipo>.<campo> são os campos do perfil, com o valor pelo valor, código
// ou nome da opção (sim/não: true/false). Os objects.<tipo>.<campo> de um mesmo
// tipo valem para o MESMO objeto, como no filtro do export.
func (s *Service) handleStats(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	name := q.Get("profile")
	if strings.TrimSpace(name) == "" {
		platform.WriteError(w, http.StatusBadRequest, "informe o perfil de anotação (?profile=)")
		return
	}
	p, err := s.profileNamed(r.Context(), name)
	if errors.Is(err, platform.ErrNotFound) {
		platform.WriteError(w, http.StatusNotFound, fmt.Sprintf("perfil de anotação %q não encontrado", name))
		return
	}
	if err != nil {
		platform.ServerError(w, err)
		return
	}
	f, err := statsFilter(q, p)
	var bad *StatsError
	if errors.As(err, &bad) {
		platform.WriteError(w, http.StatusBadRequest, bad.Msg)
		return
	}
	st, err := s.Stats(r.Context(), p.Name, f)
	switch {
	case errors.As(err, &bad):
		platform.WriteError(w, http.StatusBadRequest, bad.Msg)
	case err != nil:
		platform.ServerError(w, err)
	default:
		platform.WriteJSON(w, http.StatusOK, st)
	}
}

// statsFilter lê o filtro da query pelos campos do perfil: um campo que o
// perfil não tem, ou um valor fora das opções, é erro (melhor que um zero
// que parece resposta).
func statsFilter(q map[string][]string, p *annotation.Profile) (ImageFilter, error) {
	f := ImageFilter{Package: strings.TrimSpace(first(q, "package")), Fields: map[string]any{}}
	switch st := strings.TrimSpace(first(q, "status")); st {
	case "", StatusVerified:
		f.Status = StatusVerified
	case StatusPending:
		f.Status = StatusPending
	case "all":
	default:
		return f, &StatsError{fmt.Sprintf("status %q: use verified, pending ou all", st)}
	}
	objs := map[string]map[string]any{}
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		v := strings.TrimSpace(first(q, key))
		if v == "" {
			continue
		}
		switch {
		case strings.HasPrefix(key, "fields."):
			k := strings.TrimPrefix(key, "fields.")
			def, ok := p.Field(k)
			if !ok {
				return f, &StatsError{fmt.Sprintf("%s: o perfil %q não tem esse campo", key, p.Name)}
			}
			val, err := queryValue(def, v)
			if err != nil {
				return f, &StatsError{key + ": " + err.Error()}
			}
			f.Fields[k] = val
		case strings.HasPrefix(key, "objects."):
			typ, k, ok := strings.Cut(strings.TrimPrefix(key, "objects."), ".")
			if !ok {
				return f, &StatsError{fmt.Sprintf("%s: use objects.<tipo>.<campo>", key)}
			}
			ot, ok := p.Object(typ)
			if !ok || ot.Box {
				return f, &StatsError{fmt.Sprintf("%s: o perfil %q não tem o tipo de objeto %q (sem caixa)", key, p.Name, typ)}
			}
			def, ok := ot.Field(k)
			if !ok {
				return f, &StatsError{fmt.Sprintf("%s: o tipo %q não tem esse campo", key, typ)}
			}
			val, err := queryValue(def, v)
			if err != nil {
				return f, &StatsError{key + ": " + err.Error()}
			}
			if objs[typ] == nil {
				objs[typ] = map[string]any{}
			}
			objs[typ][k] = val
		}
	}
	for _, typ := range sortedKeys(objs) {
		f.Objects = append(f.Objects, ObjectFilter{Type: typ, Fields: objs[typ]})
	}
	return f, nil
}

func first(q map[string][]string, key string) string {
	if v := q[key]; len(v) > 0 {
		return v[0]
	}
	return ""
}

// queryValue: o texto da query no valor guardado. Numa lista (multi) é UM
// valor (a imagem passa se a lista contém ele), não a lista.
func queryValue(def *annotation.Field, s string) (any, error) {
	switch def.Type {
	case annotation.TypeChoice, annotation.TypeMulti:
		if v, ok := def.OptionOf(s); ok {
			return v, nil
		}
		return nil, fmt.Errorf("%q não é uma opção do campo", s)
	case annotation.TypeDateTime:
		return nil, errors.New("não dá para filtrar por data e hora")
	case annotation.TypeText:
		if def.Upper {
			return strings.ToUpper(s), nil
		}
		return s, nil
	}
	return def.Typed(s)
}
