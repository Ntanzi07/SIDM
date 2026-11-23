// Package export monta datasets de treino (RF-DETR, YOLO, VOC) a partir das
// imagens e das caixas guardadas no SIDM, ou só as imagens renomeadas. Sem banco e sem HTTP: quem chama
// entrega as imagens, e este pacote decide a divisão, traduz as labels e
// escreve o zip.
package export

import (
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"SIDM/internal/annotation"
)

// Formatos de saída.
const (
	FormatCOCO = "coco" // layout do Roboflow, o que o RF-DETR lê: train/valid/test com _annotations.coco.json
	FormatYOLO = "yolo" // Ultralytics: images/, labels/ e data.yaml
	FormatVOC  = "voc"  // Pascal VOC: um .xml por imagem, ao lado dela
	// FormatImages: só as imagens, com o nome de saída (o v2 ou o modelo), e
	// um anotacoes.csv com os dados anotados. Sem caixas: não usa tarefa nem
	// labels, e entra toda verificada que passa nos filtros.
	FormatImages = "images"
)

// NeedsTasks: o formato usa as caixas de uma tarefa? (todos, menos o images)
func (p *Profile) NeedsTasks() bool { return p.Format != FormatImages }

// Nomes dos grupos da divisão.
const (
	SplitTrain = "train"
	SplitValid = "valid"
	SplitTest  = "test"
)

// Profile é um perfil de export: o "molde" de um dataset.
type Profile struct {
	Name        string `json:"name" bson:"name"`
	Description string `json:"description,omitempty" bson:"description,omitempty"`
	Format      string `json:"format" bson:"format"`
	Split       Split  `json:"split" bson:"split"`
	// Layout: "split" (train/valid/test, o padrão) ou "folders" (árvore de
	// pastas pelos metadados, nos níveis de Folders; só VOC).
	Layout  string        `json:"layout,omitempty" bson:"layout,omitempty"`
	Folders []FolderLevel `json:"folders,omitempty" bson:"folders,omitempty"`
	// FileName: modelo do nome dos arquivos de saída (ver rename.go). Vazio =
	// o nome guardado no SIDM (v2).
	FileName string `json:"file_name,omitempty" bson:"file_name,omitempty"`
	// Tasks: as tarefas de anotação cujas caixas entram, juntas num dataset
	// só (ex.: para-brisa + placa). Vazio = escolher na hora de exportar (os
	// perfis padrão servem para qualquer tarefa).
	Tasks []string `json:"tasks,omitempty" bson:"tasks,omitempty"`
	// AllTasks: só entram as imagens anotadas em TODAS as tarefas. Sem isso,
	// uma imagem anotada só na tarefa do para-brisa entraria sem as caixas de
	// placa, e o modelo aprenderia que a placa não marcada é fundo.
	AllTasks bool `json:"all_tasks,omitempty" bson:"all_tasks,omitempty"`
	// Task é o formato de antes (uma tarefa só): o Normalize passa para Tasks.
	Task string `json:"task,omitempty" bson:"task,omitempty"`
	// AnnotationProfile: o perfil de anotação cujos campos os filtros, as
	// pastas e o modelo de nome usam (vazio = o padrão, "veiculos"). Não
	// limita os pacotes: imagem de pacote de outro perfil só não tem os
	// campos (não passa nos filtros; vai para a pasta "Desconhecido").
	AnnotationProfile string `json:"annotation_profile,omitempty" bson:"annotation_profile,omitempty"`

	// Packages: quais pacotes entram (vazio = todos).
	Packages []string `json:"packages,omitempty" bson:"packages,omitempty"`
	// Filters: só entram as imagens com estes metadados (vazio = qualquer).
	Filters Filters `json:"filters" bson:"filters"`
	// Labels: quais classes entram e com que nome.
	Labels LabelRules `json:"labels" bson:"labels"`

	Builtin   bool      `json:"builtin,omitempty" bson:"-"` // perfil padrão do sistema (não dá para editar)
	CreatedBy string    `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitzero" bson:"updated_at,omitempty"`

	// specs: os metadados do perfil de anotação (o Bind preenche).
	specs map[string]MetaField
	ap    *annotation.Profile
}

// Split em porcentagens (somam 100). A divisão é FIXA por imagem: cada
// imagem cai sempre no mesmo grupo (decidido pelo sha1), então re-exportar
// com imagens novas não muda o grupo das antigas.
type Split struct {
	Train int `json:"train" bson:"train"`
	Valid int `json:"valid" bson:"valid"`
	Test  int `json:"test" bson:"test"`
}

// Filters: só entram as imagens com estes valores, nos campos do perfil de
// anotação (os mesmos valores da anotação: "night", "rain"...).
type Filters struct {
	// Fields: campo da imagem -> valor (numa lista, basta conter o valor).
	Fields map[string]string `json:"fields,omitempty" bson:"fields,omitempty"`
	// Objects: tipo de objeto -> campo -> valor. Todos os campos de um tipo
	// valem para o MESMO objeto (um caminhão branco, não um caminhão e algo
	// branco).
	Objects map[string]map[string]string `json:"objects,omitempty" bson:"objects,omitempty"`

	// Os filtros de antes dos perfis de anotação (perfil veiculos). Ainda
	// são aceitos; o Normalize passa para Fields e Objects["vehicle"].
	Equipment    string `json:"equipment,omitempty" bson:"equipment,omitempty"`
	Camera       string `json:"camera,omitempty" bson:"camera,omitempty"`
	Direction    string `json:"direction,omitempty" bson:"direction,omitempty"`
	DayTime      string `json:"day_time,omitempty" bson:"day_time,omitempty"`
	Weather      string `json:"weather,omitempty" bson:"weather,omitempty"`
	VehicleType  string `json:"vehicle_type,omitempty" bson:"vehicle_type,omitempty"`
	VehicleColor string `json:"vehicle_color,omitempty" bson:"vehicle_color,omitempty"`
}

// normalize passa os filtros antigos para Fields/Objects e tira os vazios.
func (f *Filters) normalize() {
	set := func(m map[string]string, k, v string) map[string]string {
		if v = strings.TrimSpace(v); v == "" {
			return m
		}
		if m == nil {
			m = map[string]string{}
		}
		if _, ok := m[k]; !ok {
			m[k] = v
		}
		return m
	}
	f.Fields = set(f.Fields, "equipment", f.Equipment)
	f.Fields = set(f.Fields, "camera", f.Camera)
	f.Fields = set(f.Fields, "direction", f.Direction)
	f.Fields = set(f.Fields, "day_time", f.DayTime)
	f.Fields = set(f.Fields, "weather", f.Weather)
	if f.VehicleType != "" || f.VehicleColor != "" {
		if f.Objects == nil {
			f.Objects = map[string]map[string]string{}
		}
		v := set(f.Objects["vehicle"], "type", f.VehicleType)
		f.Objects["vehicle"] = set(v, "color", f.VehicleColor)
	}
	f.Equipment, f.Camera, f.Direction, f.DayTime, f.Weather, f.VehicleType, f.VehicleColor = "", "", "", "", "", "", ""

	clean := func(m map[string]string) map[string]string {
		out := map[string]string{}
		for k, v := range m {
			if k, v = strings.TrimSpace(k), strings.TrimSpace(v); k != "" && v != "" {
				out[k] = v
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	}
	f.Fields = clean(f.Fields)
	objs := map[string]map[string]string{}
	for t, m := range f.Objects {
		if m = clean(m); m != nil && strings.TrimSpace(t) != "" {
			objs[strings.TrimSpace(t)] = m
		}
	}
	f.Objects = objs
	if len(objs) == 0 {
		f.Objects = nil
	}
}

// Bind liga o perfil ao perfil de anotação dele (o de AnnotationProfile):
// os campos das pastas, do nome e dos filtros são os desse perfil. Precisa
// vir antes do Validate, do Dir e do FileNameOf.
func (p *Profile) Bind(ap *annotation.Profile) {
	p.ap = ap
	p.specs = fieldSpecs(ap)
}

func (p *Profile) annotationName() string {
	if p.ap != nil {
		return p.ap.Name
	}
	return p.AnnotationProfile
}

// FilterValues são os filtros no tipo de cada campo (número como float64,
// sim/não como bool), prontos para a busca de imagens. Depois do Bind e do
// Validate.
func (p *Profile) FilterValues() (fields map[string]any, objects map[string]map[string]any) {
	fields = map[string]any{}
	for k, v := range p.Filters.Fields {
		var def *annotation.Field
		if p.ap != nil {
			def, _ = p.ap.Field(k)
		}
		fields[k] = filterValue(def, v)
	}
	objects = map[string]map[string]any{}
	for t, m := range p.Filters.Objects {
		var ot *annotation.ObjectType
		if p.ap != nil {
			ot, _ = p.ap.Object(t)
		}
		objects[t] = map[string]any{}
		for k, v := range m {
			var def *annotation.Field
			if ot != nil {
				def, _ = ot.Field(k)
			}
			objects[t][k] = filterValue(def, v)
		}
	}
	return fields, objects
}

// filterValue converte o texto do filtro no tipo do campo. Um campo de texto
// com maiúsculas (a placa) é buscado em maiúsculas.
func filterValue(f *annotation.Field, s string) any {
	if f == nil {
		return s
	}
	switch f.Type {
	case annotation.TypeNumber:
		if n, err := strconv.ParseFloat(s, 64); err == nil {
			return n
		}
	case annotation.TypeBool:
		if b, err := strconv.ParseBool(s); err == nil {
			return b
		}
	case annotation.TypeText:
		if f.Upper {
			return strings.ToUpper(s)
		}
	}
	return s
}

// validateFilters confere os filtros contra o perfil de anotação.
func (p *Profile) validateFilters(add func(string, ...any)) {
	check := func(path string, f *annotation.Field, v string) {
		if f.Type == annotation.TypeDateTime {
			add("%s: não dá para filtrar por data e hora", path)
			return
		}
		val := filterValue(f, v)
		if s, ok := val.(string); ok && (f.Type == annotation.TypeNumber || f.Type == annotation.TypeBool) {
			add("%s: %q não é um valor de %s", path, s, map[string]string{annotation.TypeNumber: "número", annotation.TypeBool: "sim/não (true ou false)"}[f.Type])
			return
		}
		if f.Type == annotation.TypeMulti {
			val = []string{v}
		}
		if _, err := f.Clean(val); err != nil {
			add("%s: %v", path, err)
		}
	}
	for _, k := range sortedKeys(p.Filters.Fields) {
		f, ok := p.ap.Field(k)
		if !ok {
			add("filters.fields.%s: o perfil de anotação %s não tem este campo", k, p.ap.Name)
			continue
		}
		check("filters.fields."+k, f, p.Filters.Fields[k])
	}
	for _, t := range sortedKeys(p.Filters.Objects) {
		ot, ok := p.ap.Object(t)
		if !ok || ot.Box {
			add("filters.objects.%s: o perfil de anotação %s não tem este tipo de objeto (sem caixa)", t, p.ap.Name)
			continue
		}
		for _, k := range sortedKeys(p.Filters.Objects[t]) {
			f, ok := ot.Field(k)
			if !ok {
				add("filters.objects.%s.%s: %q não tem este campo", t, k, t)
				continue
			}
			check("filters.objects."+t+"."+k, f, p.Filters.Objects[t][k])
		}
	}
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// LabelRules: primeiro renomeia (Rename: label do CVAT -> nome final; dois
// nomes iguais juntam as classes), depois filtra (Include: nomes finais que
// entram, na ordem das classes; vazio = todas, em ordem alfabética).
type LabelRules struct {
	Rename  map[string]string `json:"rename,omitempty" bson:"rename,omitempty"`
	Include []string          `json:"include,omitempty" bson:"include,omitempty"`
}

// Builtins são os perfis padrão. Não ficam no banco e não podem ser editados
// (para mudar algo, salve uma cópia com outro nome).
func Builtins() []Profile {
	return []Profile{
		{
			Name: "rf-detr", Format: FormatCOCO, Split: Split{70, 15, 15}, Builtin: true,
			Description: "RF-DETR: COCO no layout do Roboflow (train/valid/test), 70/15/15, todas as labels e pacotes.",
		},
		{
			Name: "yolo", Format: FormatYOLO, Split: Split{70, 15, 15}, Builtin: true,
			Description: "YOLO (Ultralytics): images/, labels/ e data.yaml, 70/15/15, todas as labels e pacotes.",
		},
		{
			Name: "voc", Format: FormatVOC, Split: Split{70, 15, 15}, Layout: LayoutSplit, Builtin: true,
			Description: "Pascal VOC: train/valid/test, cada imagem com o seu .xml ao lado, 70/15/15.",
		},
		{
			Name: "voc-pastas", Format: FormatVOC, Split: Split{100, 0, 0}, Layout: LayoutFolders, Builtin: true,
			Description: "Pascal VOC em pastas: tipo de veículo / resolução / tipo de placa / direção / período (sem divisão), com o nome no padrão antigo (hash + características + placa).",
			FileName:    TemplateLegacy,
			Folders: []FolderLevel{
				{Field: "vehicle_type", Names: map[string]string{"car": "Car", "bus": "Bus", "truck": "Truck", "motorcycle": "motorCycle"}},
				{Field: "resolution"},
				{Field: "plate_type", Names: map[string]string{"br": "Br", "mercosul": "Mercosul"}},
				{Field: "direction", Names: map[string]string{"front": "Dianteiro", "rear": "Traseiro"}},
				{Field: "day_time", Names: map[string]string{"day": "Diurno", "night": "Noturno"}},
			},
		},
		{
			Name: "imagens", Format: FormatImages, Split: Split{100, 0, 0}, Layout: LayoutSplit, Builtin: true,
			Description: "Só as imagens verificadas, com o nome do SIDM (v2) e um anotacoes.csv; sem caixas, sem tarefa.",
		},
	}
}

// Builtin devolve o perfil padrão com esse nome.
func Builtin(name string) (Profile, bool) {
	for _, p := range Builtins() {
		if p.Name == name {
			return p, true
		}
	}
	return Profile{}, false
}

var profileNameRe = regexp.MustCompile(`^[A-Za-z0-9]+(-[A-Za-z0-9]+)*$`)

// ValidationError junta todos os problemas do perfil numa mensagem só.
type ValidationError struct{ Problems []string }

func (e *ValidationError) Error() string {
	return "perfil de export inválido:\n- " + strings.Join(e.Problems, "\n- ")
}

// Normalize limpa espaços e tira entradas vazias (antes de validar/salvar).
func (p *Profile) Normalize() {
	p.Name = strings.TrimSpace(p.Name)
	p.Description = strings.TrimSpace(p.Description)
	p.Format = strings.ToLower(strings.TrimSpace(p.Format))
	p.Tasks = cleanTasks(append([]string{p.Task}, p.Tasks...))
	p.Task = ""
	p.AnnotationProfile = strings.TrimSpace(p.AnnotationProfile)
	p.Filters.normalize()
	p.normalizeFolders()
	p.normalizeFileName()
	pkgs := p.Packages[:0:0]
	for _, name := range p.Packages {
		if name = strings.TrimSpace(name); name != "" {
			pkgs = append(pkgs, name)
		}
	}
	p.Packages = pkgs
	if len(p.Labels.Rename) > 0 {
		clean := make(map[string]string, len(p.Labels.Rename))
		for from, to := range p.Labels.Rename {
			from, to = strings.TrimSpace(from), strings.TrimSpace(to)
			if from != "" && to != "" && from != to {
				clean[from] = to
			}
		}
		p.Labels.Rename = clean
	}
	include := p.Labels.Include[:0:0]
	seen := map[string]bool{}
	for _, label := range p.Labels.Include {
		if label = strings.TrimSpace(label); label != "" && !seen[label] {
			seen[label] = true
			include = append(include, label)
		}
	}
	p.Labels.Include = include
	if !p.NeedsTasks() {
		// Sem caixas: tarefa e labels não se aplicam (não ficam salvos à toa).
		p.Tasks, p.AllTasks, p.Labels = nil, false, LabelRules{}
	}
}

// MaxTasks: tarefas num dataset só.
const MaxTasks = 10

// cleanTasks: sem espaços, sem vazias, sem repetidas, na ordem.
func cleanTasks(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, t := range in {
		if t = strings.TrimSpace(t); t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

// SetTasks troca as tarefas (as do pedido de export valem mais que as do
// perfil), já limpas. No formato sem caixas, não há tarefa.
func (p *Profile) SetTasks(tasks []string) {
	p.Tasks, p.Task = cleanTasks(tasks), ""
	if !p.NeedsTasks() {
		p.Tasks = nil
	}
}

// Validate confere o perfil (depois do Normalize) contra ap, o perfil de
// anotação dele (o de AnnotationProfile, ou o padrão).
func (p *Profile) Validate(ap *annotation.Profile) error {
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }
	p.Bind(ap)
	if p.AnnotationProfile != "" && p.AnnotationProfile != ap.Name {
		add("annotation_profile: %q, mas os campos foram conferidos contra %q", p.AnnotationProfile, ap.Name)
	}

	if !profileNameRe.MatchString(p.Name) {
		add(`name: use só letras, números e "-" (ex: yolo-noturnas)`)
	}
	if _, builtin := Builtin(p.Name); builtin && !p.Builtin {
		add("name: %q é um perfil padrão do sistema; use outro nome", p.Name)
	}
	if p.Format != FormatCOCO && p.Format != FormatYOLO && p.Format != FormatVOC && p.Format != FormatImages {
		add("format: use %q (RF-DETR), %q, %q (Pascal VOC) ou %q (só as imagens, sem caixas)", FormatCOCO, FormatYOLO, FormatVOC, FormatImages)
	}
	p.validateFolders(add)
	p.validateFileName(add)
	if len(p.Tasks) > MaxTasks {
		add("tasks: no máximo %d tarefas num dataset", MaxTasks)
	}
	s := p.Split
	if s.Train < 0 || s.Valid < 0 || s.Test < 0 || s.Train+s.Valid+s.Test != 100 {
		add("split: as porcentagens têm que ser positivas e somar 100 (veio %d/%d/%d)", s.Train, s.Valid, s.Test)
	} else if !p.usesSplit() {
		// pastas sem nível "split": a divisão não é usada
	} else if s.Train == 0 {
		add("split: train não pode ser 0")
	} else if p.Format == FormatYOLO && s.Valid == 0 {
		add("split: no YOLO, valid não pode ser 0 (o Ultralytics exige a validação)")
	}

	p.validateFilters(add)

	for _, label := range append(append([]string{}, p.Labels.Include...), mapValues(p.Labels.Rename)...) {
		if strings.ContainsAny(label, "\r\n\t") {
			add("labels: %q tem quebra de linha ou tab", label)
		}
	}

	if len(problems) > 0 {
		return &ValidationError{Problems: problems}
	}
	return nil
}

func mapValues(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

// SplitOf diz em que grupo uma imagem cai. É FIXO por imagem: vem do sha1
// (misturado de novo, para não depender de como o sha1 foi gerado), então a
// mesma imagem cai sempre no mesmo grupo, em qualquer export com a mesma
// divisão.
func SplitOf(sha1Hex string, s Split) string {
	sum := sha1.Sum([]byte("sidm-split:" + strings.ToLower(sha1Hex)))
	bucket := int(binary.BigEndian.Uint64(sum[:8]) % 10000) // 0..9999, granularidade de 0,01%
	switch {
	case bucket < s.Train*100:
		return SplitTrain
	case bucket < (s.Train+s.Valid)*100:
		return SplitValid
	default:
		return SplitTest
	}
}

// Splits devolve os grupos com porcentagem > 0, na ordem.
func (s Split) Splits() []string {
	var out []string
	if s.Train > 0 {
		out = append(out, SplitTrain)
	}
	if s.Valid > 0 {
		out = append(out, SplitValid)
	}
	if s.Test > 0 {
		out = append(out, SplitTest)
	}
	return out
}

// Labels traduz as labels do CVAT para as classes do dataset.
type Labels struct {
	rename  map[string]string
	include map[string]bool
	fixed   []string // ordem das classes quando o perfil diz (Include)
}

// NewLabels monta o tradutor a partir das regras do perfil.
func NewLabels(r LabelRules) *Labels {
	l := &Labels{rename: r.Rename}
	if len(r.Include) > 0 {
		l.include = make(map[string]bool, len(r.Include))
		for _, name := range r.Include {
			l.include[name] = true
		}
		l.fixed = append([]string(nil), r.Include...)
	}
	return l
}

// Map: label do CVAT -> classe final. ok=false: a caixa fica de fora.
func (l *Labels) Map(label string) (string, bool) {
	if to, renamed := l.rename[label]; renamed {
		label = to
	}
	if l.include != nil && !l.include[label] {
		return "", false
	}
	return label, true
}

// Classes: a lista final, na ordem dos ids (YOLO começa em 0, COCO em 1).
// Com Include, é a ordem do perfil; senão, ordem alfabética das que
// apareceram.
func (l *Labels) Classes(seen map[string]int) []string {
	if l.fixed != nil {
		return append([]string(nil), l.fixed...)
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
