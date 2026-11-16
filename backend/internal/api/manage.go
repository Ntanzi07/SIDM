package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"SIDM/internal/filename"
	"SIDM/internal/files"
	"SIDM/internal/store"
)

// ---------- equipamentos e câmeras ----------

type registryUpdate struct {
	Name   *string `json:"name"`   // novo nome (renomeia em cascata; se já existir, FUNDE)
	Active *bool   `json:"active"` // ativar / desativar
}

type registryReport struct {
	Name            string   `json:"name"`             // o nome final
	Renamed         bool     `json:"renamed"`          // o nome mudou
	Merged          bool     `json:"merged"`           // o novo nome já existia: os dois viraram um
	Active          bool     `json:"active"`           //
	PendingUpdated  int64    `json:"pending_updated"`  // pendentes que trocaram de nome (só no banco)
	VerifiedRenamed int      `json:"verified_renamed"` // verificadas: arquivo renomeado para o novo nome v2
	Failed          []string `json:"failed"`           // sha1 das que não deu para renomear agora
	Complete        bool     `json:"complete"`         // false: rode de novo para terminar as que falharam
}

// PATCH /equipments/{name}  e  PATCH /cameras/{name}
//
//	{"name": "8071-MFR2"}  renomeia em CASCATA: todas as imagens que usam o
//	                       nome antigo passam a usar o novo; nas verificadas,
//	                       o arquivo é renomeado para o novo nome v2.
//	                       Se o novo nome já existir, os dois são FUNDIDOS
//	                       (útil para limpar duplicatas criadas pelo import).
//	{"active": false}      desativa (some do autocomplete e não pode ser
//	                       usado em novas verificações).
func (s *Server) handleUpdateRegistry(reg store.Registry, sanitize func(string) (string, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		oldName := r.PathValue("name")
		entry, ok := s.registryByName(w, r, reg, oldName)
		if !ok {
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
		var req registryUpdate
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, `JSON inválido: esperado {"name": "..."} e/ou {"active": true|false}`)
			return
		}
		if req.Name == nil && req.Active == nil {
			writeError(w, http.StatusBadRequest, `nada para alterar: informe "name" e/ou "active"`)
			return
		}

		rep := registryReport{Name: oldName, Active: entry.Active, Failed: []string{}, Complete: true}

		if req.Name != nil {
			newName, err := sanitize(*req.Name)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			if newName != oldName {
				if !s.cascadeRename(w, r, reg, entry, newName, &rep) {
					return
				}
			}
		}

		if req.Active != nil {
			if err := s.store.SetRegistryActive(ctx, reg, rep.Name, *req.Active); err != nil {
				s.serverError(w, err)
				return
			}
			rep.Active = *req.Active
		}
		writeJSON(w, http.StatusOK, rep)
	}
}

// cascadeRename faz a renomeação em cascata. A ordem foi pensada para poder
// ser REPETIDA se algo falhar no meio:
//
//  1. garante que o nome novo existe no cadastro;
//  2. troca o nome nas pendentes (só banco, uma operação);
//  3. renomeia as verificadas uma a uma (arquivo + banco, desfazendo o
//     arquivo se o banco falhar);
//  4. só então apaga o nome antigo. Se alguma verificada falhou, o antigo
//     fica, e o mesmo PATCH pode ser chamado de novo para terminar.
func (s *Server) cascadeRename(w http.ResponseWriter, r *http.Request, reg store.Registry, old *store.RegistryEntry, newName string, rep *registryReport) bool {
	ctx := r.Context()
	now := time.Now().UTC()

	// 1. o nome novo
	target, err := s.store.FindRegistry(ctx, reg, newName)
	switch {
	case err == nil:
		rep.Merged = true
		rep.Active = target.Active || old.Active
	case errors.Is(err, store.ErrNotFound):
		if err := s.createRegistry(ctx, reg, newName); err != nil && !errors.Is(err, store.ErrDuplicate) {
			s.serverError(w, err)
			return false
		}
		rep.Active = old.Active
	default:
		s.serverError(w, err)
		return false
	}
	if err := s.store.SetRegistryActive(ctx, reg, newName, rep.Active); err != nil {
		s.serverError(w, err)
		return false
	}

	// 2. pendentes
	n, err := s.store.RenameOnPending(ctx, reg, old.Name, newName, now)
	if err != nil {
		s.serverError(w, err)
		return false
	}
	rep.PendingUpdated = n

	// 3. verificadas, em páginas. As que dão certo deixam de casar com o
	// filtro (já têm o nome novo), então a próxima página começa do zero;
	// só as que falharam são puladas.
	changedBy := fmt.Sprintf("%s via renomear %s", userFrom(r), reg.ImageField())
	const pageSize = 200
	skip := int64(0)
	for {
		filter := store.ImageFilter{Status: store.StatusVerified, Limit: pageSize, Skip: skip}
		if reg == store.Cameras {
			filter.Camera = old.Name
		} else {
			filter.Equipment = old.Name
		}
		page, _, err := s.store.ListImages(ctx, filter)
		if err != nil {
			s.serverError(w, err)
			return false
		}
		if len(page) == 0 {
			break
		}
		for i := range page {
			if err := s.renameVerified(ctx, &page[i], reg, newName, changedBy, now); err != nil {
				s.logError(fmt.Errorf("renomear %s da imagem %s: %w", reg.ImageField(), page[i].SHA1, err))
				rep.Failed = append(rep.Failed, page[i].SHA1)
				skip++
				continue
			}
			rep.VerifiedRenamed++
		}
	}

	// 4. o nome antigo, só se tudo deu certo
	rep.Name, rep.Renamed = newName, true
	if len(rep.Failed) > 0 {
		rep.Complete = false
		return true
	}
	if err := s.store.DeleteRegistry(ctx, reg, old.Name); err != nil && !errors.Is(err, store.ErrNotFound) {
		s.serverError(w, err)
		return false
	}
	return true
}

// renameVerified troca o equipamento/câmera de UMA imagem verificada e
// renomeia o arquivo para o novo nome v2.
func (s *Server) renameVerified(ctx context.Context, img *store.Image, reg store.Registry, newName, changedBy string, now time.Time) error {
	before := *img
	if reg == store.Cameras {
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
		if err := s.files.Move(img.Path, newPath); err != nil {
			return err
		}
	}
	img.Path, img.FileName, img.UpdatedAt = newPath, base+img.Ext, now

	if err := s.store.ReplaceImage(ctx, img, before.UpdatedAt); err != nil {
		if newPath != before.Path {
			_ = s.files.Move(newPath, before.Path) // banco e disco nunca divergem
		}
		return err
	}
	if err := s.store.InsertHistory(ctx, diffHistory(&before, img, changedBy)); err != nil {
		s.logError(err)
	}
	return nil
}

func (s *Server) createRegistry(ctx context.Context, reg store.Registry, name string) error {
	if reg == store.Cameras {
		return s.store.CreateCamera(ctx, name)
	}
	return s.store.CreateEquipment(ctx, name)
}

func (s *Server) registryByName(w http.ResponseWriter, r *http.Request, reg store.Registry, name string) (*store.RegistryEntry, bool) {
	e, err := s.store.FindRegistry(r.Context(), reg, name)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("%s %q não existe", reg.ImageField(), name))
		return nil, false
	}
	if err != nil {
		s.serverError(w, err)
		return nil, false
	}
	return e, true
}

// DELETE /equipments/{name}  e  DELETE /cameras/{name}
// Em uso por alguma imagem: DESATIVA (as imagens continuam intactas).
// Sem uso: apaga de vez.
func (s *Server) handleDeleteRegistry(reg store.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		name := r.PathValue("name")
		if _, ok := s.registryByName(w, r, reg, name); !ok {
			return
		}

		n, err := s.store.CountImagesUsing(ctx, reg, name)
		if err != nil {
			s.serverError(w, err)
			return
		}

		resp := map[string]any{"name": name, "images": n}
		if n > 0 {
			if err := s.store.SetRegistryActive(ctx, reg, name, false); err != nil {
				s.serverError(w, err)
				return
			}
			resp["deleted"], resp["deactivated"] = false, true
		} else {
			if err := s.store.DeleteRegistry(ctx, reg, name); err != nil {
				s.serverError(w, err)
				return
			}
			resp["deleted"], resp["deactivated"] = true, false
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// ---------- pacotes ----------

// PATCH /packages/{name}  {"name": "novo nome"}
// Renomeia o pacote: a pasta no disco, o cadastro e o caminho de todas as
// imagens. O nome dos arquivos não muda (o pacote não faz parte do nome v2).
func (s *Server) handleRenamePackage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	oldName := r.PathValue("name")
	if _, ok := s.packageByName(w, r, oldName); !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
	var req struct {
		Name string `json:"name"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, `JSON inválido: esperado {"name": "..."}`)
		return
	}
	newName, err := filename.SanitizePackage(req.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if newName == oldName {
		writeJSON(w, http.StatusOK, map[string]any{"name": newName, "images": 0})
		return
	}
	if _, err := s.store.FindPackage(ctx, newName); err == nil {
		writeError(w, http.StatusConflict, fmt.Sprintf("já existe um pacote %q", newName))
		return
	}

	// Disco primeiro (um rename de pasta, instantâneo); se o banco falhar,
	// a pasta volta ao nome antigo.
	if err := s.files.RenamePackageDir(oldName, newName); err != nil {
		s.serverError(w, err)
		return
	}
	n, err := s.store.RenamePackage(ctx, oldName, newName)
	if err != nil {
		_ = s.files.RenamePackageDir(newName, oldName)
		if errors.Is(err, store.ErrDuplicate) {
			writeError(w, http.StatusConflict, fmt.Sprintf("já existe um pacote %q", newName))
			return
		}
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": newName, "images": n})
}

// DELETE /packages/{name}?confirm=<name>
// Apaga o pacote, TODAS as imagens dele (banco e arquivos) e o histórico.
// IRREVERSÍVEL: por isso exige repetir o nome em ?confirm=.
func (s *Server) handleDeletePackage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name := r.PathValue("name")
	if r.URL.Query().Get("confirm") != name {
		writeError(w, http.StatusBadRequest, fmt.Sprintf(
			"isto apaga o pacote, todas as imagens e os arquivos, sem volta. Para confirmar, repita o nome: ?confirm=%s", name))
		return
	}
	if _, ok := s.packageByName(w, r, name); !ok {
		return
	}

	// Banco primeiro: se o disco falhar depois, sobram arquivos órfãos (que
	// dá para apagar na mão), mas nunca um registro apontando para um arquivo
	// que não existe.
	n, err := s.store.DeletePackage(ctx, name)
	if err != nil {
		s.serverError(w, err)
		return
	}
	warnings := []string{}
	if err := s.files.RemovePackageDir(name); err != nil {
		s.logError(err)
		warnings = append(warnings, "os registros foram apagados, mas a pasta não: apague dataset/"+name+" manualmente")
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "deleted_images": n, "warnings": warnings})
}

func (s *Server) packageByName(w http.ResponseWriter, r *http.Request, name string) (*store.Package, bool) {
	p, err := s.store.FindPackage(r.Context(), name)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("pacote %q não existe", name))
		return nil, false
	}
	if err != nil {
		s.serverError(w, err)
		return nil, false
	}
	return p, true
}
