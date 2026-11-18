package api

import (
	"context"
	"fmt"
	"time"

	"SIDM/internal/catalog"
	"SIDM/internal/filename"
	"SIDM/internal/files"
	"SIDM/internal/platform"
	"SIDM/internal/store"
)

// LegacyImages é o lado das IMAGENS das operações do catálogo
// (catalog.Images), enquanto as imagens não viram um módulo: usa o store e
// os arquivos direto. Quando o módulo images existir, isto vai para lá.
type LegacyImages struct {
	store Store
	files *files.Storage
	now   func() time.Time
}

func NewLegacyImages(s Store, fs *files.Storage) *LegacyImages {
	return &LegacyImages{store: s, files: fs, now: func() time.Time { return time.Now().UTC() }}
}

var _ catalog.Images = (*LegacyImages)(nil)

func (l *LegacyImages) PackageProgress(ctx context.Context, names []string) (map[string]catalog.Progress, error) {
	return l.store.PackageProgress(ctx, names)
}

func (l *LegacyImages) RenamePackageImages(ctx context.Context, oldName, newName string) (int64, error) {
	return l.store.RenamePackageImages(ctx, oldName, newName)
}

func (l *LegacyImages) DeletePackageImages(ctx context.Context, name string) (int64, error) {
	return l.store.DeletePackageImages(ctx, name)
}

func (l *LegacyImages) CountUsing(ctx context.Context, reg catalog.Registry, name string) (int64, error) {
	return l.store.CountUsing(ctx, reg, name)
}

// RenameRegistry troca o equipamento/câmera nas imagens: nas pendentes só no
// banco (uma operação); nas verificadas, uma a uma, renomeando também o
// arquivo para o novo nome v2 (desfazendo o arquivo se o banco falhar).
func (l *LegacyImages) RenameRegistry(ctx context.Context, reg catalog.Registry, oldName, newName, user string) (catalog.RenameResult, error) {
	now := l.now()
	res := catalog.RenameResult{Failed: []string{}}

	n, err := l.store.RenameOnPending(ctx, reg, oldName, newName, now)
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
		filter := store.ImageFilter{Status: store.StatusVerified, Limit: pageSize, Skip: skip}
		if reg == catalog.Cameras {
			filter.Camera = oldName
		} else {
			filter.Equipment = oldName
		}
		page, _, err := l.store.ListImages(ctx, filter)
		if err != nil {
			return res, err
		}
		if len(page) == 0 {
			return res, nil
		}
		for i := range page {
			if err := l.renameVerified(ctx, &page[i], reg, newName, changedBy, now); err != nil {
				platform.LogError(fmt.Errorf("renomear %s da imagem %s: %w", reg.ImageField(), page[i].SHA1, err))
				res.Failed = append(res.Failed, page[i].SHA1)
				skip++
				continue
			}
			res.VerifiedRenamed++
		}
	}
}

// renameVerified troca o equipamento/câmera de UMA imagem verificada e
// renomeia o arquivo para o novo nome v2.
func (l *LegacyImages) renameVerified(ctx context.Context, img *store.Image, reg catalog.Registry, newName, changedBy string, now time.Time) error {
	before := *img
	if reg == catalog.Cameras {
		img.Camera = newName
	} else {
		img.Equipment = newName
	}

	base, err := filename.Format(metaFromImage(img))
	if err != nil {
		return err
	}
	newPath := files.VerifiedPath(img.Package, base+img.Ext)
	if newPath != img.Path {
		if err := l.files.Move(img.Path, newPath); err != nil {
			return err
		}
	}
	img.Path, img.FileName, img.UpdatedAt = newPath, base+img.Ext, now

	if err := l.store.ReplaceImage(ctx, img, before.UpdatedAt); err != nil {
		if newPath != before.Path {
			_ = l.files.Move(newPath, before.Path) // banco e disco nunca divergem
		}
		return err
	}
	if err := l.store.InsertHistory(ctx, diffHistory(&before, img, changedBy)); err != nil {
		platform.LogError(err)
	}
	return nil
}
