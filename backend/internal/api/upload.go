package api

import (
	"context"
	"errors"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"SIDM/internal/filename"
	"SIDM/internal/files"
	"SIDM/internal/store"
)

// Limite de um request de upload (várias imagens juntas).
const maxUploadBytes = 5 << 30 // 5 GB

type uploadResult struct {
	File      string `json:"file"`
	SHA1      string `json:"sha1,omitempty"`
	Status    string `json:"status"`              // created | duplicate | error
	Prefilled bool   `json:"prefilled,omitempty"` // nome original já era v2 válido
	Error     string `json:"error,omitempty"`
}

// handleUpload recebe multipart/form-data com um ou mais campos "files".
// Usa MultipartReader (streaming): cada arquivo vai direto para o disco,
// nada de carregar GBs na memória como faria o r.ParseMultipartForm.
// Responde 200 com o resultado de CADA arquivo: um duplicado ou inválido
// não derruba o lote inteiro.
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
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
		results = append(results, s.ingestOne(r.Context(), part))
		part.Close()
	}

	writeJSON(w, http.StatusOK, results)
}

func (s *Server) ingestOne(ctx context.Context, part *multipart.Part) uploadResult {
	name := filepath.Base(part.FileName()) // nunca confie no caminho que vem do cliente
	res := uploadResult{File: name}

	ext := strings.ToLower(filepath.Ext(name))
	if !filename.IsAllowedExt(ext) {
		res.Status, res.Error = "error", "extensão não suportada: "+ext
		return res
	}

	tmp, sum, size, err := s.files.Receive(part)
	if err != nil {
		log.Printf("upload %s: %v", name, err)
		res.Status, res.Error = "error", "falha ao gravar o arquivo"
		return res
	}
	res.SHA1 = sum

	now := time.Now().UTC()
	img := &store.Image{
		SHA1:         sum,
		Status:       store.StatusPending,
		OriginalName: name,
		Path:         files.StagingPath(sum, ext),
		Ext:          ext,
		Size:         size,
		IngestedAt:   now,
		UpdatedAt:    now,
	}

	// Se o nome original já está no padrão v2 E o hash do nome bate com o
	// conteúdo, pré-preenche a anotação. Continua pending: alguém confere.
	if meta, err := filename.Parse(name); err == nil && meta.SHA1 == sum {
		applyMeta(img, meta)
		res.Prefilled = true
	}

	// Banco primeiro, disco depois: se o sha1 já existe, o Mongo recusa
	// e o temporário é descartado sem nunca tocar em staging.
	if err := s.store.InsertImage(ctx, img); err != nil {
		s.files.Discard(tmp)
		if errors.Is(err, store.ErrDuplicate) {
			res.Status = "duplicate"
			return res
		}
		log.Printf("upload %s: %v", name, err)
		res.Status, res.Error = "error", "falha ao registrar no banco"
		return res
	}

	if err := s.files.Stage(tmp, sum, ext); err != nil {
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
	t := m.CapturedAt
	img.CapturedAt = &t
	img.Vehicles = make([]store.Vehicle, 0, len(m.Vehicles))
	for _, v := range m.Vehicles {
		img.Vehicles = append(img.Vehicles, store.Vehicle{
			Plate: v.Plate, PlateType: v.PlateType, PlateCategory: v.PlateCategory,
			Type: v.Type, Color: v.Color,
		})
	}
}
