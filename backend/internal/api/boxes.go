package api

import (
	"archive/zip"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // DecodeConfig: tamanho das imagens
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path"
	"sort"
	"strings"

	"SIDM/internal/boxes"
	"SIDM/internal/importer"
	"SIDM/internal/store"
)

// Caixas (bounding boxes) anotadas no CVAT. O import é por TAREFA (ver
// tasks.go); aqui ficam o casamento das imagens, o zip para o CVAT e as
// consultas.
//
//	GET  /packages/{name}/cvat.zip       as imagens do pacote, para criar a task no CVAT
//	GET  /packages/{name}/boxes          por tarefa: imagens do pacote anotadas nela
//	GET  /images/{sha1}/boxes            as caixas de uma imagem, em todas as tarefas

// boxImportReport é o resultado do import (no ensaio e na aplicação).
type boxImportReport struct {
	DryRun        bool             `json:"dry_run"`
	Task          string           `json:"task"`
	File          string           `json:"file"`
	Negatives     bool             `json:"negatives"`   // imagens sem caixas entram como negativas?
	FileImages    int              `json:"file_images"` // imagens no arquivo
	FileBoxes     int              `json:"file_boxes"`  // caixas válidas no arquivo
	Labels        map[string]int   `json:"labels"`      // caixas por label (só das imagens que serão gravadas)
	NewLabels     []string         `json:"new_labels"`  // labels que não estão na tarefa (são acrescentadas)
	Matched       int              `json:"matched"`     // imagens do arquivo encontradas no SIDM
	Packages      map[string]int   `json:"packages"`    // das que serão gravadas, quantas por pacote
	NotFound      []string         `json:"not_found"`   // até 20 nomes
	NotFoundCount int              `json:"not_found_count"`
	Ambiguous     []string         `json:"ambiguous"`     // nome original repetido no banco
	Empty         int              `json:"empty"`         // encontradas, mas sem nenhuma caixa no arquivo
	SizeMismatch  []sizeMismatch   `json:"size_mismatch"` // tamanho no CVAT ≠ arquivo: puladas
	WillApply     int              `json:"will_apply"`    // imagens que serão gravadas
	Replaced      int              `json:"replaced"`      // dessas, as que já estavam na tarefa (serão substituídas)
	Applied       int              `json:"applied"`       // imagens gravadas (0 no ensaio)
	AppliedBoxes  int              `json:"applied_boxes"`
	Warnings      []string         `json:"warnings"` // caixas ignoradas na leitura
	matched       []boxImportMatch `json:"-"`
}

type sizeMismatch struct {
	FileName string `json:"file_name"`
	CVAT     string `json:"cvat"`
	SIDM     string `json:"sidm"`
}

type boxImportMatch struct {
	img   store.Image
	boxes boxes.Image
}

const maxNotFoundListed = 20

// matchBoxes casa as imagens do arquivo com as do SIDM (em TODOS os pacotes)
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
func (s *Server) matchBoxes(r *http.Request, task *store.Task, f *boxes.File, negatives bool) (*boxImportReport, error) {
	ctx := r.Context()
	rep := &boxImportReport{
		Task: task.Name, Negatives: negatives,
		FileImages: len(f.Images), FileBoxes: f.Boxes, Labels: map[string]int{}, NewLabels: []string{},
		Packages: map[string]int{}, NotFound: []string{}, Ambiguous: []string{}, SizeMismatch: []sizeMismatch{},
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
	bySHA, err := s.store.FindImagesByKeys(ctx, "", importer.MatchBySHA1, shas)
	if err != nil {
		return nil, err
	}
	byName, err := s.store.FindImagesByKeys(ctx, "", importer.MatchByOriginalName, names)
	if err != nil {
		return nil, err
	}
	shaIndex := make(map[string]store.Image, len(bySHA))
	for _, img := range bySHA {
		shaIndex[img.SHA1] = img
	}
	nameIndex := map[string][]store.Image{}
	for _, img := range byName {
		nameIndex[img.OriginalName] = append(nameIndex[img.OriginalName], img)
	}

	seen := map[string]bool{} // a mesma imagem duas vezes no arquivo: fica a primeira
	for i, fi := range f.Images {
		var img store.Image
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
		rep.matched = append(rep.matched, boxImportMatch{img: img, boxes: fi})
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
	existing, err := s.store.FindBoxSets(ctx, task.Name, ids)
	if err != nil {
		return nil, err
	}
	rep.Replaced = len(existing)
	return rep, nil
}

func (rep *boxImportReport) notFound(name string) {
	rep.NotFoundCount++
	if len(rep.NotFound) < maxNotFoundListed {
		rep.NotFound = append(rep.NotFound, name)
	}
}

// checkSize confere o tamanho da imagem no CVAT com o arquivo guardado. Se o
// CVAT não informou o tamanho, não há o que conferir.
func (s *Server) checkSize(img store.Image, fi boxes.Image) (sizeMismatch, bool) {
	if fi.Width == 0 || fi.Height == 0 {
		return sizeMismatch{}, true
	}
	cvat := fmt.Sprintf("%dx%d", fi.Width, fi.Height)
	f, err := os.Open(s.files.Abs(img.Path))
	if err != nil {
		s.logError(fmt.Errorf("conferir tamanho de %s: %w", img.Path, err))
		return sizeMismatch{FileName: fi.FileName, CVAT: cvat, SIDM: "arquivo não encontrado"}, false
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return sizeMismatch{FileName: fi.FileName, CVAT: cvat, SIDM: "não foi possível ler o tamanho"}, false
	}
	if cfg.Width != fi.Width || cfg.Height != fi.Height {
		return sizeMismatch{FileName: fi.FileName, CVAT: cvat, SIDM: fmt.Sprintf("%dx%d", cfg.Width, cfg.Height)}, false
	}
	return sizeMismatch{}, true
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

// readUploadedFile lê o campo "file" de um multipart, com limite.
func readUploadedFile(w http.ResponseWriter, r *http.Request, limit int64) (string, []byte, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, limit+1<<20)
	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, `esperado multipart/form-data com o campo "file"`)
		return "", nil, false
	}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			writeError(w, http.StatusBadRequest, `faltou o campo "file" com o export do CVAT (zip ou .json)`)
			return "", nil, false
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, "erro lendo o upload: "+err.Error())
			return "", nil, false
		}
		if part.FormName() != "file" {
			part.Close()
			continue
		}
		data, err := io.ReadAll(io.LimitReader(part, limit+1))
		name := part.FileName()
		part.Close()
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) || int64(len(data)) > limit {
			writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("arquivo maior que %d MB", limit>>20))
			return "", nil, false
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, "erro lendo o arquivo: "+err.Error())
			return "", nil, false
		}
		return name, data, true
	}
}

// GET /images/{sha1}/boxes
// As caixas da imagem em cada tarefa ({"items": [...]}, vazio se nenhuma).
func (s *Server) handleImageBoxes(w http.ResponseWriter, r *http.Request) {
	img, ok := s.imageFromPath(w, r)
	if !ok {
		return
	}
	sets, err := s.store.FindImageBoxSets(r.Context(), img.SHA1)
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": sets})
}

// GET /packages/{name}/boxes
// Por tarefa, quantas imagens do pacote estão anotadas nela.
func (s *Server) handlePackageBoxes(w http.ResponseWriter, r *http.Request) {
	pkg, ok := s.packageByName(w, r, r.PathValue("name"))
	if !ok {
		return
	}
	counts, err := s.store.PackageBoxCounts(r.Context(), pkg.Name)
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"package": pkg.Name, "tasks": counts})
}

// GET /packages/{name}/cvat.zip
// As imagens do pacote, com os nomes guardados no SIDM (começam pelo sha1),
// para criar a task no CVAT. Sem compressão: JPEG já é comprimido, e assim o
// zip sai rápido. É gerado enquanto é baixado (não ocupa disco).
func (s *Server) handleCVATZip(w http.ResponseWriter, r *http.Request) {
	pkg, ok := s.packageByName(w, r, r.PathValue("name"))
	if !ok {
		return
	}
	ctx := r.Context()
	// Confere antes de começar a mandar: depois do primeiro byte, não dá
	// mais para responder com erro.
	if _, total, err := s.store.ListImages(ctx, store.ImageFilter{Package: pkg.Name, Limit: 1}); err != nil {
		s.serverError(w, err)
		return
	} else if total == 0 {
		writeError(w, http.StatusNotFound, "o pacote não tem imagens")
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-cvat.zip"`, pkg.Name))
	zw := zip.NewWriter(w)
	var missing []string
	const page = 500
	for skip := int64(0); ; skip += page {
		imgs, _, err := s.store.ListImages(ctx, store.ImageFilter{Package: pkg.Name, Limit: page, Skip: skip})
		if err != nil {
			s.logError(fmt.Errorf("cvat.zip de %s: %w", pkg.Name, err))
			missing = append(missing, "(erro listando imagens: o zip pode estar incompleto)")
			break
		}
		for _, img := range imgs {
			if err := s.addToZip(zw, img); err != nil {
				s.logError(fmt.Errorf("cvat.zip de %s: %w", pkg.Name, err))
				missing = append(missing, img.Path)
			}
		}
		if len(imgs) < page {
			break
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		if fw, err := zw.Create("FALTANDO.txt"); err == nil {
			fmt.Fprintf(fw, "Estas imagens não puderam ser incluídas:\r\n%s\r\n", strings.Join(missing, "\r\n"))
		}
	}
	if err := zw.Close(); err != nil {
		s.logError(fmt.Errorf("cvat.zip de %s: %w", pkg.Name, err))
	}
}

func (s *Server) addToZip(zw *zip.Writer, img store.Image) error {
	f, err := os.Open(s.files.Abs(img.Path))
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	hdr := &zip.FileHeader{Name: path.Base(img.Path), Method: zip.Store, Modified: info.ModTime()}
	fw, err := zw.CreateHeader(hdr)
	if err != nil {
		return err
	}
	_, err = io.Copy(fw, f)
	return err
}
