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
	"time"

	"SIDM/internal/boxes"
	"SIDM/internal/importer"
	"SIDM/internal/store"
)

// Caixas (bounding boxes) anotadas no CVAT, por pacote.
//
//	GET  /packages/{name}/cvat.zip       as imagens do pacote, para criar a task no CVAT
//	POST /packages/{name}/boxes/import   importa o export COCO 1.0 do CVAT (ensaio por padrão)
//	GET  /packages/{name}/boxes          resumo: imagens com caixas e caixas por label
//	GET  /images/{sha1}/boxes            as caixas de uma imagem

// boxImportReport é o resultado do import (no ensaio e na aplicação).
type boxImportReport struct {
	DryRun        bool             `json:"dry_run"`
	Package       string           `json:"package"`
	File          string           `json:"file"`
	FileImages    int              `json:"file_images"` // imagens no arquivo
	FileBoxes     int              `json:"file_boxes"`  // caixas válidas no arquivo
	Labels        map[string]int   `json:"labels"`      // caixas por label (só das imagens casadas)
	Matched       int              `json:"matched"`     // imagens do arquivo que estão no pacote
	NotFound      []string         `json:"not_found"`   // até 20 nomes
	NotFoundCount int              `json:"not_found_count"`
	Ambiguous     []string         `json:"ambiguous"`     // nome original repetido no pacote
	Empty         int              `json:"empty"`         // casadas, mas sem nenhuma caixa: não mexidas
	SizeMismatch  []sizeMismatch   `json:"size_mismatch"` // tamanho no CVAT ≠ arquivo: puladas
	Replaced      int              `json:"replaced"`      // já tinham caixas (serão/foram substituídas)
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

// POST /packages/{name}/boxes/import?dry_run=true   (multipart, campo "file")
// dry_run=true (padrão): só o relatório. dry_run=false: grava.
//
// Casamento de cada imagem do arquivo com o pacote, pelo nome do arquivo
// (sem pasta): se começa com um sha1 (o zip do SIDM para o CVAT usa os nomes
// guardados, que começam pelo sha1), é por ele; se não, pelo nome original,
// quando ele é único no pacote.
//
// Imagens sem nenhuma caixa no arquivo NÃO são mexidas: o CVAT exporta todas
// as imagens da task, inclusive as que ninguém anotou ainda. Imagens cujo
// tamanho no CVAT é diferente do arquivo guardado são puladas (as caixas
// estariam no lugar errado).
func (s *Server) handleImportBoxes(w http.ResponseWriter, r *http.Request) {
	pkg, ok := s.packageByName(w, r, r.PathValue("name"))
	if !ok {
		return
	}
	dryRun := r.URL.Query().Get("dry_run") != "false"

	fileName, data, ok := readUploadedFile(w, r, boxes.MaxFileBytes)
	if !ok {
		return
	}
	parsed, err := boxes.Read(data)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	rep, err := s.matchBoxes(r, pkg.Name, parsed)
	if err != nil {
		s.serverError(w, err)
		return
	}
	rep.DryRun, rep.Package, rep.File = dryRun, pkg.Name, fileName

	if !dryRun && len(rep.matched) > 0 {
		now := time.Now().UTC()
		user := userFrom(r)
		sets := make([]store.BoxSet, 0, len(rep.matched))
		for _, m := range rep.matched {
			sets = append(sets, store.BoxSet{
				ImageID: m.img.SHA1, Package: pkg.Name,
				Width: m.boxes.Width, Height: m.boxes.Height, Boxes: m.boxes.Boxes,
				Source:     store.BoxSource{Tool: "cvat", Format: "coco", File: fileName, FileName: m.boxes.FileName},
				ImportedBy: user, ImportedAt: now,
			})
			rep.AppliedBoxes += len(m.boxes.Boxes)
		}
		if err := s.store.ReplaceBoxSets(r.Context(), sets); err != nil {
			s.serverError(w, err)
			return
		}
		rep.Applied = len(sets)
	}
	writeJSON(w, http.StatusOK, rep)
}

// matchBoxes casa as imagens do arquivo com as do pacote e monta o relatório.
func (s *Server) matchBoxes(r *http.Request, pkg string, f *boxes.File) (*boxImportReport, error) {
	ctx := r.Context()
	rep := &boxImportReport{
		FileImages: len(f.Images), FileBoxes: f.Boxes, Labels: map[string]int{},
		NotFound: []string{}, Ambiguous: []string{}, SizeMismatch: []sizeMismatch{}, Warnings: f.Warnings,
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
	bySHA, err := s.store.FindImagesByKeys(ctx, pkg, importer.MatchBySHA1, shas)
	if err != nil {
		return nil, err
	}
	byName, err := s.store.FindImagesByKeys(ctx, pkg, importer.MatchByOriginalName, names)
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
			continue
		}
		if mismatch, ok := s.checkSize(img, fi); !ok {
			rep.SizeMismatch = append(rep.SizeMismatch, mismatch)
			continue
		}
		rep.matched = append(rep.matched, boxImportMatch{img: img, boxes: fi})
		for _, b := range fi.Boxes {
			rep.Labels[b.Label]++
		}
	}

	ids := make([]string, len(rep.matched))
	for i, m := range rep.matched {
		ids[i] = m.img.SHA1
	}
	existing, err := s.store.BoxImageIDs(ctx, ids)
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
func (s *Server) handleImageBoxes(w http.ResponseWriter, r *http.Request) {
	img, ok := s.imageFromPath(w, r)
	if !ok {
		return
	}
	set, err := s.store.FindBoxSet(r.Context(), img.SHA1)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "esta imagem não tem caixas importadas")
		return
	}
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, set)
}

// GET /packages/{name}/boxes
func (s *Server) handlePackageBoxes(w http.ResponseWriter, r *http.Request) {
	pkg, ok := s.packageByName(w, r, r.PathValue("name"))
	if !ok {
		return
	}
	images, labels, err := s.store.BoxLabelCounts(r.Context(), pkg.Name)
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"package": pkg.Name, "images": images, "labels": labels})
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
