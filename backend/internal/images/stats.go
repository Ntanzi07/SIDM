package images

// Pesquisa/dashboard: quantas imagens de um perfil de anotação têm cada valor
// de cada campo, por pacote e por status. Sempre de UM perfil por vez (o perfil
// é do pacote: as imagens do perfil são as dos pacotes que usam ele).

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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
//
// Missing: por campo, quantos ficaram sem valor ("Não informado"): imagens
// em Stats.Missing, objetos em ObjectStats.Missing. Vem pronto porque num
// campo multi a conta "total − soma" não funciona.
type ObjectStats struct {
	Images  int64             `json:"images"`
	Count   int64             `json:"count"`
	Fields  map[string]Counts `json:"fields"`
	Missing Counts            `json:"missing"`
}

// Stats é a resposta do GET /images/stats.
type Stats struct {
	Profile  string                  `json:"profile"`
	Total    int64                   `json:"total"`
	Packages Counts                  `json:"packages"`
	Status   Counts                  `json:"status"`
	Fields   map[string]Counts       `json:"fields"`
	Missing  Counts                  `json:"missing"`
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
	st := &Stats{Packages: Counts{}, Status: Counts{}, Fields: map[string]Counts{}, Missing: Counts{}, Objects: map[string]*ObjectStats{}}
	for _, k := range spec.Fields {
		st.Fields[k] = Counts{}
		st.Missing[k] = 0
	}
	for t, keys := range spec.Objects {
		o := &ObjectStats{Fields: map[string]Counts{}, Missing: Counts{}}
		for _, k := range keys {
			o.Fields[k] = Counts{}
			o.Missing[k] = 0
		}
		st.Objects[t] = o
	}
	return st
}

// hasValue: o campo tem valor (nem ausente, nem vazio, nem lista vazia).
func hasValue(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return x != ""
	case []string:
		return len(x) > 0
	case []any:
		return len(x) > 0
	}
	return true
}

// CountStats faz a conta do Stats em Go, imagem por imagem (a mesma do
// aggregate do MongoRepo.Stats): a memória usa, e o módulo versions conta com
// ela as imagens congeladas de uma versão.
func CountStats(imgs []*Image, spec StatsSpec) *Stats {
	st := emptyStats(spec)
	// cada valor de uma lista conta; um valor solto conta uma vez ($unwind)
	add := func(c Counts, v any) {
		switch x := v.(type) {
		case nil:
		case []string:
			for _, s := range x {
				c[s]++
			}
		case []any:
			for _, e := range x {
				c[countKey(e)]++
			}
		default:
			c[countKey(x)]++
		}
	}
	for _, img := range imgs {
		st.Total++
		st.Packages[img.Package]++
		st.Status[img.Status]++
		for _, k := range spec.Fields {
			add(st.Fields[k], img.Fields[k])
			if !hasValue(img.Fields[k]) {
				st.Missing[k]++
			}
		}
		for t, keys := range spec.Objects {
			o, n := st.Objects[t], int64(0)
			for _, obj := range img.Objects {
				if obj.Type != t {
					continue
				}
				n++
				for _, k := range keys {
					add(o.Fields[k], obj.Fields[k])
					if !hasValue(obj.Fields[k]) {
						o.Missing[k]++
					}
				}
			}
			o.Count += n
			if n > 0 {
				o.Images++
			}
		}
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

// Unwrap: para o boxes (que não importa este módulo) reconhecer o 400.
func (e *StatsError) Unwrap() error { return platform.ErrInvalid }

// searchFilter prepara o filtro de uma pesquisa no perfil: Package/Packages
// do filtro são os pacotes PEDIDOS, cruzados com os do perfil (vazio = todos
// os do perfil; um pacote de outro perfil não conta nada). none = nenhum
// pacote sobrou: não há o que buscar.
func (s *Service) searchFilter(ctx context.Context, profile string, f ImageFilter) (p *annotation.Profile, out ImageFilter, none bool, err error) {
	if strings.TrimSpace(profile) == "" {
		return nil, f, false, &StatsError{"informe o perfil de anotação (?profile=)"}
	}
	if p, err = s.profileNamed(ctx, profile); err != nil {
		return nil, f, false, err
	}
	pkgs, err := s.catalog.PackagesWithProfile(ctx, p.Name)
	if err != nil {
		return nil, f, false, err
	}
	asked := f.Packages
	if f.Package != "" {
		asked = append(slices.Clone(asked), f.Package)
	}
	if len(asked) > 0 {
		pkgs = slices.DeleteFunc(slices.Clone(pkgs), func(p string) bool { return !slices.Contains(asked, p) })
	}
	f.Package, f.Packages, f.Limit, f.Skip = "", pkgs, 0, 0
	return p, f, len(pkgs) == 0, nil
}

// Stats conta as imagens do perfil que passam no filtro (ver searchFilter).
func (s *Service) Stats(ctx context.Context, profile string, f ImageFilter) (*Stats, error) {
	p, f, none, err := s.searchFilter(ctx, profile, f)
	if err != nil {
		return nil, err
	}
	spec := SpecOf(p)
	st := emptyStats(spec)
	if !none {
		if st, err = s.repo.Stats(ctx, f, spec); err != nil {
			return nil, err
		}
	}
	st.Profile = p.Name
	return st, nil
}

// SearchIDs: os sha1 das imagens que passam na pesquisa da query (os mesmos
// parâmetros do GET /images/stats). É o que o boxes usa para contar as labels
// com os mesmos filtros. Erros: *StatsError (pedido inválido) ou
// platform.ErrNotFound (perfil que não existe).
func (s *Service) SearchIDs(ctx context.Context, q url.Values) ([]string, error) {
	name := strings.TrimSpace(q.Get("profile"))
	if name == "" {
		return nil, &StatsError{"informe o perfil de anotação (?profile=)"}
	}
	p, err := s.profileNamed(ctx, name)
	if errors.Is(err, platform.ErrNotFound) {
		return nil, fmt.Errorf("perfil de anotação %q: %w", name, err)
	}
	if err != nil {
		return nil, err
	}
	f, err := statsFilter(q, p)
	if err != nil {
		return nil, err
	}
	_, f, none, err := s.searchFilter(ctx, p.Name, f)
	if err != nil || none {
		return []string{}, err
	}
	return s.repo.MatchingIDs(ctx, f)
}

// GET /images/stats?profile=veiculos&status=verified&package=a&package=b&fields.<campo>=...&objects.<tipo>.<campo>=...
//
// package: repetido para vários (sem nenhum: todos os pacotes do perfil).
// status: verified (padrão), pending ou all. fields.<campo> e
// objects.<tipo>.<campo> são os campos do perfil, com o valor pelo valor, código
// ou nome da opção (sim/não: true/false). Os objects.<tipo>.<campo> de um mesmo
// tipo valem para o MESMO objeto, como no filtro do export.
func (s *Service) handleStats(w http.ResponseWriter, r *http.Request) {
	p, f, ok := s.searchRequest(w, r)
	if !ok {
		return
	}
	st, err := s.Stats(r.Context(), p.Name, f)
	var bad *StatsError
	switch {
	case errors.As(err, &bad):
		platform.WriteError(w, http.StatusBadRequest, bad.Msg)
	case err != nil:
		platform.ServerError(w, err)
	default:
		platform.WriteJSON(w, http.StatusOK, st)
	}
}

// GET /images/search?<os mesmos parâmetros do /images/stats>&limit=50&skip=0
//
// As imagens da pesquisa, paginadas (na ordem da fila, como o GET /images).
func (s *Service) handleSearch(w http.ResponseWriter, r *http.Request) {
	p, f, ok := s.searchRequest(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	_, f, none, err := s.searchFilter(r.Context(), p.Name, f)
	if err != nil {
		platform.ServerError(w, err)
		return
	}
	page := platform.Page[Image]{Items: []Image{}}
	if !none {
		f.Limit, f.Skip = platform.IntParam(q, "limit", 50, 1, 500), platform.IntParam(q, "skip", 0, 0, 1<<31)
		if page.Items, page.Total, err = s.repo.ListImages(r.Context(), f); err != nil {
			platform.ServerError(w, err)
			return
		}
	}
	platform.WriteJSON(w, http.StatusOK, page)
}

// searchRequest lê o perfil e o filtro de uma pesquisa; responde o erro
// (400/404) e devolve ok=false quando não dá.
func (s *Service) searchRequest(w http.ResponseWriter, r *http.Request) (*annotation.Profile, ImageFilter, bool) {
	q := r.URL.Query()
	name := q.Get("profile")
	if strings.TrimSpace(name) == "" {
		platform.WriteError(w, http.StatusBadRequest, "informe o perfil de anotação (?profile=)")
		return nil, ImageFilter{}, false
	}
	p, err := s.profileNamed(r.Context(), name)
	if errors.Is(err, platform.ErrNotFound) {
		platform.WriteError(w, http.StatusNotFound, fmt.Sprintf("perfil de anotação %q não encontrado", name))
		return nil, ImageFilter{}, false
	}
	if err != nil {
		platform.ServerError(w, err)
		return nil, ImageFilter{}, false
	}
	f, err := statsFilter(q, p)
	var bad *StatsError
	if errors.As(err, &bad) {
		platform.WriteError(w, http.StatusBadRequest, bad.Msg)
		return nil, ImageFilter{}, false
	}
	return p, f, true
}

// statsFilter lê o filtro da query pelos campos do perfil: um campo que o
// perfil não tem, ou um valor fora das opções, é erro (melhor que um zero
// que parece resposta).
func statsFilter(q map[string][]string, p *annotation.Profile) (ImageFilter, error) {
	f := ImageFilter{Fields: map[string]any{}}
	for _, p := range q["package"] { // package=a&package=b
		if p = strings.TrimSpace(p); p != "" && !slices.Contains(f.Packages, p) {
			f.Packages = append(f.Packages, p)
		}
	}
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

// ParseFilter lê a pesquisa da query (?package=&status=&fields.x=&objects.t.k=)
// do mesmo jeito que o /images/stats. O módulo versions filtra com ela os
// itens congelados de uma versão.
func ParseFilter(q map[string][]string, p *annotation.Profile) (ImageFilter, error) {
	return statsFilter(q, p)
}

// Matches: a imagem passa no filtro (a mesma regra da memória).
func (f ImageFilter) Matches(img *Image) bool { return f.match(img) }
