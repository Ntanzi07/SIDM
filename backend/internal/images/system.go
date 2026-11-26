package images

// O "Sistema" do Configurar (só admin): quanto o disco das imagens tem
// livre, o que a lixeira ocupa e os envios de zip que ficaram pela metade.
// O disco cheio é o que derruba o resto (o Mongo, os uploads), então a tela
// avisa antes.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"SIDM/internal/files"
	"SIDM/internal/platform"
)

// DiskWarnPercent: a partir daqui a tela avisa que o disco está enchendo.
const DiskWarnPercent = 90

// Storage é o resumo do disco.
type Storage struct {
	Disk struct {
		Total       uint64  `json:"total"`
		Free        uint64  `json:"free"`
		UsedPercent float64 `json:"used_percent"`
		Warn        bool    `json:"warn"` // passou de DiskWarnPercent
	} `json:"disk"`
	Trash struct {
		Count int   `json:"count"` // arquivos na lixeira
		Bytes int64 `json:"bytes"`
	} `json:"trash"`
	Uploads struct {
		Count int         `json:"count"` // envios de zip que não terminaram
		Bytes int64       `json:"bytes"` // o que os .zip parados ocupam
		Items []ZipUpload `json:"items"`
	} `json:"uploads"`
}

// dirSize soma os arquivos de uma pasta (0 se não existe).
func dirSize(dir string) (count int, bytes int64) {
	filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			count++
			bytes += info.Size()
		}
		return nil
	})
	return count, bytes
}

func (s *Service) StorageSummary() (*Storage, error) {
	var out Storage
	total, free, err := diskSpace(s.files.Abs("."))
	if err != nil {
		return nil, err
	}
	out.Disk.Total, out.Disk.Free = total, free
	if total > 0 {
		out.Disk.UsedPercent = float64(total-free) * 100 / float64(total)
	}
	out.Disk.Warn = out.Disk.UsedPercent >= DiskWarnPercent
	out.Trash.Count, out.Trash.Bytes = dirSize(s.files.Abs(files.TrashDir))

	s.zips.mu.Lock()
	all, err := s.listZips("")
	s.zips.mu.Unlock()
	if err != nil {
		return nil, err
	}
	out.Uploads.Items = []ZipUpload{}
	for _, z := range all {
		if z.Status == ZipDone {
			continue
		}
		out.Uploads.Count++
		if info, err := os.Stat(s.zipPath(z.ID, ".zip")); err == nil {
			out.Uploads.Bytes += info.Size()
		}
		out.Uploads.Items = append(out.Uploads.Items, z)
	}
	return &out, nil
}

// PurgeResult: o que saiu de vez da lixeira.
type PurgeResult struct {
	Images int   `json:"images"`
	Bytes  int64 `json:"bytes"`
}

// PurgeTrash apaga de vez as imagens que estão na lixeira há mais de
// olderThan (o registro e o arquivo). Depois disso, uma versão antiga não
// consegue mais trazer essas imagens de volta.
func (s *Service) PurgeTrash(ctx context.Context, olderThan time.Duration) (PurgeResult, error) {
	var res PurgeResult
	before := s.now().Add(-olderThan)
	for {
		batch, err := s.repo.TrashBefore(ctx, before, 500)
		if err != nil {
			return res, err
		}
		for _, t := range batch {
			rel := files.TrashPath(t.SHA1, t.Ext)
			if info, err := os.Stat(s.files.Abs(rel)); err == nil {
				res.Bytes += info.Size()
			}
			if err := s.files.Remove(rel); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return res, err
			}
			if err := s.repo.DeleteTrash(ctx, t.SHA1); err != nil {
				return res, err
			}
			res.Images++
		}
		if len(batch) < 500 {
			return res, nil
		}
	}
}

// GET /system/storage
func (s *Service) handleStorage(w http.ResponseWriter, r *http.Request) {
	st, err := s.StorageSummary()
	if err != nil {
		platform.ServerError(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, st)
}

// POST /system/trash/purge  {"older_than_days": 30}
func (s *Service) handlePurgeTrash(w http.ResponseWriter, r *http.Request) {
	var in struct {
		OlderThanDays int `json:"older_than_days"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&in); err != nil || in.OlderThanDays < 0 || in.OlderThanDays > 3650 {
		platform.WriteError(w, http.StatusBadRequest, "informe older_than_days (0 a 3650): apaga o que está na lixeira há mais que isso")
		return
	}
	res, err := s.PurgeTrash(r.Context(), time.Duration(in.OlderThanDays)*24*time.Hour)
	if err != nil {
		platform.ServerError(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, res)
}
