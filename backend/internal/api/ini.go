package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"

	"SIDM/internal/filename"
	"SIDM/internal/importer"
	"SIDM/internal/store"
)

// Coluna interna que identifica a imagem quando o INI vira uma "tabela".
const iniKeyColumn = "_sidm_sha1"

// Acima disso nem tentamos procurar INI (não é uma foto de equipamento).
const maxINIScanBytes = 64 << 20

// readEmbeddedINI lê o INI gravado depois do fim do JPEG, ou nil.
func readEmbeddedINI(path string, size int64) []importer.INIEntry {
	if size > maxINIScanBytes {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return importer.ParseINI(importer.ExtractJPEGTrailer(data))
}

// annotationFromINI aplica um perfil jpeg-ini nas entradas de UMA imagem,
// passando pelo mesmo Transform do CSV (mesmas regras de map, data, template).
func annotationFromINI(p *importer.Profile, sha string, entries []importer.INIEntry) (importer.Annotation, importer.Report) {
	run := *p
	run.Match = importer.MatchConfig{Column: iniKeyColumn, By: importer.MatchBySHA1}
	records, rep := importer.Transform(&run, importer.INITable(iniKeyColumn, sha, entries))
	if len(records) == 0 {
		return importer.Annotation{}, rep
	}
	return records[0].Annotation, rep
}

// sanitizeAndRegister: equipamento e câmera vindos do INI passam pela mesma
// sanitização do cadastro e são cadastrados se ainda não existirem (igual ao
// import de CSV). Nome impossível de sanitizar vira aviso e fica vazio.
func (s *Server) sanitizeAndRegister(ctx context.Context, equipment, camera string) (string, string, []string) {
	var warnings []string
	clean := func(value, what string,
		sanitize func(string) (string, error),
		exists func(context.Context, string) (bool, error),
		create func(context.Context, string) error,
	) string {
		if value == "" {
			return ""
		}
		name, err := sanitize(value)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s %q ignorado: %v", what, value, err))
			return ""
		}
		if ok, err := exists(ctx, name); err == nil && !ok {
			if err := create(ctx, name); err != nil && !errors.Is(err, store.ErrDuplicate) {
				warnings = append(warnings, fmt.Sprintf("não foi possível cadastrar %s %q", what, name))
				s.logError(err)
			}
		}
		return name
	}

	equipment = clean(equipment, "equipamento", filename.SanitizeEquipment, s.store.EquipmentExists, s.store.CreateEquipment)
	camera = clean(camera, "câmera", filename.SanitizeCamera, s.store.CameraExists, s.store.CreateCamera)
	return equipment, camera, warnings
}

// reportWarnings resume o relatório de uma imagem em avisos legíveis.
func reportWarnings(rep importer.Report) []string {
	var out []string
	for _, e := range rep.Errors {
		out = append(out, fmt.Sprintf("%s: %s", e.Field, e.Error))
	}
	fields := make([]string, 0, len(rep.Unmapped))
	for f := range rep.Unmapped {
		fields = append(fields, f)
	}
	sort.Strings(fields)
	for _, f := range fields {
		for v := range rep.Unmapped[f] {
			out = append(out, fmt.Sprintf("%s: valor %q sem tradução no perfil", f, v))
		}
	}
	return out
}

// iniRecords monta os "registros" do import a partir do INI JÁ GUARDADO no
// banco de cada imagem pendente do pacote. Não precisa de arquivo: serve para
// aplicar um perfil de INI criado (ou corrigido) depois do upload.
func (s *Server) iniRecords(w http.ResponseWriter, r *http.Request, pkg string, p *importer.Profile, rep *importReport) ([]importer.Record, map[string][]store.Image, bool) {
	records := []importer.Record{}
	byKey := map[string][]store.Image{}

	const pageSize = 500
	for skip := int64(0); ; skip += pageSize {
		page, _, err := s.store.ListImages(r.Context(), store.ImageFilter{
			Package: pkg, Status: store.StatusPending, Limit: pageSize, Skip: skip,
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
			ann, one := annotationFromINI(p, img.SHA1, img.INI)
			mergeTransformReport(&rep.Report, one, img.SHA1)
			records = append(records, importer.Record{Key: img.SHA1, Annotation: ann})
			byKey[img.SHA1] = []store.Image{img}
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
