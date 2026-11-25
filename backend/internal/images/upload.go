package images

import (
	"SIDM/internal/annotation"
	"SIDM/internal/crypt"
	"SIDM/internal/filename"
	"SIDM/internal/files"
	"SIDM/internal/importer"
	"SIDM/internal/platform"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// INIKeyColumn é a coluna-chave da tabela montada a partir do INI de uma
// imagem (o sha1), para os perfis jpeg-ini casarem pelo sha1.
const INIKeyColumn = "_sidm_sha1"

// MaxINIScanBytes: até onde o arquivo é lido procurando o INI.
const MaxINIScanBytes = 64 << 20

type UploadResult struct {
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
func (s *Service) handleUpload(w http.ResponseWriter, r *http.Request) {
	pkg := r.URL.Query().Get("package")
	if pkg == "" {
		platform.WriteError(w, http.StatusBadRequest, "informe o pacote: POST /images?package=<nome>")
		return
	}
	annot, err := s.ProfileOf(r.Context(), pkg)
	if err != nil {
		if errors.Is(err, platform.ErrNotFound) {
			platform.WriteError(w, http.StatusNotFound, fmt.Sprintf("pacote %q não existe: crie antes com POST /packages", pkg))
			return
		}
		platform.ServerError(w, err)
		return
	}

	profile, ok := s.iniProfileFromRequest(w, r, pkg, r.URL.Query().Get("profile"))
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)

	mr, err := r.MultipartReader()
	if err != nil {
		platform.WriteError(w, http.StatusBadRequest, "esperado multipart/form-data com campo \"files\"")
		return
	}

	results := []UploadResult{}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			platform.WriteError(w, http.StatusBadRequest, "erro lendo o upload: "+err.Error())
			return
		}
		if part.FormName() != "files" || part.FileName() == "" {
			part.Close()
			continue
		}
		results = append(results, s.ingestOne(r.Context(), pkg, annot, profile, part.FileName(), part))
		part.Close()
	}

	platform.WriteJSON(w, http.StatusOK, results)
}

// ingestOne grava UM arquivo (do upload ou de dentro de um zip).
// iniProfileFromRequest busca o perfil de INI do upload (vazio = nenhum).
// Se der errado, já escreve a resposta e devolve ok=false.
func (s *Service) iniProfileFromRequest(w http.ResponseWriter, r *http.Request, pkg, name string) (*importer.Profile, bool) {
	if name == "" {
		return nil, true
	}
	p, err := s.profiles.FindProfile(r.Context(), name)
	if errors.Is(err, platform.ErrNotFound) {
		platform.WriteError(w, http.StatusNotFound, fmt.Sprintf("perfil %q não existe", name))
		return nil, false
	}
	if err != nil {
		platform.ServerError(w, err)
		return nil, false
	}
	if p.Reader.Type != importer.ReaderJPEGINI {
		platform.WriteError(w, http.StatusBadRequest, fmt.Sprintf(
			"o perfil %q é do tipo %q: no upload só vale perfil %q (CSV é com POST /packages/%s/import)",
			p.Name, p.Reader.Type, importer.ReaderJPEGINI, pkg))
		return nil, false
	}
	return p, true
}

func (s *Service) ingestOne(ctx context.Context, pkg string, annot *annotation.Profile, profile *importer.Profile, fileName string, body io.Reader) UploadResult {
	name := filepath.Base(filepath.FromSlash(strings.ReplaceAll(fileName, "\\", "/"))) // nunca confie no caminho que vem do cliente
	res := UploadResult{File: name}

	ext := strings.ToLower(filepath.Ext(name))

	tmp, sum, size, err := s.files.Receive(body)
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
	img := &Image{
		SHA1:         sum,
		Package:      pkg,
		Status:       StatusPending,
		OriginalName: name,
		Path:         files.PendingPath(pkg, sum, ext),
		Ext:          ext,
		Size:         size,
		Encryption:   res.Decrypted,
		QueuedAt:     now, // entra no fim da fila do pacote
		IngestedAt:   now,
		UpdatedAt:    now,
		Fields:       map[string]any{},
		Objects:      []annotation.Object{},
	}

	// Se o nome original já está no padrão v2 E o hash do nome bate com o
	// conteúdo, pré-preenche a anotação (o v2 só existe no perfil veiculos).
	// Continua pending: alguém confere.
	if annot.Is(annotation.DefaultProfile) {
		if meta, err := filename.Parse(name); err == nil && meta.SHA1 == sum {
			fit, _ := annot.Fit(ValuesFromMeta(meta))
			if MergeAnnotation(img, fit, false) {
				res.Prefilled = true
			}
		}
	}

	// INI embutido: lido do temporário (o arquivo em si nunca é alterado).
	if entries := ReadEmbeddedINI(tmp, size); len(entries) > 0 {
		img.INI = entries
		res.INI = true
		if profile != nil {
			vals, rep := AnnotationFromINI(profile, annot, sum, entries)
			res.Warnings = s.SanitizeAndRegister(ctx, annot, &vals)
			res.Warnings = append(res.Warnings, ReportWarnings(rep)...)
			fit, dropped := annot.Fit(vals)
			res.Warnings = append(res.Warnings, dropped...)
			if MergeAnnotation(img, fit, false) {
				res.Prefilled = true
			}
		}
	}

	// Banco primeiro, disco depois: se o sha1 já existe, o Mongo recusa
	// e o temporário é descartado sem nunca tocar na pasta do pacote.
	if err := s.repo.InsertImage(ctx, img); err != nil {
		s.files.Discard(tmp)
		if errors.Is(err, platform.ErrDuplicate) {
			res.Status = "duplicate"
			if existing, err := s.repo.FindImage(ctx, sum); err == nil {
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
		_ = s.repo.DeleteImage(context.Background(), sum)
		s.files.Discard(tmp)
		log.Printf("upload %s: %v", name, err)
		res.Status, res.Error = "error", "falha ao mover o arquivo"
		return res
	}

	res.Status = "created"
	return res
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
func (s *Service) openIfEncrypted(tmp, sum string, size int64, ext string, res *UploadResult) (string, string, int64, string, error) {
	head, err := readHead(tmp, 3)
	if err == nil && crypt.IsJPEG(head) {
		if !filename.IsAllowedExt(ext) {
			return tmp, sum, size, ext, fmt.Errorf("extensão não suportada: %s", ext)
		}
		return tmp, sum, size, ext, nil
	}
	if size > MaxINIScanBytes {
		return tmp, sum, size, ext, fmt.Errorf("arquivo grande demais para verificar a criptografia (máx. %d MB)", MaxINIScanBytes>>20)
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

// ReadEmbeddedINI lê o INI gravado depois do fim do JPEG, ou nil.
func ReadEmbeddedINI(path string, size int64) []importer.INIEntry {
	if size > MaxINIScanBytes {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return importer.ParseINI(importer.ExtractJPEGTrailer(data))
}

// AnnotationFromINI aplica um perfil jpeg-ini nas entradas de UMA imagem,
// passando pelo mesmo Transform do CSV (mesmas regras de map, data, template).
// ap é o perfil de anotação do pacote.
func AnnotationFromINI(p *importer.Profile, ap *annotation.Profile, sha string, entries []importer.INIEntry) (annotation.Values, importer.Report) {
	run := *p
	run.Match = importer.MatchConfig{Column: INIKeyColumn, By: importer.MatchBySHA1}
	records, rep := importer.Transform(&run, ap, importer.INITable(INIKeyColumn, sha, entries))
	if len(records) == 0 {
		return annotation.Values{Fields: map[string]any{}, Objects: []annotation.Object{}}, rep
	}
	return records[0].Values, rep
}

// SanitizeAndRegister: os campos de cadastro (equipamento, câmera) vindos do
// INI passam pela mesma sanitização do cadastro e são cadastrados se ainda
// não existirem (igual ao import de CSV). Nome impossível de sanitizar vira
// aviso e sai dos valores.
func (s *Service) SanitizeAndRegister(ctx context.Context, ap *annotation.Profile, v *annotation.Values) []string {
	var warnings []string
	for _, f := range ap.Fields {
		if f.Type != annotation.TypeRef {
			continue
		}
		value, _ := v.Fields[f.Key].(string)
		if value == "" {
			continue
		}
		what, sanitize, exists, create := "equipamento", filename.SanitizeEquipment, s.catalog.EquipmentExists, s.catalog.CreateEquipment
		if f.Ref == annotation.RefCamera {
			what, sanitize, exists, create = "câmera", filename.SanitizeCamera, s.catalog.CameraExists, s.catalog.CreateCamera
		}
		name, err := sanitize(value)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s %q ignorado: %v", what, value, err))
			delete(v.Fields, f.Key)
			continue
		}
		v.Fields[f.Key] = name
		if ok, err := exists(ctx, name); err == nil && !ok {
			switch err := create(ctx, name); {
			case errors.Is(err, platform.ErrDuplicate):
				// Existe, mas desativado: o valor fica (é o que o arquivo diz),
				// e o aviso explica por que a anotação vai reclamar dele.
				warnings = append(warnings, fmt.Sprintf("%s %q está desativado: reative em Configurar para verificar esta imagem", what, name))
			case err != nil:
				warnings = append(warnings, fmt.Sprintf("não foi possível cadastrar %s %q", what, name))
				platform.LogError(err)
			}
		}
	}
	return warnings
}

// ReportWarnings resume o relatório de uma imagem em avisos legíveis.
func ReportWarnings(rep importer.Report) []string {
	var out []string
	for _, e := range rep.Errors {
		out = append(out, fmt.Sprintf("%s: %s", e.Field, e.Error))
	}
	fields := make([]string, 0, len(rep.Unmapped))
	for f := range rep.Unmapped {
		fields = append(fields, f)
	}
	sort.Strings(fields)
	for _, f := range fields {
		for v := range rep.Unmapped[f] {
			out = append(out, fmt.Sprintf("%s: valor %q sem tradução no perfil", f, v))
		}
	}
	return out
}
