package imports

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"SIDM/internal/filename"
	"SIDM/internal/images"
	"SIDM/internal/importer"
	"SIDM/internal/platform"
)

// MaxListedKeys é quantas chaves (não encontradas, nomes inválidos, erros)
// o relatório lista. As contagens são sempre completas.
const MaxListedKeys = 100

type Service struct {
	repo    Repo
	images  Images
	catalog Catalog
	now     func() time.Time
}

func New(repo Repo, img Images, cat Catalog) *Service {
	return &Service{repo: repo, images: img, catalog: cat, now: func() time.Time { return time.Now().UTC() }}
}

// ---------- perfis ----------

// FindProfile busca um perfil pelo nome (platform.ErrNotFound se não existe).
func (s *Service) FindProfile(ctx context.Context, name string) (*importer.Profile, error) {
	return s.repo.FindProfile(ctx, name)
}

// CreateProfile cadastra um perfil já validado (platform.ErrDuplicate se o
// nome já existe).
func (s *Service) CreateProfile(ctx context.Context, p *importer.Profile, user string) error {
	p.CreatedBy = user
	p.UpdatedAt = s.now()
	return s.repo.CreateProfile(ctx, p)
}

// ReplaceProfile grava a nova versão de um perfil existente.
func (s *Service) ReplaceProfile(ctx context.Context, p *importer.Profile) error {
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
	if err := s.repo.RenameProfile(ctx, oldName, newName, s.now()); err != nil {
		return nil, err
	}
	return s.repo.FindProfile(ctx, newName)
}

// DeleteProfile apaga um perfil. Os rascunhos que ele já preencheu ficam.
func (s *Service) DeleteProfile(ctx context.Context, name string) error {
	return s.repo.DeleteProfile(ctx, name)
}

// ListProfiles lista em ordem alfabética, com busca por nome.
func (s *Service) ListProfiles(ctx context.Context, q string, limit, skip int64) ([]importer.Profile, int64, error) {
	return s.repo.ListProfiles(ctx, q, limit, skip)
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
		NewEquipments: []string{}, NewCameras: []string{}, InvalidNames: []RowName{},
	}

	var records []importer.Record
	var byKey map[string][]images.Image
	var err error
	if p.Reader.Type == importer.ReaderJPEGINI {
		records, byKey, err = s.iniRecords(ctx, run.Package, p, rep)
	} else {
		records, byKey, err = s.csvRecords(ctx, run.Package, p, table, rep)
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

	for _, rec := range records {
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

		ann := rec.Annotation
		ann.Equipment = rep.sanitizeName(rec.Line, "equipment", ann.Equipment, filename.SanitizeEquipment)
		ann.Camera = rep.sanitizeName(rec.Line, "camera", ann.Camera, filename.SanitizeCamera)

		before := img
		if !images.MergeAnnotation(&img, ann, run.Overwrite) {
			rep.Unchanged++
			continue
		}
		if img.Equipment != "" {
			equipments[img.Equipment] = true
		}
		if img.Camera != "" {
			cameras[img.Camera] = true
		}
		writes = append(writes, write{before: before, after: img})
	}

	// 2ª passada: equipamentos e câmeras que ainda não existem.
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
	for _, wr := range writes {
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
	return rep, nil
}

// csvRecords traduz as linhas do CSV e busca as imagens de uma vez (em
// lotes), não uma consulta por linha.
func (s *Service) csvRecords(ctx context.Context, pkg string, p *importer.Profile, table *importer.Table, rep *Report) ([]importer.Record, map[string][]images.Image, error) {
	if len(table.Rows) > 0 && !hasColumn(table, p.Match.Column) {
		return nil, nil, &InvalidError{fmt.Sprintf(
			"o CSV não tem a coluna %q que o perfil usa para achar a imagem (colunas: %s)",
			p.Match.Column, strings.Join(table.Columns, ", "))}
	}

	var records []importer.Record
	records, rep.Report = importer.Transform(p, table)

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
func (s *Service) iniRecords(ctx context.Context, pkg string, p *importer.Profile, rep *Report) ([]importer.Record, map[string][]images.Image, error) {
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
