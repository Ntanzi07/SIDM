package images

// O tamanho da imagem, para os campos que vêm do arquivo (FromImage "size",
// ex.: a resolução). Lê só o cabeçalho (image.DecodeConfig), não a imagem
// inteira; o arquivo nunca é alterado.

import (
	"context"
	"image"
	_ "image/jpeg" // registra os formatos que o DecodeConfig entende
	_ "image/png"
	"os"

	"SIDM/internal/annotation"
)

// measure: o tamanho do arquivo em path (zero se não der para ler).
func measure(path string) annotation.ImageSize {
	f, err := os.Open(path)
	if err != nil {
		return annotation.ImageSize{}
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return annotation.ImageSize{}
	}
	return annotation.ImageSize{Width: cfg.Width, Height: cfg.Height}
}

// sizeOf: o tamanho de uma imagem já guardada.
func (s *Service) sizeOf(img *Image) annotation.ImageSize {
	return measure(s.files.Abs(img.Path))
}

// usesImage: o perfil tem algum campo que vem do arquivo?
func usesImage(p *annotation.Profile) bool {
	for _, f := range p.Fields {
		if f.FromImage != "" {
			return true
		}
	}
	return false
}

// presented: a imagem como vai para a tela. Numa pendente, os campos que vêm
// do arquivo já aparecem medidos (o rascunho guardado pode ser de antes da
// regra). Só na resposta: grava quando alguém salvar. Na verificada fica o
// que está guardado (é o que o export usa).
func (s *Service) presented(ctx context.Context, img *Image) *Image {
	if img.Status == StatusVerified {
		return img
	}
	p, err := s.ProfileOf(ctx, img.Package)
	if err != nil || !usesImage(p) {
		return img
	}
	out := *img
	fields := make(map[string]any, len(img.Fields)+1)
	for k, v := range img.Fields {
		fields[k] = v
	}
	out.Fields, _ = p.FillFromImage(fields, s.sizeOf(img))
	return &out
}
