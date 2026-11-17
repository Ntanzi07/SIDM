package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"SIDM/internal/crypt"
	"SIDM/internal/filename"
	"SIDM/internal/files"
	"SIDM/internal/importer"
	"SIDM/internal/store"
)

// Limite de um request de upload (várias imagens juntas).
const maxUploadBytes = 5 << 30 // 5 GB

type uploadResult struct {
	File            string   `json:"file"`
	SHA1            string   `json:"sha1,omitempty"`
	Status          string   `json:"status"`                     // created | duplicate | error
	ExistingPackage string   `json:"existing_package,omitempty"` // duplicate: onde a imagem já está
	Decrypted       string   `json:"decrypted,omitempty"`        // formato de criptografia que foi aberto (ex: rc4-legacy)
	INI             bool     `json:"ini,omitempty"`              // o arquivo tinha INI embutido
	Prefilled       bool     `json:"prefilled,omitempty"`        // nome v2 válido ou perfil de INI aplicado
	Warnings        []string `json:"warnings,omitempty"`         // ex: data do INI em formato inesperado
	Error           string   `json:"error,omitempty"`
}

// handleUpload: POST /images?package=<nome>[&profile=<perfil jpeg-ini>],
// multipart/form-data com um ou mais campos "files". O pacote precisa existir.
//
// Todo INI embutido no JPEG é guardado no banco. Com ?profile=, o perfil
// (do tipo jpeg-ini) também pré-preenche o rascunho de cada imagem.
//
// Usa MultipartReader (streaming): cada arquivo vai direto para o disco,
// nada de carregar GBs na memória como faria o r.ParseMultipartForm.
// Responde 200 com o resultado de CADA arquivo: uma imagem repetida (que já
// está em outro pacote) é PULADA e o resto do lote segue normalmente.
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	pkg := r.URL.Query().Get("package")
	if pkg == "" {
		writeError(w, http.StatusBadRequest, "informe o pacote: POST /images?package=<nome>")
		return
	}
	if _, err := s.store.FindPackage(r.Context(), pkg); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, fmt.Sprintf("pacote %q não existe: crie antes com POST /packages", pkg))
			return
		}
		s.serverError(w, err)
		return
	}

	var profile *importer.Profile
	if name := r.URL.Query().Get("profile"); name != "" {
		p, ok := s.profileByName(w, r, name)
		if !ok {
			return
		}
		if p.Reader.Type != importer.ReaderJPEGINI {
			writeError(w, http.StatusBadRequest, fmt.Sprintf(
				"o perfil %q é do tipo %q: no upload só vale perfil %q (CSV é com POST /packages/%s/import)",
				p.Name, p.Reader.Type, importer.ReaderJPEGINI, pkg))
			return
		}
		profile = p
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)

	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "esperado multipart/form-data com campo \"files\"")
		return
	}

	results := []uploadResult{}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, "erro lendo o upload: "+err.Error())
			return
		}
		if part.FormName() != "files" || part.FileName() == "" {
			part.Close()
			continue
		}
		results = append(results, s.ingestOne(r.Context(), pkg, profile, part))
		part.Close()
	}

	writeJSON(w, http.StatusOK, results)
}

func (s *Server) ingestOne(ctx context.Context, pkg string, profile *importer.Profile, part *multipart.Part) uploadResult {
	name := filepath.Base(part.FileName()) // nunca confie no caminho que vem do cliente
	res := uploadResult{File: name}

	ext := strings.ToLower(filepath.Ext(name))

	tmp, sum, size, err := s.files.Receive(part)
	if err != nil {
		log.Printf("upload %s: %v", name, err)
		res.Status, res.Error = "error", "falha ao gravar o arquivo"
		return res
	}

	// Arquivo criptografado pelo equipamento? Abre ANTES de tudo: o hash, o
	// INI e o arquivo guardado são os do original (o que foi assinado).
	tmp, sum, size, ext, err = s.openIfEncrypted(tmp, sum, size, ext, &res)
	if err != nil {
		s.files.Discard(tmp)
		res.Status, res.Error = "error", err.Error()
		return res
	}
	res.SHA1 = sum

	now := time.Now().UTC()
	img := &store.Image{
		SHA1:         sum,
		Package:      pkg,
		Status:       store.StatusPending,
		OriginalName: name,
		Path:         files.PendingPath(pkg, sum, ext),
		Ext:          ext,
		Size:         size,
		Encryption:   res.Decrypted,
		QueuedAt:     now, // entra no fim da fila do pacote
		IngestedAt:   now,
		UpdatedAt:    now,
	}

	// Se o nome original já está no padrão v2 E o hash do nome bate com o
	// conteúdo, pré-preenche a anotação. Continua pending: alguém confere.
	if meta, err := filename.Parse(name); err == nil && meta.SHA1 == sum {
		applyMeta(img, meta)
		res.Prefilled = true
	}

	// INI embutido: lido do temporário (o arquivo em si nunca é alterado).
	if entries := readEmbeddedINI(tmp, size); len(entries) > 0 {
		img.INI = entries
		res.INI = true
		if profile != nil {
			ann, rep := annotationFromINI(profile, sum, entries)
			ann.Equipment, ann.Camera, res.Warnings = s.sanitizeAndRegister(ctx, ann.Equipment, ann.Camera)
			res.Warnings = append(res.Warnings, reportWarnings(rep)...)
			if mergeAnnotation(img, ann, false) {
				res.Prefilled = true
			}
		}
	}

	// Banco primeiro, disco depois: se o sha1 já existe, o Mongo recusa
	// e o temporário é descartado sem nunca tocar na pasta do pacote.
	if err := s.store.InsertImage(ctx, img); err != nil {
		s.files.Discard(tmp)
		if errors.Is(err, store.ErrDuplicate) {
			res.Status = "duplicate"
			if existing, err := s.store.FindImage(ctx, sum); err == nil {
				res.ExistingPackage = existing.Package
			}
			return res
		}
		log.Printf("upload %s: %v", name, err)
		res.Status, res.Error = "error", "falha ao registrar no banco"
		return res
	}

	if err := s.files.Store(tmp, img.Path); err != nil {
		// desfaz o registro para banco e disco não divergirem
		_ = s.store.DeleteImage(context.Background(), sum)
		s.files.Discard(tmp)
		log.Printf("upload %s: %v", name, err)
		res.Status, res.Error = "error", "falha ao mover o arquivo"
		return res
	}

	res.Status = "created"
	return res
}

func applyMeta(img *store.Image, m filename.ImageMeta) {
	img.PatternVersion = m.Version
	img.Equipment = m.Equipment
	img.Camera = m.Camera
	img.Direction = m.Direction
	img.Resolution = m.Resolution
	img.CaptureAngle = m.CaptureAngle
	img.DayTime = m.DayTime
	img.ImageRange = m.ImageRange
	img.Weather = m.Weather
	img.Conditions = m.Conditions
	img.CapturedAt = nil
	if !m.CapturedAt.IsZero() { // posição vazia no nome = data desconhecida
		t := m.CapturedAt
		img.CapturedAt = &t
	}
	img.Vehicles = make([]store.Vehicle, 0, len(m.Vehicles))
	for _, v := range m.Vehicles {
		img.Vehicles = append(img.Vehicles, store.Vehicle{
			Plate: v.Plate, PlateType: v.PlateType, PlateCategory: v.PlateCategory,
			Type: v.Type, Color: v.Color,
		})
	}
}

// openIfEncrypted confere se o arquivo recebido é criptografado e, se for,
// troca o temporário pelo arquivo aberto (com o novo sha1 e tamanho).
//
// Regras:
//   - JPEG aberto: segue, exigindo extensão permitida.
//   - RC4 legado: abre; a extensão vira ".jpg" se a original não for de imagem
//     (os equipamentos usam extensões próprias para o arquivo criptografado).
//   - PGP: recusa, com uma mensagem clara (ainda não suportado).
//   - qualquer outro conteúdo: segue as regras de antes (só a extensão importa).
func (s *Server) openIfEncrypted(tmp, sum string, size int64, ext string, res *uploadResult) (string, string, int64, string, error) {
	head, err := readHead(tmp, 3)
	if err == nil && crypt.IsJPEG(head) {
		if !filename.IsAllowedExt(ext) {
			return tmp, sum, size, ext, fmt.Errorf("extensão não suportada: %s", ext)
		}
		return tmp, sum, size, ext, nil
	}
	if size > maxINIScanBytes {
		return tmp, sum, size, ext, fmt.Errorf("arquivo grande demais para verificar a criptografia (máx. %d MB)", maxINIScanBytes>>20)
	}

	data, err := os.ReadFile(tmp)
	if err != nil {
		return tmp, sum, size, ext, fmt.Errorf("falha ao ler o arquivo")
	}
	plain, format, err := crypt.Open(data)
	switch {
	case errors.Is(err, crypt.ErrPGPUnsupported):
		return tmp, sum, size, ext, err
	case errors.Is(err, crypt.ErrNotEncrypted):
		if !filename.IsAllowedExt(ext) {
			return tmp, sum, size, ext, fmt.Errorf("extensão não suportada: %s", ext)
		}
		return tmp, sum, size, ext, nil
	case err != nil:
		return tmp, sum, size, ext, err
	}

	// Troca o temporário criptografado pelo aberto.
	newTmp, newSum, newSize, err := s.files.Receive(bytes.NewReader(plain))
	if err != nil {
		return tmp, sum, size, ext, fmt.Errorf("falha ao gravar o arquivo aberto")
	}
	s.files.Discard(tmp)
	if !filename.IsAllowedExt(ext) {
		ext = ".jpg"
	}
	res.Decrypted = format
	return newTmp, newSum, newSize, ext, nil
}

// readHead lê só os primeiros n bytes de um arquivo.
func readHead(path string, n int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, n)
	k, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}
	return buf[:k], nil
}
