// Package export monta datasets de treino (RF-DETR, YOLO) a partir das
// imagens e das caixas guardadas no SIDM. Sem banco e sem HTTP: quem chama
// entrega as imagens, e este pacote decide a divisão, traduz as labels e
// escreve o zip.
package export

import (
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"SIDM/internal/filename"
)

// Formatos de saída.
const (
	FormatCOCO = "coco" // layout do Roboflow, o que o RF-DETR lê: train/valid/test com _annotations.coco.json
	FormatYOLO = "yolo" // Ultralytics: images/, labels/ e data.yaml
)

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
	// Task: a tarefa de anotação cujas caixas entram. Vazio = escolher na
	// hora de exportar (os perfis padrão servem para qualquer tarefa).
	Task string `json:"task,omitempty" bson:"task,omitempty"`

	// Packages: quais pacotes entram (vazio = todos).
	Packages []string `json:"packages,omitempty" bson:"packages,omitempty"`
	// Filters: só entram as imagens com estes metadados (vazio = qualquer).
	Filters Filters `json:"filters" bson:"filters"`
	// Labels: quais classes entram e com que nome.
	Labels LabelRules `json:"labels" bson:"labels"`

	Builtin   bool      `json:"builtin,omitempty" bson:"-"` // perfil padrão do sistema (não dá para editar)
	CreatedBy string    `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitzero" bson:"updated_at,omitempty"`
}

// Split em porcentagens (somam 100). A divisão é FIXA por imagem: cada
// imagem cai sempre no mesmo grupo (decidido pelo sha1), então re-exportar
// com imagens novas não muda o grupo das antigas.
type Split struct {
	Train int `json:"train" bson:"train"`
	Valid int `json:"valid" bson:"valid"`
	Test  int `json:"test" bson:"test"`
}

// Filters usam os mesmos valores da anotação ("night", "rain"...).
type Filters struct {
	Equipment    string `json:"equipment,omitempty" bson:"equipment,omitempty"`
	Camera       string `json:"camera,omitempty" bson:"camera,omitempty"`
	Direction    string `json:"direction,omitempty" bson:"direction,omitempty"`
	DayTime      string `json:"day_time,omitempty" bson:"day_time,omitempty"`
	Weather      string `json:"weather,omitempty" bson:"weather,omitempty"`
	VehicleType  string `json:"vehicle_type,omitempty" bson:"vehicle_type,omitempty"`
	VehicleColor string `json:"vehicle_color,omitempty" bson:"vehicle_color,omitempty"`
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
	p.Task = strings.TrimSpace(p.Task)
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
}

// Validate confere o perfil (depois do Normalize).
func (p *Profile) Validate() error {
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }

	if !profileNameRe.MatchString(p.Name) {
		add(`name: use só letras, números e "-" (ex: yolo-noturnas)`)
	}
	if _, builtin := Builtin(p.Name); builtin && !p.Builtin {
		add("name: %q é um perfil padrão do sistema; use outro nome", p.Name)
	}
	if p.Format != FormatCOCO && p.Format != FormatYOLO {
		add("format: use %q (RF-DETR) ou %q", FormatCOCO, FormatYOLO)
	}
	s := p.Split
	if s.Train < 0 || s.Valid < 0 || s.Test < 0 || s.Train+s.Valid+s.Test != 100 {
		add("split: as porcentagens têm que ser positivas e somar 100 (veio %d/%d/%d)", s.Train, s.Valid, s.Test)
	} else if s.Train == 0 {
		add("split: train não pode ser 0")
	} else if p.Format == FormatYOLO && s.Valid == 0 {
		add("split: no YOLO, valid não pode ser 0 (o Ultralytics exige a validação)")
	}

	codes := filename.Codes()
	check := func(field, table, value string) {
		if value == "" {
			return
		}
		for _, v := range codes[table] {
			if v == value {
				return
			}
		}
		add("filters.%s: valor desconhecido %q", field, value)
	}
	f := p.Filters
	check("direction", "direction", f.Direction)
	check("day_time", "day_time", f.DayTime)
	check("weather", "weather", f.Weather)
	check("vehicle_type", "vehicle_type", f.VehicleType)
	check("vehicle_color", "vehicle_color", f.VehicleColor)

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
