package api

import (
	"net/http"

	"SIDM/internal/images"
	"SIDM/internal/importer"
)

// Coluna interna que identifica a imagem quando o INI vira uma "tabela".

// Acima disso nem tentamos procurar INI (não é uma foto de equipamento).

// iniRecords monta os "registros" do import a partir do INI JÁ GUARDADO no
// banco de cada imagem pendente do pacote. Não precisa de arquivo: serve para
// aplicar um perfil de INI criado (ou corrigido) depois do upload.
func (s *Server) iniRecords(w http.ResponseWriter, r *http.Request, pkg string, p *importer.Profile, rep *importReport) ([]importer.Record, map[string][]images.Image, bool) {
	records := []importer.Record{}
	byKey := map[string][]images.Image{}

	const pageSize = 500
	for skip := int64(0); ; skip += pageSize {
		page, _, err := s.images.List(r.Context(), images.ImageFilter{
			Package: pkg, Status: images.StatusPending, Limit: pageSize, Skip: skip,
		})
		if err != nil {
			s.serverError(w, err)
			return nil, nil, false
		}

		for _, img := range page {
			rep.Rows++
			if len(img.INI) == 0 {
				rep.WithoutINI++
				continue
			}
			ann, one := images.AnnotationFromINI(p, img.SHA1, img.INI)
			mergeTransformReport(&rep.Report, one, img.SHA1)
			records = append(records, importer.Record{Key: img.SHA1, Annotation: ann})
			byKey[img.SHA1] = []images.Image{img}
		}
		if len(page) < pageSize {
			break
		}
	}
	rep.Records = len(records)
	return records, byKey, true
}

// mergeTransformReport soma o relatório de UMA imagem no relatório geral.
func mergeTransformReport(dst *importer.Report, src importer.Report, key string) {
	for field, values := range src.Unmapped {
		if dst.Unmapped[field] == nil {
			dst.Unmapped[field] = map[string]int{}
		}
		for v, n := range values {
			dst.Unmapped[field][v] += n
		}
	}
	for _, e := range src.Errors {
		dst.ErrorCount++
		if len(dst.Errors) < maxListedKeys {
			e.Key, e.Line = key, 0
			dst.Errors = append(dst.Errors, e)
		}
	}
}
