package annotation

// Os campos que vêm do próprio arquivo da imagem (Field.FromImage): quem
// anota não escolhe; o servidor mede a imagem e preenche ao salvar.

import "fmt"

// FromImageSize: "<largura>x<altura>", como as opções da resolução ("1920x1080").
const FromImageSize = "size"

// ImageSize é o tamanho medido do arquivo (0 = não deu para medir).
type ImageSize struct{ Width, Height int }

func (s ImageSize) String() string { return fmt.Sprintf("%dx%d", s.Width, s.Height) }

// FillFromImage põe nos campos da imagem com FromImage o valor medido,
// por cima do que veio. Num choice, um tamanho que não é opção deixa o campo
// vazio e volta como aviso (o perfil precisa ganhar a opção). Sem tamanho
// (não deu para medir), os campos ficam como vieram.
func (p *Profile) FillFromImage(fields map[string]any, size ImageSize) (map[string]any, []string) {
	if size.Width <= 0 || size.Height <= 0 {
		return fields, nil
	}
	var warnings []string
	out := fields
	for _, f := range p.Fields {
		if f.FromImage != FromImageSize {
			continue
		}
		if out == nil {
			out = map[string]any{}
		}
		value := size.String()
		if f.Type == TypeChoice && !f.hasOption(value) {
			delete(out, f.Key)
			warnings = append(warnings, fmt.Sprintf("%s: a imagem tem %s, que não está entre as opções do perfil", labelOrKey(f), value))
			continue
		}
		out[f.Key] = value
	}
	return out, warnings
}

func labelOrKey(f Field) string {
	if f.Label != "" {
		return f.Label
	}
	return f.Key
}
