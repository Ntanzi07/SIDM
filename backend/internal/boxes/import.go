package boxes

import (
	"context"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/jpeg" // DecodeConfig: tamanho das imagens
	_ "image/png"
	"os"
	"path"
	"sort"
	"strings"

	"SIDM/internal/platform"
)

// O import do export COCO 1.0 do CVAT numa tarefa: casar as imagens do
// arquivo com as do SIDM (em todos os pacotes), montar o relatório (ensaio)
// e gravar.

// ImportReport é o resultado do import (no ensaio e na aplicação).
type ImportReport struct {
	DryRun        bool           `json:"dry_run"`
	Task          string         `json:"task"`
	File          string         `json:"file"`
	Negatives     bool           `json:"negatives"`   // imagens sem caixas entram como negativas?
	FileImages    int            `json:"file_images"` // imagens no arquivo
	FileBoxes     int            `json:"file_boxes"`  // caixas válidas no arquivo
	Labels        map[string]int `json:"labels"`      // caixas por label (só das imagens que serão gravadas)
	NewLabels     []string       `json:"new_labels"`  // labels que não estão na tarefa (são acrescentadas)
	Matched       int            `json:"matched"`     // imagens do arquivo encontradas no SIDM
	Packages      map[string]int `json:"packages"`    // das que serão gravadas, quantas por pacote
	NotFound      []string       `json:"not_found"`   // até 20 nomes
	NotFoundCount int            `json:"not_found_count"`
	Ambiguous     []string       `json:"ambiguous"`     // nome original repetido no banco
	Empty         int            `json:"empty"`         // encontradas, mas sem nenhuma caixa no arquivo
	SizeMismatch  []SizeMismatch `json:"size_mismatch"` // tamanho no CVAT ≠ arquivo: puladas
	WillApply     int            `json:"will_apply"`    // imagens que serão gravadas
	Replaced      int            `json:"replaced"`      // dessas, as que já estavam na tarefa (serão substituídas)
	Applied       int            `json:"applied"`       // imagens gravadas (0 no ensaio)
	AppliedBoxes  int            `json:"applied_boxes"`
	Warnings      []string       `json:"warnings"` // caixas ignoradas na leitura
	matched       []importMatch  `json:"-"`
}

type SizeMismatch struct {
	FileName string `json:"file_name"`
	CVAT     string `json:"cvat"`
	SIDM     string `json:"sidm"`
}

type importMatch struct {
	img   ImageRef
	boxes Image
}

const maxNotFoundListed = 20

// match casa as imagens do arquivo com as do SIDM (em TODOS os pacotes)
// e monta o relatório.
//
// Casamento pelo nome do arquivo (sem pasta): se começa com um sha1 (o zip do
// SIDM para o CVAT usa os nomes guardados, que começam pelo sha1), é por ele;
// se não, pelo nome original, só quando ele é único no banco todo (nomes como
// IMG_0001.jpg se repetem entre câmeras: na dúvida, a imagem fica de fora).
//
// Imagens sem nenhuma caixa no arquivo: com negatives=false (padrão) NÃO são
// mexidas, porque o CVAT exporta todas as imagens da task, inclusive as que
// ninguém anotou ainda. Com negatives=true entram como anotadas sem objetos
// (use só quando a task do CVAT estiver toda anotada). Imagens cujo tamanho no
// CVAT é diferente do arquivo guardado são puladas (as caixas ficariam no
// lugar errado).
func (s *Service) match(ctx context.Context, task *Task, f *File, negatives bool) (*ImportReport, error) {
	rep := &ImportReport{
		Task: task.Name, Negatives: negatives,
		FileImages: len(f.Images), FileBoxes: f.Boxes, Labels: map[string]int{}, NewLabels: []string{},
		Packages: map[string]int{}, NotFound: []string{}, Ambiguous: []string{}, SizeMismatch: []SizeMismatch{},
		Warnings: f.Warnings,
	}
	if rep.Warnings == nil {
		rep.Warnings = []string{}
	}

	// Separa quem tem sha1 no nome de quem tem só o nome original.
	keyOf := make([]string, len(f.Images))
	isSHA := make([]bool, len(f.Images))
	var shas, names []string
	for i, img := range f.Images {
		base := path.Base(strings.ReplaceAll(img.FileName, `\`, "/"))
		if sha, ok := leadingSHA1(base); ok {
			keyOf[i], isSHA[i] = sha, true
			shas = append(shas, sha)
		} else {
			keyOf[i] = base
			names = append(names, base)
		}
	}
	bySHA, err := s.images.BySHA1(ctx, shas)
	if err != nil {
		return nil, err
	}
	byName, err := s.images.ByOriginalName(ctx, names)
	if err != nil {
		return nil, err
	}
	shaIndex := make(map[string]ImageRef, len(bySHA))
	for _, img := range bySHA {
		shaIndex[img.SHA1] = img
	}
	nameIndex := map[string][]ImageRef{}
	for _, img := range byName {
		nameIndex[img.OriginalName] = append(nameIndex[img.OriginalName], img)
	}

	seen := map[string]bool{} // a mesma imagem duas vezes no arquivo: fica a primeira
	for i, fi := range f.Images {
		var img ImageRef
		if isSHA[i] {
			found, ok := shaIndex[keyOf[i]]
			if !ok {
				rep.notFound(fi.FileName)
				continue
			}
			img = found
		} else {
			found := nameIndex[keyOf[i]]
			switch {
			case len(found) == 0:
				rep.notFound(fi.FileName)
				continue
			case len(found) > 1:
				rep.Ambiguous = append(rep.Ambiguous, fi.FileName)
				continue
			}
			img = found[0]
		}
		if seen[img.SHA1] {
			continue
		}
		seen[img.SHA1] = true
		rep.Matched++

		if len(fi.Boxes) == 0 {
			rep.Empty++
			if !negatives {
				continue
			}
		}
		if mismatch, ok := s.checkSize(img, fi); !ok {
			rep.SizeMismatch = append(rep.SizeMismatch, mismatch)
			continue
		}
		rep.matched = append(rep.matched, importMatch{img: img, boxes: fi})
		rep.Packages[img.Package]++
		for _, b := range fi.Boxes {
			rep.Labels[b.Label]++
		}
	}
	rep.WillApply = len(rep.matched)

	known := make(map[string]bool, len(task.Labels))
	for _, l := range task.Labels {
		known[l] = true
	}
	for l := range rep.Labels {
		if !known[l] {
			rep.NewLabels = append(rep.NewLabels, l)
		}
	}
	sort.Strings(rep.NewLabels)

	ids := make([]string, len(rep.matched))
	for i, m := range rep.matched {
		ids[i] = m.img.SHA1
	}
	existing, err := s.repo.FindSets(ctx, task.Name, ids)
	if err != nil {
		return nil, err
	}
	rep.Replaced = len(existing)
	return rep, nil
}

func (rep *ImportReport) notFound(name string) {
	rep.NotFoundCount++
	if len(rep.NotFound) < maxNotFoundListed {
		rep.NotFound = append(rep.NotFound, name)
	}
}

// checkSize confere o tamanho da imagem no CVAT com o arquivo guardado. Se o
// CVAT não informou o tamanho, não há o que conferir.
func (s *Service) checkSize(img ImageRef, fi Image) (SizeMismatch, bool) {
	if fi.Width == 0 || fi.Height == 0 {
		return SizeMismatch{}, true
	}
	cvat := fmt.Sprintf("%dx%d", fi.Width, fi.Height)
	f, err := os.Open(s.files.Abs(img.Path))
	if err != nil {
		platform.LogError(fmt.Errorf("conferir tamanho de %s: %w", img.Path, err))
		return SizeMismatch{FileName: fi.FileName, CVAT: cvat, SIDM: "arquivo não encontrado"}, false
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return SizeMismatch{FileName: fi.FileName, CVAT: cvat, SIDM: "não foi possível ler o tamanho"}, false
	}
	if cfg.Width != fi.Width || cfg.Height != fi.Height {
		return SizeMismatch{FileName: fi.FileName, CVAT: cvat, SIDM: fmt.Sprintf("%dx%d", cfg.Width, cfg.Height)}, false
	}
	return SizeMismatch{}, true
}

// leadingSHA1: "<40 hex>..." -> sha1 em minúsculas.
func leadingSHA1(name string) (string, bool) {
	if len(name) < 40 {
		return "", false
	}
	sha := strings.ToLower(name[:40])
	if _, err := hex.DecodeString(sha); err != nil {
		return "", false
	}
	// "<sha1>.jpg", "<sha1>_v2_..." — o que vem depois não pode continuar o hex.
	if len(name) > 40 && isHex(name[40]) {
		return "", false
	}
	return sha, true
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// Import casa as imagens do arquivo e, se dryRun for false, grava as caixas
// na tarefa. Labels que a tarefa ainda não tem são acrescentadas a ela.
func (s *Service) Import(ctx context.Context, taskName, fileName string, data []byte, dryRun, negatives bool, user string) (*ImportReport, error) {
	task, err := s.repo.FindTask(ctx, taskName)
	if err != nil {
		return nil, err
	}
	parsed, err := Read(data)
	if err != nil {
		return nil, &BadFileError{err}
	}
	rep, err := s.match(ctx, task, parsed, negatives)
	if err != nil {
		return nil, err
	}
	rep.DryRun, rep.File = dryRun, fileName
	if dryRun || len(rep.matched) == 0 {
		return rep, nil
	}

	now := s.now()
	sets := make([]Set, 0, len(rep.matched))
	for _, m := range rep.matched {
		sets = append(sets, Set{
			Task: task.Name, ImageID: m.img.SHA1, Package: m.img.Package,
			Width: m.boxes.Width, Height: m.boxes.Height, Boxes: m.boxes.Boxes,
			Source:     Source{Tool: "cvat", Format: "coco", File: fileName, FileName: m.boxes.FileName},
			ImportedBy: user, ImportedAt: now,
		})
		rep.AppliedBoxes += len(m.boxes.Boxes)
	}
	// Labels primeiro: se gravar as caixas falhar, sobra só label a mais.
	if err := s.repo.AddTaskLabels(ctx, task.Name, rep.NewLabels, now); err != nil {
		return nil, err
	}
	if err := s.repo.ReplaceSets(ctx, sets); err != nil {
		return nil, err
	}
	rep.Applied = len(sets)
	return rep, nil
}

// BadFileError: o arquivo não é um COCO 1.0 que dê para ler (vira 400).
type BadFileError struct{ Err error }

func (e *BadFileError) Error() string { return e.Err.Error() }
func (e *BadFileError) Unwrap() error { return e.Err }
