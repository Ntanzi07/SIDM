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

	Options []Option `json:"options,omitempty" bson:"options,omitempty"` // choice, multi

	Pattern string `json:"pattern,omitempty" bson:"pattern,omitempty"` // text: expressão regular do valor inteiro
	MaxLen  int    `json:"max_len,omitempty" bson:"max_len,omitempty"` // text
	Upper   bool   `json:"upper,omitempty" bson:"upper,omitempty"`     // text: guarda em maiúsculas (ex.: placa)

	Min     *float64 `json:"min,omitempty" bson:"min,omitempty"` // number
	Max     *float64 `json:"max,omitempty" bson:"max,omitempty"` // number
	Integer bool     `json:"integer,omitempty" bson:"integer,omitempty"`

	Ref string `json:"ref,omitempty" bson:"ref,omitempty"` // ref: equipment ou camera
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
}

// Profile é um perfil de anotação.
type Profile struct {
	Name        string       `json:"name" bson:"name"`
	Description string       `json:"description,omitempty" bson:"description,omitempty"`
	Builtin     bool         `json:"builtin,omitempty" bson:"-"` // padrão do código: não pode ser alterado
	Fields      []Field      `json:"fields" bson:"fields"`
	Objects     []ObjectType `json:"objects" bson:"objects"`
	CreatedBy   string       `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedAt   time.Time    `json:"updated_at,omitzero" bson:"updated_at"`
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
