package imports

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"SIDM/internal/annotation"
	"SIDM/internal/filename"
	"SIDM/internal/images"
	"SIDM/internal/importer"
	"SIDM/internal/platform"
)

// MaxListedKeys é quantas chaves (não encontradas, nomes inválidos, erros)
// o relatório lista. As contagens são sempre completas.
const MaxListedKeys = 100

type Service struct {
	repo       Repo
	images     Images
	catalog    Catalog
	annotation Annotation
	now        func() time.Time
}

func New(repo Repo, img Images, cat Catalog, ann Annotation) *Service {
	return &Service{repo: repo, images: img, catalog: cat, annotation: ann, now: func() time.Time { return time.Now().UTC() }}
}

// Validate normaliza (o formato antigo, vehicles, vira objects.vehicle) e
// confere o perfil contra o perfil de anotação que ele preenche.
func (s *Service) Validate(ctx context.Context, p *importer.Profile) error {
	p.Normalize()
	ap, err := s.annotation.FindProfile(ctx, p.AnnotationProfile)
	if errors.Is(err, platform.ErrNotFound) {
		ap = nil // o Validate diz que não existe, junto com os outros erros
	} else if err != nil {
		return err
	}
	if err := p.Validate(ap); err != nil {
		return &InvalidError{err.Error()}
	}
	return nil
}

// ---------- perfis ----------

// FindProfile busca um perfil pelo nome (platform.ErrNotFound se não existe).
// Os perfis padrão (importer.Builtins) vêm do código.
func (s *Service) FindProfile(ctx context.Context, name string) (*importer.Profile, error) {
	return Profiles{s.repo}.FindProfile(ctx, name)
}

// Profiles busca perfis (padrão primeiro, depois o banco) para quem não tem
// o Service: o upload de imagens é montado antes dele.
type Profiles struct{ Repo Repo }

func (ps Profiles) FindProfile(ctx context.Context, name string) (*importer.Profile, error) {
	if p, ok := importer.Builtin(name); ok {
		return &p, nil
	}
	return ps.Repo.FindProfile(ctx, name)
}

// builtinError: perfil padrão não se edita, renomeia nem apaga.
func builtinError(name string) error {
	if _, ok := importer.Builtin(name); ok {
		return &InvalidError{fmt.Sprintf("%q é um perfil padrão do sistema e não pode ser alterado; copie com outro nome", name)}
	}
	return nil
}

// CreateProfile cadastra um perfil já validado (platform.ErrDuplicate se o
// nome já existe).
func (s *Service) CreateProfile(ctx context.Context, p *importer.Profile, user string) error {
	if err := builtinError(p.Name); err != nil {
		return err
	}
	p.CreatedBy = user
	p.UpdatedAt = s.now()
	return s.repo.CreateProfile(ctx, p)
}

// ReplaceProfile grava a nova versão de um perfil existente.
func (s *Service) ReplaceProfile(ctx context.Context, p *importer.Profile) error {
	if err := builtinError(p.Name); err != nil {
		return err
	}
	p.UpdatedAt = s.now()
	return s.repo.ReplaceProfile(ctx, p)
}

// RenameProfile troca o nome e devolve o perfil como ficou. Nenhuma imagem
// guarda o nome do perfil, então não há cascata.
func (s *Service) RenameProfile(ctx context.Context, oldName, newName string) (*importer.Profile, error) {
	newName = strings.TrimSpace(newName)
	if !importer.ValidProfileName(newName) {
		return nil, &InvalidError{`name: use só letras, números e "-" (ex: csv-equipamento-8003)`}
	}
	if newName == oldName {
		return nil, &InvalidError{"o nome novo é igual ao atual"}
	}
	for _, name := range []string{oldName, newName} {
		if err := builtinError(name); err != nil {
			return nil, err
		}
	}
	if err := s.repo.RenameProfile(ctx, oldName, newName, s.now()); err != nil {
		return nil, err
	}
	return s.repo.FindProfile(ctx, newName)
}

// DeleteProfile apaga um perfil. Os rascunhos que ele já preencheu ficam.
func (s *Service) DeleteProfile(ctx context.Context, name string) error {
	if err := builtinError(name); err != nil {
		return err
	}
	return s.repo.DeleteProfile(ctx, name)
}

// ListProfiles lista os perfis padrão primeiro e depois os do banco, em ordem
// alfabética, com busca por nome. Um perfil salvo com o nome de um padrão
// (de antes de ele virar padrão) fica escondido: vale o do código.
func (s *Service) ListProfiles(ctx context.Context, q string, limit, skip int64) ([]importer.Profile, int64, error) {
	builtins := []importer.Profile{}
	for _, p := range importer.Builtins() {
		if strings.Contains(strings.ToLower(p.Name), strings.ToLower(q)) {
			builtins = append(builtins, p)
		}
	}
	nb := int64(len(builtins))
	out := []importer.Profile{}
	if skip < nb {
		out = append(out, builtins[skip:min(nb, skip+limit)]...)
	}
	savedSkip := max(0, skip-nb)
	saved, total, err := s.repo.ListProfiles(ctx, q, limit+nb, savedSkip) // +nb: os escondidos
	if err != nil {
		return nil, 0, err
	}
	for _, p := range saved {
		if _, shadowed := importer.Builtin(p.Name); shadowed {
			total--
			continue
		}
		if int64(len(out)) < limit {
			out = append(out, p)
		}
	}
	return out, total + nb, nil
}

// ---------- import ----------

// Import casa cada registro com uma imagem do pacote e preenche o RASCUNHO.
// Nunca verifica: uma pessoa continua conferindo pela fila.
//
// Com perfil de CSV, table é o arquivo lido. Com perfil de INI, table é nil:
// os registros saem do INI já guardado de cada imagem pendente do pacote
// (serve para aplicar um perfil criado ou corrigido depois do upload).
//
// São três passadas: decidir o que muda (sem gravar), cadastrar
// equipamentos e câmeras novos, e gravar as imagens. No ensaio (DryRun) só
// a primeira roda de verdade.
func (s *Service) Import(ctx context.Context, run Run, table *importer.Table) (*Report, error) {
	p := run.Profile
	rep := &Report{
		DryRun: run.DryRun, Package: run.Package, Profile: p.Name, Overwrite: run.Overwrite,
		Report:   importer.Report{Unmapped: map[string]map[string]int{}, Errors: []importer.RowError{}},
		NotFound: []string{}, Ambiguous: []string{},
		NewEquipments: []string{}, NewCameras: []string{}, InvalidNames: []RowName{}, OutsideProfile: []string{},
	}
	annot, err := s.images.ProfileOf(ctx, run.Package)
	if err != nil {
		return nil, err
	}
	// O Transform usa os campos do perfil do PACOTE (normalmente o mesmo do
	// perfil de import): o que ele não tem fica de fora e é listado.
	p.Normalize()

	run.step(PhaseReading, 0, 0)
	var records []importer.Record
	var byKey map[string][]images.Image
	if p.Reader.Type == importer.ReaderJPEGINI {
		records, byKey, err = s.iniRecords(ctx, run.Package, p, annot, rep)
	} else {
		records, byKey, err = s.csvRecords(ctx, run.Package, p, annot, table, rep)
	}
	if err != nil {
		return nil, err
	}

	changedBy := fmt.Sprintf("%s via import:%s", run.User, p.Name)
	now := s.now()
	equipments, cameras := map[string]bool{}, map[string]bool{}

	// 1ª passada: decide o que muda em cada imagem, SEM gravar nada.
	type write struct{ before, after images.Image }
	var writes []write

	for i, rec := range records {
		if i%progressEvery == 0 {
			run.step(PhaseChecking, i, len(records))
		}
		found := byKey[rec.Key]
		switch {
		case len(found) == 0:
			rep.NotFoundCount++
			if len(rep.NotFound) < MaxListedKeys {
				rep.NotFound = append(rep.NotFound, rec.Key)
			}
			continue
		case len(found) > 1:
			rep.Ambiguous = append(rep.Ambiguous, rec.Key)
			continue
		}
		rep.Matched++
		img := found[0]

		if img.Status == images.StatusVerified {
			rep.SkippedVerified++
			continue
		}
		if img.ClaimedByOther(run.User, now) {
			rep.SkippedClaimed++
			continue
		}

		vals := rec.Values
		refs := refFields(annot)
		for key, ref := range refs {
			if v, _ := vals.Fields[key].(string); v != "" {
				sanitize := filename.SanitizeEquipment
				if ref == annotation.RefCamera {
					sanitize = filename.SanitizeCamera
				}
				if name := rep.sanitizeName(rec.Line, key, v, sanitize); name != "" {
					vals.Fields[key] = name
				} else {
					delete(vals.Fields, key)
				}
			}
		}

		// Encaixa no perfil do pacote: o que ele não tem fica de fora.
		fit, outside := annot.Fit(vals)
		for _, o := range outside {
			rep.OutsideProfileCount++
			if len(rep.OutsideProfile) < MaxListedKeys {
				where := fmt.Sprintf("linha %d", rec.Line)
				if rec.Line == 0 { // INI: não tem linha, tem a imagem
					where = rec.Key
				}
				rep.OutsideProfile = append(rep.OutsideProfile, where+": "+o)
			}
		}

		before := img
		if !images.MergeAnnotation(&img, fit, run.Overwrite) {
			rep.Unchanged++
			continue
		}
		for key, ref := range refs {
			if v, _ := img.Fields[key].(string); v != "" && fit.Fields[key] == v {
				if ref == annotation.RefCamera {
					cameras[v] = true
				} else {
					equipments[v] = true
				}
			}
		}
		writes = append(writes, write{before: before, after: img})
	}

	run.step(PhaseChecking, len(records), len(records))

	// 2ª passada: equipamentos e câmeras que ainda não existem.
	run.step(PhaseRegistry, 0, 0)
	// Ao aplicar, são criados ANTES das imagens, para que nenhuma imagem
	// fique apontando para um cadastro que não existe.
	if rep.NewEquipments, err = s.registerMissing(ctx, sortedSet(equipments), run.DryRun,
		s.catalog.EquipmentExists, s.catalog.CreateEquipment); err != nil {
		return nil, err
	}
	if rep.NewCameras, err = s.registerMissing(ctx, sortedSet(cameras), run.DryRun,
		s.catalog.CameraExists, s.catalog.CreateCamera); err != nil {
		return nil, err
	}

	if run.DryRun {
		rep.Applied = len(writes)
		return rep, nil
	}

	// 3ª passada: grava as imagens.
	for i, wr := range writes {
		if i%progressEvery == 0 {
			run.step(PhaseWriting, i, len(writes))
		}
		img := wr.after
		img.UpdatedAt = now
		if err := s.images.Replace(ctx, &img, wr.before.UpdatedAt); err != nil {
			if errors.Is(err, images.ErrConflict) {
				rep.Conflicts++
				continue
			}
			return nil, err
		}
		rep.Applied++
		before := wr.before
		if err := s.images.InsertHistory(ctx, images.DiffHistory(&before, &img, changedBy)); err != nil {
			platform.LogError(err)
		}
	}
	run.step(PhaseWriting, len(writes), len(writes))
	return rep, nil
}

// csvRecords traduz as linhas do CSV e busca as imagens de uma vez (em
// lotes), não uma consulta por linha.
func (s *Service) csvRecords(ctx context.Context, pkg string, p *importer.Profile, annot *annotation.Profile, table *importer.Table, rep *Report) ([]importer.Record, map[string][]images.Image, error) {
	if len(table.Rows) > 0 && !hasColumn(table, p.Match.Column) {
		return nil, nil, &InvalidError{fmt.Sprintf(
			"o CSV não tem a coluna %q que o perfil usa para achar a imagem (colunas: %s)",
			p.Match.Column, strings.Join(table.Columns, ", "))}
	}

	var records []importer.Record
	records, rep.Report = importer.Transform(p, annot, table)

	keys := make([]string, len(records))
	for i, rec := range records {
		keys[i] = rec.Key
	}
	imgs, err := s.images.FindByKeys(ctx, pkg, p.Match.By, keys)
	if err != nil {
		return nil, nil, err
	}
	byKey := map[string][]images.Image{}
	for _, img := range imgs {
		k := img.OriginalName
		if p.Match.By == importer.MatchBySHA1 {
			k = img.SHA1
		}
		byKey[k] = append(byKey[k], img)
	}
	return records, byKey, nil
}

// iniRecords monta os registros a partir do INI JÁ GUARDADO no banco de cada
// imagem pendente do pacote. Não precisa de arquivo.
func (s *Service) iniRecords(ctx context.Context, pkg string, p *importer.Profile, annot *annotation.Profile, rep *Report) ([]importer.Record, map[string][]images.Image, error) {
	records := []importer.Record{}
	byKey := map[string][]images.Image{}

	const pageSize = 500
	for skip := int64(0); ; skip += pageSize {
		page, _, err := s.images.List(ctx, images.ImageFilter{
			Package: pkg, Status: images.StatusPending, Limit: pageSize, Skip: skip,
		})
		if err != nil {
			return nil, nil, err
		}

		for _, img := range page {
			rep.Rows++
			if len(img.INI) == 0 {
				rep.WithoutINI++
				continue
			}
			vals, one := images.AnnotationFromINI(p, annot, img.SHA1, img.INI)
			mergeTransformReport(&rep.Report, one, img.SHA1)
			records = append(records, importer.Record{Key: img.SHA1, Values: vals})
			byKey[img.SHA1] = []images.Image{img}
		}
		if len(page) < pageSize {
			break
		}
	}
	rep.Records = len(records)
	return records, byKey, nil
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
		if len(dst.Errors) < MaxListedKeys {
			e.Key, e.Line = key, 0
			dst.Errors = append(dst.Errors, e)
		}
	}
}

// sanitizeName passa o nome de equipamento/câmera do CSV pela mesma
// sanitização do cadastro manual ("8003 MF4739" -> "8003-MF4739").
// Nome impossível de sanitizar: anota no relatório e devolve "".
func (rep *Report) sanitizeName(line int, field, value string, sanitize func(string) (string, error)) string {
	if value == "" {
		return ""
	}
	name, err := sanitize(value)
	if err != nil {
		if len(rep.InvalidNames) < MaxListedKeys {
			rep.InvalidNames = append(rep.InvalidNames, RowName{Line: line, Field: field, Value: value, Error: err.Error()})
		}
		return ""
	}
	return name
}

// registerMissing devolve os nomes que ainda não estão cadastrados e, fora
// do ensaio, cadastra cada um. ErrDuplicate é ignorado: outra pessoa pode
// ter cadastrado o mesmo nome no meio do import, e o resultado é o mesmo.
func (s *Service) registerMissing(
	ctx context.Context, names []string, dryRun bool,
	exists func(ctx context.Context, name string) (bool, error),
	create func(ctx context.Context, name string) error,
) ([]string, error) {
	missing := []string{}
	for _, name := range names {
		ok, err := exists(ctx, name)
		if err != nil {
			return nil, err
		}
		if ok {
			continue
		}
		missing = append(missing, name)
		if dryRun {
			continue
		}
		if err := create(ctx, name); err != nil && !errors.Is(err, platform.ErrDuplicate) {
			return nil, err
		}
	}
	return missing, nil
}

// refFields: os campos de cadastro da imagem (chave -> equipment/camera).
func refFields(p *annotation.Profile) map[string]string {
	out := map[string]string{}
	for _, f := range p.Fields {
		if f.Type == annotation.TypeRef {
			out[f.Key] = f.Ref
		}
	}
	return out
}

func hasColumn(t *importer.Table, col string) bool {
	for _, c := range t.Columns {
		if c == col {
			return true
		}
	}
	return false
}

func sortedSet(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
