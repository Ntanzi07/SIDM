package images

// Corrigir pelo arquivo os campos que vêm da imagem (Field.FromImage, ex.: a
// resolução) nas imagens gravadas antes da regra, quando eram escolhidos à
// mão. Uma ação de manutenção, só de admin (POST /system/from-image): primeiro
// o ensaio (nada muda), depois corrigir. Na verificada o nome v2 também muda
// (ele leva o código da resolução). Cada correção fica no histórico.
//
// Duas passadas: a primeira só lê e anota o que muda (gravar durante a
// paginação podia mudar a ordem da lista e pular imagens); a segunda relê
// cada uma e grava com a versão dela (outra pessoa salvou no meio = pula).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"time"

	"SIDM/internal/annotation"
	"SIDM/internal/platform"
)

// FromImageReport é o resultado do ensaio ou da correção.
type FromImageReport struct {
	DryRun   bool `json:"dry_run"`
	Checked  int  `json:"checked"`  // imagens de perfis com campo que vem da imagem
	Changed  int  `json:"changed"`  // com o valor diferente do arquivo (corrigidas, fora do ensaio)
	Verified int  `json:"verified"` // das Changed, verificadas: o nome v2 muda junto
	Cleared  int  `json:"cleared"`  // das Changed, o tamanho não é opção: o campo fica vazio
	Busy     int  `json:"busy"`     // alguém está anotando: puladas
	Failed   int  `json:"failed"`   // não deu para ler o arquivo ou montar o nome
	// Examples: algumas das que mudam ("antes → depois"), para conferir.
	Examples []string `json:"examples"`
	Errors   []string `json:"errors"`
}

const fromImageExamples = 8

type fromImagePlan struct{ sha1 string }

// FixFromImage confere (dryRun) ou corrige as imagens de todos os pacotes.
func (s *Service) FixFromImage(ctx context.Context, dryRun bool, user string) (*FromImageReport, error) {
	rep := &FromImageReport{DryRun: dryRun, Examples: []string{}, Errors: []string{}}
	now := time.Now().UTC()
	profiles := map[string]*annotation.Profile{} // por pacote (nil = sem campo que vem da imagem)
	profileOf := func(pkg string) *annotation.Profile {
		if p, ok := profiles[pkg]; ok {
			return p
		}
		p, err := s.ProfileOf(ctx, pkg)
		if err != nil || !usesImage(p) {
			p = nil
		}
		profiles[pkg] = p
		return p
	}

	// 1ª passada: só lê.
	var plan []fromImagePlan
	const pageSize = 500
	for skip := int64(0); ; skip += pageSize {
		page, _, err := s.repo.ListImages(ctx, ImageFilter{Limit: pageSize, Skip: skip})
		if err != nil {
			return nil, err
		}
		for i := range page {
			img := &page[i]
			p := profileOf(img.Package)
			if p == nil {
				continue
			}
			rep.Checked++
			size := s.sizeOf(img)
			if size.Width == 0 {
				rep.Failed++
				rep.addError(fmt.Sprintf("%s: não deu para ler o tamanho do arquivo", img.OriginalName))
				continue
			}
			fixed, _ := p.FillFromImage(copyFields(img.Fields), size)
			if reflect.DeepEqual(fixed, img.Fields) {
				continue
			}
			if img.ClaimedByOther(user, now) {
				rep.Busy++
				continue
			}
			rep.Changed++
			if img.Status == StatusVerified {
				rep.Verified++
			}
			if len(fixed) < len(img.Fields) {
				rep.Cleared++
			}
			if len(rep.Examples) < fromImageExamples {
				rep.Examples = append(rep.Examples, fmt.Sprintf("%s: %s", img.OriginalName, fromImageChange(p, img.Fields, fixed)))
			}
			plan = append(plan, fromImagePlan{sha1: img.SHA1})
		}
		if len(page) < pageSize {
			break
		}
	}
	if dryRun {
		return rep, nil
	}

	// 2ª passada: relê e grava cada uma.
	changedBy := user + " via resolução da imagem"
	for _, item := range plan {
		img, err := s.repo.FindImage(ctx, item.sha1)
		if err != nil {
			rep.fail(fmt.Sprintf("%s: %v", item.sha1, err))
			continue
		}
		p := profileOf(img.Package)
		if p == nil {
			continue
		}
		before := *img
		img.Fields, _ = p.FillFromImage(copyFields(img.Fields), s.sizeOf(img))
		if reflect.DeepEqual(img.Fields, before.Fields) {
			rep.Changed-- // já estava certo (alguém salvou no meio)
			continue
		}
		if img.Status == StatusVerified {
			name, version, msg := s.finalName(ctx, img, p)
			if msg != "" {
				rep.fail(fmt.Sprintf("%s: %s", img.OriginalName, msg))
				continue
			}
			img.FileName, img.PatternVersion = name, version
		}
		img.UpdatedAt = time.Now().UTC()
		if err := s.repo.ReplaceImage(ctx, img, before.UpdatedAt); err != nil {
			if errors.Is(err, ErrConflict) {
				rep.Busy++
				rep.Changed--
				continue
			}
			rep.fail(fmt.Sprintf("%s: %v", img.OriginalName, err))
			continue
		}
		if err := s.repo.InsertHistory(ctx, DiffHistory(&before, img, changedBy)); err != nil {
			platform.LogError(err) // a correção já foi gravada
		}
	}
	return rep, nil
}

func (r *FromImageReport) addError(msg string) {
	if len(r.Errors) < 50 {
		r.Errors = append(r.Errors, msg)
	}
}

func (r *FromImageReport) fail(msg string) {
	r.Failed++
	r.Changed--
	r.addError(msg)
}

func copyFields(in map[string]any) map[string]any {
	out := make(map[string]any, len(in)+1)
	for k, v := range in {
		out[k] = v
	}
	return out
}

// "Resolução: 1920x1080 → 1280x960"
func fromImageChange(p *annotation.Profile, before, after map[string]any) string {
	for _, f := range p.Fields {
		if f.FromImage == "" || reflect.DeepEqual(before[f.Key], after[f.Key]) {
			continue
		}
		old, cur := before[f.Key], after[f.Key]
		if old == nil {
			old = "vazio"
		}
		if cur == nil {
			cur = "vazio (o tamanho não é opção)"
		}
		label := f.Label
		if label == "" {
			label = f.Key
		}
		return fmt.Sprintf("%s: %v → %v", label, old, cur)
	}
	return "muda"
}

// POST /system/from-image  {"dry_run": true}  (sem corpo = ensaio)
func (s *Service) handleFixFromImage(w http.ResponseWriter, r *http.Request) {
	in := struct {
		DryRun *bool `json:"dry_run"`
	}{}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&in); err != nil && !errors.Is(err, io.EOF) {
		platform.WriteError(w, http.StatusBadRequest, `JSON inválido: esperado {"dry_run": true|false}`)
		return
	}
	dryRun := in.DryRun == nil || *in.DryRun // por segurança, só grava se pedir
	rep, err := s.FixFromImage(r.Context(), dryRun, platform.UserFrom(r))
	if err != nil {
		platform.ServerError(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, rep)
}
