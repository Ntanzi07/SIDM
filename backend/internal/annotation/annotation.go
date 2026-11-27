// Package annotation é o módulo dos PERFIS DE ANOTAÇÃO: o que se anota em
// cada imagem de um pacote. Um perfil tem:
//
//   - os campos da imagem (ex.: direção, clima, dia/noite);
//   - os tipos de objeto, cada um com os seus campos (ex.: veículo com
//     placa e cor; pessoa com cor do cabelo). Um tipo de objeto pode ter
//     CAIXA (é uma label do CVAT, e cada caixa ganha os campos) ou não (é
//     uma lista na imagem, como os veículos de hoje).
//
// O perfil padrão "veiculos" reproduz os campos fixos de hoje, com os mesmos
// códigos do nome v2 (ver builtin.go). A coleção do módulo é a
// annotation_profiles (só os salvos: o padrão fica no código).
//
// Este passo é só o cadastro e a validação. Os próximos ligam o perfil ao
// pacote, às imagens, ao nome do arquivo, ao import e ao export.
package annotation

import (
	"context"
	"slices"
	"time"
)

// Tipos de campo.
const (
	TypeChoice   = "choice"   // um valor de uma lista (options)
	TypeMulti    = "multi"    // vários valores da lista (options)
	TypeText     = "text"     // texto livre (pattern, max_len, upper)
	TypeNumber   = "number"   // número (min, max, integer)
	TypeBool     = "bool"     // sim/não
	TypeDateTime = "datetime" // data e hora (RFC 3339; guardada em UTC)
	TypeRef      = "ref"      // um nome do cadastro (ref: equipment ou camera)
)

// Cadastros que um campo "ref" pode usar.
const (
	RefEquipment = "equipment"
	RefCamera    = "camera"
)

// Option é um valor possível de um campo choice/multi.
type Option struct {
	Value string `json:"value" bson:"value"`                     // o que é guardado (ex.: "front")
	Label string `json:"label,omitempty" bson:"label,omitempty"` // o que aparece na tela (ex.: "Dianteira")
	// Code é a forma curta usada no nome do arquivo (ex.: "F"). Opcional,
	// mas se uma opção do campo tem, todas têm.
	Code string `json:"code,omitempty" bson:"code,omitempty"`
	// Exclusive (só em multi): a opção que diz "nenhum dos outros" (ex.:
	// "Sem avarias"). Não convive com as outras: se vier junto de alguma,
	// sai (o problema marcado ganha). No máximo uma por campo.
	Exclusive bool `json:"exclusive,omitempty" bson:"exclusive,omitempty"`
}

// Field é um campo da imagem ou de um objeto.
type Field struct {
	Key      string `json:"key" bson:"key"`                         // nome no banco e na API (ex.: "day_time")
	Label    string `json:"label,omitempty" bson:"label,omitempty"` // nome na tela (ex.: "Dia/noite")
	Type     string `json:"type" bson:"type"`
	Required bool   `json:"required,omitempty" bson:"required,omitempty"` // obrigatório para verificar
	Help     string `json:"help,omitempty" bson:"help,omitempty"`
	// Default é o valor que a tela já traz numa imagem sem rascunho (ex.:
	// "rear"). Tem que ser um valor válido do campo.
	Default any `json:"default,omitempty" bson:"default,omitempty"`
	// Repeat: na anotação, a próxima imagem pendente do pacote já vem com o
	// valor da última salva (ex.: equipamento, câmera, direção, que mudam
	// pouco de uma foto para a outra). Só nos campos da imagem.
	Repeat bool `json:"repeat,omitempty" bson:"repeat,omitempty"`

	Options []Option `json:"options,omitempty" bson:"options,omitempty"` // choice, multi

	Pattern string `json:"pattern,omitempty" bson:"pattern,omitempty"` // text: expressão regular do valor inteiro
	MaxLen  int    `json:"max_len,omitempty" bson:"max_len,omitempty"` // text
	Upper   bool   `json:"upper,omitempty" bson:"upper,omitempty"`     // text: guarda em maiúsculas (ex.: placa)

	Min     *float64 `json:"min,omitempty" bson:"min,omitempty"` // number
	Max     *float64 `json:"max,omitempty" bson:"max,omitempty"` // number
	Integer bool     `json:"integer,omitempty" bson:"integer,omitempty"`

	Ref string `json:"ref,omitempty" bson:"ref,omitempty"` // ref: equipment ou camera

	// From: o perfil de onde o campo veio (herança). Só na resposta: no
	// banco cada perfil guarda só os campos dele.
	From string `json:"from,omitempty" bson:"-"`
}

// ObjectType é um tipo de objeto que aparece na imagem.
type ObjectType struct {
	Key   string `json:"key" bson:"key"` // ex.: "vehicle"; com caixa, é a label do CVAT
	Label string `json:"label,omitempty" bson:"label,omitempty"`
	// Box: cada objeto é uma caixa (vem do CVAT). Sem caixa, é só uma lista
	// na imagem.
	Box    bool    `json:"box,omitempty" bson:"box,omitempty"`
	Min    int     `json:"min,omitempty" bson:"min,omitempty"` // quantos por imagem, no mínimo, para verificar
	Max    int     `json:"max,omitempty" bson:"max,omitempty"` // no máximo (0 = sem limite)
	Fields []Field `json:"fields" bson:"fields"`
	// BoxLabels: as labels de caixa (do CVAT) que representam um objeto
	// deste tipo, e o que cada uma já diz dele (motorcycle → vehicle_type =
	// moto). Na anotação, cada objeto pode apontar para a sua caixa.
	BoxLabels []BoxLabel `json:"box_labels,omitempty" bson:"box_labels,omitempty"`
	// From: o perfil de onde o tipo veio (herança). Um perfil filho pode
	// repetir a chave de um tipo do pai só para acrescentar campos.
	From string `json:"from,omitempty" bson:"-"`
}

// BoxLabel: uma label de caixa que representa um tipo de objeto, e os
// valores que ela implica nos campos dele.
type BoxLabel struct {
	Label  string         `json:"label" bson:"label"`
	Fields map[string]any `json:"fields,omitempty" bson:"fields,omitempty"`
	// Attributes: atributo da caixa (do CVAT) → campo do objeto que é a
	// mesma coisa (color → color). Ao ligar, o campo vazio vem do atributo;
	// depois vale o campo (o CVAT não é reescrito).
	Attributes map[string]string `json:"attributes,omitempty" bson:"attributes,omitempty"`
	From       string            `json:"from,omitempty" bson:"-"` // herdada (só na resposta)
}

// Profile é um perfil de anotação.
type Profile struct {
	Name        string `json:"name" bson:"name"`
	Description string `json:"description,omitempty" bson:"description,omitempty"`
	Builtin     bool   `json:"builtin,omitempty" bson:"-"` // padrão do código: não pode ser alterado
	// Extends: o perfil pai. O filho tem todos os campos e tipos de objeto
	// dele, mais os seus (ex.: "moto" estende "veiculos" e acrescenta
	// quantas pessoas estão na moto). Uma pesquisa do pai inclui os
	// pacotes dos filhos: os campos do pai existem em todos.
	Extends string `json:"extends,omitempty" bson:"extends,omitempty"`
	// Lineage: o próprio nome e os dos pais, do mais perto ao mais longe
	// (só na resposta).
	Lineage   []string     `json:"lineage,omitempty" bson:"-"`
	Fields    []Field      `json:"fields" bson:"fields"`
	Objects   []ObjectType `json:"objects" bson:"objects"`
	CreatedBy string       `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedAt time.Time    `json:"updated_at,omitzero" bson:"updated_at"`
}

// Is diz se o perfil é name ou estende name (direto ou não). Ex.: um perfil
// "moto" que estende "veiculos" é "veiculos" (tem os campos do nome v2).
func (p *Profile) Is(name string) bool {
	return p.Name == name || slices.Contains(p.Lineage, name)
}

// Field devolve o campo da imagem com essa chave.
func (p *Profile) Field(key string) (*Field, bool) {
	for i := range p.Fields {
		if p.Fields[i].Key == key {
			return &p.Fields[i], true
		}
	}
	return nil, false
}

// Object devolve o tipo de objeto com essa chave.
func (p *Profile) Object(key string) (*ObjectType, bool) {
	for i := range p.Objects {
		if p.Objects[i].Key == key {
			return &p.Objects[i], true
		}
	}
	return nil, false
}

// Field devolve o campo do objeto com essa chave.
func (o *ObjectType) Field(key string) (*Field, bool) {
	for i := range o.Fields {
		if o.Fields[i].Key == key {
			return &o.Fields[i], true
		}
	}
	return nil, false
}

// Object é um objeto anotado numa imagem (o valor, não o tipo).
type Object struct {
	Type   string         `json:"type" bson:"type"`
	Fields map[string]any `json:"fields" bson:"fields"`
	// Box: a caixa (do CVAT) que é este objeto, se ligada (ex.: o Veículo 1
	// é a caixa motorcycle da tarefa helmet_detection). A label tem que
	// estar nas BoxLabels do tipo.
	Box *BoxRef `json:"box,omitempty" bson:"box,omitempty"`
}

// BoxRef aponta uma caixa de uma tarefa. As caixas não têm id fixo (uma
// tarefa importada de novo troca todas): a caixa é achada pela label e pela
// posição (a mais parecida, se mudou um pouco).
type BoxRef struct {
	Task  string  `json:"task" bson:"task"`
	Label string  `json:"label" bson:"label"`
	X     float64 `json:"x" bson:"x"`
	Y     float64 `json:"y" bson:"y"`
	W     float64 `json:"w" bson:"w"`
	H     float64 `json:"h" bson:"h"`
}

// Repo é a coleção annotation_profiles.
type Repo interface {
	CreateProfile(ctx context.Context, p *Profile) error
	FindProfile(ctx context.Context, name string) (*Profile, error)
	ReplaceProfile(ctx context.Context, p *Profile) error
	DeleteProfile(ctx context.Context, name string) error
	ListProfiles(ctx context.Context) ([]Profile, error)
}

// Packages diz quais pacotes usam um perfil (o catálogo). Um perfil em uso
// não pode ser apagado: a anotação desses pacotes ficaria sem perfil.
type Packages interface {
	PackagesWithProfile(ctx context.Context, profile string) ([]string, error)
}

// InvalidError é um pedido que não dá para atender como veio (vira 400).
// Problems tem cada erro com o caminho do campo, para a tela marcar.
type InvalidError struct {
	Msg      string
	Problems []Problem
}

func (e *InvalidError) Error() string { return e.Msg }

// Problem é um erro de validação: Path diz onde (ex.: "fields[2].options",
// "objects[0].plate").
type Problem struct {
	Path string `json:"path"`
	Msg  string `json:"msg"`
}
