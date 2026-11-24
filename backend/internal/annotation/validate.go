package annotation

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	nameRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,47}$`)   // nome do perfil (vai na URL)
	keyRe    = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)      // chave de campo/objeto (vai no banco)
	valueRe  = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,47}$`) // valor de opção (ex.: "front", "640x480")
	codeRe   = regexp.MustCompile(`^[A-Z0-9]{1,8}$`)             // código no nome do arquivo
	reserved = map[string]bool{"sha1": true, "package": true, "status": true, "objects": true}
)

// ValidName diz se o nome serve para um perfil.
func ValidName(name string) bool { return nameRe.MatchString(name) }

// problems junta os erros de validação.
type problems []Problem

func (ps *problems) add(path, format string, args ...any) {
	*ps = append(*ps, Problem{Path: path, Msg: fmt.Sprintf(format, args...)})
}

// Normalize limpa o que veio de fora (espaços, maiúsculas nos códigos).
func (p *Profile) Normalize() {
	p.Name = strings.TrimSpace(p.Name)
	p.Description = strings.TrimSpace(p.Description)
	if p.Fields == nil {
		p.Fields = []Field{}
	}
	if p.Objects == nil {
		p.Objects = []ObjectType{}
	}
	normFields(p.Fields)
	for i := range p.Objects {
		o := &p.Objects[i]
		o.Key, o.Label = strings.TrimSpace(o.Key), strings.TrimSpace(o.Label)
		if o.Fields == nil {
			o.Fields = []Field{}
		}
		normFields(o.Fields)
	}
}

func normFields(fs []Field) {
	for i := range fs {
		f := &fs[i]
		f.Key, f.Label, f.Type = strings.TrimSpace(f.Key), strings.TrimSpace(f.Label), strings.TrimSpace(f.Type)
		for j := range f.Options {
			o := &f.Options[j]
			o.Value, o.Label = strings.TrimSpace(o.Value), strings.TrimSpace(o.Label)
			o.Code = strings.ToUpper(strings.TrimSpace(o.Code))
		}
	}
}

// Validate confere o perfil inteiro e devolve TODOS os erros juntos (um
// *InvalidError), para a pessoa corrigir de uma vez.
func (p *Profile) Validate() error {
	var ps problems
	if !ValidName(p.Name) {
		ps.add("name", `use só letras minúsculas, números e "-" (até 48), ex.: "capacete-moto"`)
	}
	validFields(&ps, "fields", p.Fields)

	seen := map[string]bool{}
	for i, o := range p.Objects {
		path := fmt.Sprintf("objects[%d]", i)
		switch {
		case !keyRe.MatchString(o.Key):
			ps.add(path+".key", "use letras minúsculas, números e _ (começando por letra), ex.: \"vehicle\"")
		case seen[o.Key]:
			ps.add(path+".key", "tipo de objeto %q repetido", o.Key)
		}
		seen[o.Key] = true
		if o.Min < 0 || o.Max < 0 || (o.Max > 0 && o.Min > o.Max) {
			ps.add(path, "min e max: números ≥ 0, com min ≤ max (max 0 = sem limite)")
		}
		validFields(&ps, path+".fields", o.Fields)
		for j, f := range o.Fields {
			if f.Repeat {
				ps.add(fmt.Sprintf("%s.fields[%d].repeat", path, j), "repeat só vale nos campos da imagem")
			}
		}
	}
	if len(ps) > 0 {
		return &InvalidError{Msg: summary(ps), Problems: ps}
	}
	return nil
}

// summary: a primeira mensagem, e quantas mais há.
func summary(ps problems) string {
	msg := ps[0].Path + ": " + ps[0].Msg
	if len(ps) > 1 {
		msg += fmt.Sprintf(" (e mais %d)", len(ps)-1)
	}
	return msg
}

func validFields(ps *problems, base string, fs []Field) {
	seen := map[string]bool{}
	for i, f := range fs {
		path := fmt.Sprintf("%s[%d]", base, i)
		switch {
		case !keyRe.MatchString(f.Key):
			ps.add(path+".key", "use letras minúsculas, números e _ (começando por letra), ex.: \"day_time\"")
		case reserved[f.Key]:
			ps.add(path+".key", "%q é um nome reservado", f.Key)
		case seen[f.Key]:
			ps.add(path+".key", "campo %q repetido", f.Key)
		}
		seen[f.Key] = true

		hasOptions := f.Type == TypeChoice || f.Type == TypeMulti
		if !hasOptions && len(f.Options) > 0 {
			ps.add(path+".options", "options só vale para choice e multi")
		}
		if f.Type != TypeText && (f.Pattern != "" || f.MaxLen != 0 || f.Upper) {
			ps.add(path, "pattern, max_len e upper só valem para text")
		}
		if f.Type != TypeNumber && (f.Min != nil || f.Max != nil || f.Integer) {
			ps.add(path, "min, max e integer só valem para number")
		}
		if f.Type != TypeRef && f.Ref != "" {
			ps.add(path+".ref", "ref só vale para o tipo ref")
		}

		if f.Default != nil && f.Type != TypeRef {
			if _, err := f.Clean(f.Default); err != nil {
				ps.add(path+".default", "valor inicial inválido: %v", err)
			}
		} else if f.Default != nil {
			ps.add(path+".default", "um campo ref não tem valor inicial")
		}

		switch f.Type {
		case TypeChoice, TypeMulti:
			validOptions(ps, path+".options", f.Options)
		case TypeText:
			if f.Pattern != "" {
				if _, err := regexp.Compile(f.Pattern); err != nil {
					ps.add(path+".pattern", "expressão regular inválida: %v", err)
				}
			}
			if f.MaxLen < 0 {
				ps.add(path+".max_len", "não pode ser negativo")
			}
		case TypeNumber:
			if f.Min != nil && f.Max != nil && *f.Min > *f.Max {
				ps.add(path, "min maior que max")
			}
		case TypeRef:
			switch {
			case f.Ref != RefEquipment && f.Ref != RefCamera:
				ps.add(path+".ref", "use %q ou %q", RefEquipment, RefCamera)
			case f.Key != f.Ref:
				// A cascata de renomear um equipamento procura o campo pelo
				// nome, em qualquer perfil: por isso o nome é fixo.
				ps.add(path+".key", "um campo ref %s precisa se chamar %q", f.Ref, f.Ref)
			}
		case TypeBool, TypeDateTime:
		default:
			ps.add(path+".type", "tipo %q não existe: use choice, multi, text, number, bool, datetime ou ref", f.Type)
		}
	}
}

func validOptions(ps *problems, path string, opts []Option) {
	if len(opts) == 0 {
		ps.add(path, "faltou a lista de opções")
		return
	}
	values, codes, withCode := map[string]bool{}, map[string]bool{}, 0
	for j, o := range opts {
		op := fmt.Sprintf("%s[%d]", path, j)
		switch {
		case !valueRe.MatchString(o.Value):
			ps.add(op+".value", "use letras minúsculas, números, _ . - (até 48), ex.: \"front\"")
		case values[o.Value]:
			ps.add(op+".value", "valor %q repetido", o.Value)
		}
		values[o.Value] = true
		if o.Code == "" {
			continue
		}
		withCode++
		switch {
		case !codeRe.MatchString(o.Code):
			ps.add(op+".code", "use letras maiúsculas e números (até 8), ex.: \"F\"")
		case codes[o.Code]:
			ps.add(op+".code", "código %q repetido", o.Code)
		}
		codes[o.Code] = true
	}
	if withCode > 0 && withCode < len(opts) {
		ps.add(path, "se uma opção tem código, todas precisam ter (o código vai no nome do arquivo)")
	}
}
