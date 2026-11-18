package images

import (
	"SIDM/internal/annotation"
	"SIDM/internal/boxes"
	"SIDM/internal/catalog"
	"SIDM/internal/filename"
	"SIDM/internal/files"
	"SIDM/internal/importer"
	"SIDM/internal/platform"
	"context"
	"errors"
	"fmt"
	"maps"
	"time"
)

// Service são as regras do módulo. Ele também é o lado das imagens do que os
// outros módulos pedem: catalog.Images e boxes.Images (ver o fim do arquivo).
type Service struct {
	repo      Repo
	files     *files.Storage
	catalog   Catalog
	profiles  ImportProfiles
	annots    Annotations
	listeners []Listener
	now       func() time.Time
}

// New monta o módulo. catalog e profiles podem ser nil nos testes que não
// fazem upload nem verificação; annots nil usa só o perfil padrão.
func New(repo Repo, fs *files.Storage, cat Catalog, profiles ImportProfiles, annots Annotations) *Service {
	return &Service{repo: repo, files: fs, catalog: cat, profiles: profiles, annots: annots,
		now: func() time.Time { return time.Now().UTC() }}
}

// ProfileOf devolve o perfil de anotação do pacote.
func (s *Service) ProfileOf(ctx context.Context, pkg string) (*annotation.Profile, error) {
	p, err := s.catalog.FindPackage(ctx, pkg)
	if err != nil {
		return nil, err
	}
	return s.profileNamed(ctx, p.Profile)
}

func (s *Service) profileNamed(ctx context.Context, name string) (*annotation.Profile, error) {
	if b, ok := annotation.Builtin(name); ok || name == "" {
		if !ok {
			b, _ = annotation.Builtin(annotation.DefaultProfile)
		}
		return &b, nil
	}
	if s.annots == nil {
		return nil, fmt.Errorf("perfil de anotação %q: sem o módulo de perfis", name)
	}
	return s.annots.FindProfile(ctx, name)
}

// SetCatalog liga o catálogo depois de criado (o catálogo também depende
// deste módulo: o main cria os dois e depois liga).
func (s *Service) SetCatalog(cat Catalog) { s.catalog = cat }

// Listen acrescenta quem quer ser avisado quando uma imagem é apagada.
func (s *Service) Listen(l Listener) { s.listeners = append(s.listeners, l) }

// imageDeleted avisa os outros módulos (ex.: as caixas da imagem).
func (s *Service) imageDeleted(ctx context.Context, sha1 string) error {
	for _, l := range s.listeners {
		if err := l.DeleteImage(ctx, sha1); err != nil {
			return err
		}
	}
	return nil
}

var (
	_ catalog.Images = (*Service)(nil)
	_ boxes.Images   = (*Service)(nil)
)

// Limite de um request de upload (várias imagens juntas).
const maxUploadBytes = 5 << 30

// Quantos hashes podem ser consultados por request.
const MaxExistsBatch = 1000

// Quanto tempo uma pessoa "segura" uma imagem sem mexer nela.
// Salvar rascunho renova o prazo; depois disso, a imagem volta para a fila.
const claimLease = 15 * time.Minute

func toRefs(imgs []Image) []boxes.ImageRef {
	out := make([]boxes.ImageRef, len(imgs))
	for i, img := range imgs {
		out[i] = boxes.ImageRef{SHA1: img.SHA1, Package: img.Package, OriginalName: img.OriginalName, Path: img.Path}
	}
	return out
}

// BySHA1 e ByOriginalName: em todos os pacotes (pkg "").
func (s *Service) BySHA1(ctx context.Context, shas []string) ([]boxes.ImageRef, error) {
	imgs, err := s.repo.FindImagesByKeys(ctx, "", importer.MatchBySHA1, shas)
	return toRefs(imgs), err
}

func (s *Service) ByOriginalName(ctx context.Context, names []string) ([]boxes.ImageRef, error) {
	imgs, err := s.repo.FindImagesByKeys(ctx, "", importer.MatchByOriginalName, names)
	return toRefs(imgs), err
}

func (s *Service) Exists(ctx context.Context, sha1 string) (bool, error) {
	_, err := s.repo.FindImage(ctx, sha1)
	if errors.Is(err, platform.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

func (s *Service) InPackage(ctx context.Context, pkg string, limit, skip int64) ([]boxes.ImageRef, int64, error) {
	imgs, total, err := s.repo.ListImages(ctx, ImageFilter{Package: pkg, Limit: limit, Skip: skip})
	return toRefs(imgs), total, err
}

func (s *Service) Verified(ctx context.Context, ids []string) (map[string]bool, error) {
	return s.repo.VerifiedIDs(ctx, ids)
}

func (s *Service) PackageProgress(ctx context.Context, names []string) (map[string]catalog.Progress, error) {
	return s.repo.PackageProgress(ctx, names)
}

func (s *Service) RenamePackageImages(ctx context.Context, oldName, newName string) (int64, error) {
	return s.repo.RenamePackageImages(ctx, oldName, newName)
}

func (s *Service) DeletePackageImages(ctx context.Context, name string) (int64, error) {
	return s.repo.DeletePackageImages(ctx, name)
}

func (s *Service) CountUsing(ctx context.Context, reg catalog.Registry, name string) (int64, error) {
	return s.repo.CountUsing(ctx, reg, name)
}

// RenameRegistry troca o equipamento/câmera nas imagens: nas pendentes numa
// operação só; nas verificadas, uma a uma, porque o nome v2 (file_name) tem o
// equipamento e é calculado de novo. Nenhum arquivo muda no disco.
func (s *Service) RenameRegistry(ctx context.Context, reg catalog.Registry, oldName, newName, user string) (catalog.RenameResult, error) {
	now := s.now()
	res := catalog.RenameResult{Failed: []string{}}

	n, err := s.repo.RenameOnPending(ctx, reg, oldName, newName, now)
	if err != nil {
		return res, err
	}
	res.PendingUpdated = n

	// Verificadas, em páginas. As que dão certo deixam de casar com o filtro
	// (já têm o nome novo), então a próxima página começa do zero; só as que
	// falharam são puladas.
	changedBy := fmt.Sprintf("%s via renomear %s", user, reg.ImageField())
	const pageSize = 200
	skip := int64(0)
	for {
		filter := ImageFilter{Status: StatusVerified, Fields: map[string]any{reg.ImageField(): oldName}, Limit: pageSize, Skip: skip}
		page, _, err := s.repo.ListImages(ctx, filter)
		if err != nil {
			return res, err
		}
		if len(page) == 0 {
			return res, nil
		}
		for i := range page {
			if err := s.renameVerified(ctx, &page[i], reg, newName, changedBy, now); err != nil {
				platform.LogError(fmt.Errorf("renomear %s da imagem %s: %w", reg.ImageField(), page[i].SHA1, err))
				res.Failed = append(res.Failed, page[i].SHA1)
				skip++
				continue
			}
			res.VerifiedRenamed++
		}
	}
}

// renameVerified troca o equipamento/câmera de UMA imagem verificada e, se
// ela tem o nome v2, calcula o nome de novo.
func (s *Service) renameVerified(ctx context.Context, img *Image, reg catalog.Registry, newName, changedBy string, now time.Time) error {
	before := *img
	img.Fields = maps.Clone(img.Fields) // o before continua com o mapa antigo
	img.Fields[reg.ImageField()] = newName

	if img.PatternVersion == filename.CurrentVersion {
		base, err := filename.Format(MetaFromImage(img))
		if err != nil {
			return err
		}
		img.FileName = base + img.Ext
	}
	img.UpdatedAt = now

	if err := s.repo.ReplaceImage(ctx, img, before.UpdatedAt); err != nil {
		return err
	}
	if err := s.repo.InsertHistory(ctx, DiffHistory(&before, img, changedBy)); err != nil {
		platform.LogError(err)
	}
	return nil
}

// ---------- para os outros módulos (import de CSV e export) ----------

// Find busca uma imagem (platform.ErrNotFound se não existe).
func (s *Service) Find(ctx context.Context, sha1 string) (*Image, error) {
	return s.repo.FindImage(ctx, sha1)
}

// List: uma página de imagens que passam no filtro.
func (s *Service) List(ctx context.Context, f ImageFilter) ([]Image, int64, error) {
	return s.repo.ListImages(ctx, f)
}

// FindByKeys busca imagens de um pacote (pkg "" = de todos) pela chave do
// import: importer.MatchBySHA1 ou importer.MatchByOriginalName.
func (s *Service) FindByKeys(ctx context.Context, pkg, by string, keys []string) ([]Image, error) {
	return s.repo.FindImagesByKeys(ctx, pkg, by, keys)
}

// Replace grava a imagem inteira, só se ninguém gravou desde prevUpdatedAt
// (ErrConflict se gravou).
func (s *Service) Replace(ctx context.Context, img *Image, prevUpdatedAt time.Time) error {
	return s.repo.ReplaceImage(ctx, img, prevUpdatedAt)
}

// InsertHistory grava entradas no histórico de alterações.
func (s *Service) InsertHistory(ctx context.Context, entries []HistoryEntry) error {
	return s.repo.InsertHistory(ctx, entries)
}
