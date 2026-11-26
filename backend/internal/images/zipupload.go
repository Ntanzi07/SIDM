package images

// O envio de um .zip em partes: para pastas grandes (dezenas de milhares de
// imagens), o navegador trava só de listar a pasta. Com o zip, ele manda o
// arquivo em pedaços (file.slice, sem ler tudo), e se a conexão cair, volta
// de onde parou (escolhendo o mesmo zip de novo). Quando chega tudo, o
// servidor abre o zip e grava cada imagem como no upload normal, em segundo
// plano (segue mesmo se o navegador fechar), e depois apaga o zip.
//
// O estado de cada envio fica num .json ao lado do zip, em envios/: um
// envio interrompido por um reinício do servidor continua com o mesmo
// "finish".

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"SIDM/internal/annotation"
	"SIDM/internal/importer"
	"SIDM/internal/platform"
)

const (
	zipDir        = "envios"
	ZipChunkBytes = 16 << 20 // o tamanho de cada parte que a tela manda
	maxZipChunk   = 64 << 20 // o maior pedaço aceito num PUT
	maxZipErrors  = 100      // erros guardados (o resto só conta)
	zipSaveEvery  = 2 * time.Second
)

// Os estados de um envio.
const (
	ZipReceiving  = "receiving"  // chegando as partes
	ZipProcessing = "processing" // gravando as imagens
	ZipDone       = "done"
	ZipFailed     = "failed" // o zip não abriu (corrompido?)
)

// ZipUpload é um envio de zip.
type ZipUpload struct {
	ID        string `json:"id"`
	Package   string `json:"package"`
	FileName  string `json:"file_name"`
	Size      int64  `json:"size"`
	Received  int64  `json:"received"`
	Profile   string `json:"profile,omitempty"` // perfil de INI
	Status    string `json:"status"`
	ChunkSize int64  `json:"chunk_size"`

	Total      int      `json:"total"`     // imagens no zip
	Processed  int      `json:"processed"` // já lidas (gravadas, repetidas ou com erro)
	Created    int      `json:"created"`
	Duplicates int      `json:"duplicates"`
	Decrypted  int      `json:"decrypted"`
	WithINI    int      `json:"with_ini"`
	Prefilled  int      `json:"prefilled"`
	Skipped    int      `json:"skipped"` // arquivos que não são imagem (csv, txt...)
	Errors     []string `json:"errors"`
	ErrorCount int      `json:"error_count"`
	Error      string   `json:"error,omitempty"` // por que falhou

	Running   bool      `json:"running"` // gravando agora (senão, "finish" continua)
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// zipState: o que está rodando (o resto fica nos .json).
type zipState struct {
	mu      sync.Mutex
	running map[string]bool
}

var zipIDRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

func (s *Service) zipPath(id, ext string) string { return s.files.Abs(path.Join(zipDir, id+ext)) }

func (s *Service) loadZip(id string) (*ZipUpload, error) {
	if !zipIDRe.MatchString(id) {
		return nil, platform.ErrNotFound
	}
	data, err := os.ReadFile(s.zipPath(id, ".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, platform.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var z ZipUpload
	if err := json.Unmarshal(data, &z); err != nil {
		return nil, err
	}
	z.Running = s.zips.running[id]
	return &z, nil
}

// saveZip grava o estado (num temporário e troca: um reinício no meio não
// deixa o .json pela metade).
func (s *Service) saveZip(z *ZipUpload) error {
	z.UpdatedAt = s.now()
	data, err := json.MarshalIndent(z, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.zipPath(z.ID, ".json.tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.zipPath(z.ID, ".json"))
}

func (s *Service) listZips(pkg string) ([]ZipUpload, error) {
	entries, err := os.ReadDir(s.files.Abs(zipDir))
	if errors.Is(err, os.ErrNotExist) {
		return []ZipUpload{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []ZipUpload{}
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok {
			continue
		}
		z, err := s.loadZip(id)
		if err != nil || (pkg != "" && z.Package != pkg) {
			continue
		}
		out = append(out, *z)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// StartZip abre um envio, ou devolve o que não terminou do mesmo arquivo
// (pacote, nome e tamanho iguais): é assim que a tela retoma.
func (s *Service) StartZip(ctx context.Context, pkg, fileName string, size int64, profile, user string) (*ZipUpload, error) {
	s.zips.mu.Lock()
	defer s.zips.mu.Unlock()
	existing, err := s.listZips(pkg)
	if err != nil {
		return nil, err
	}
	for _, z := range existing {
		if z.FileName == fileName && z.Size == size && z.Status != ZipDone {
			if z.Status == ZipReceiving {
				if err := s.needSpace(zipRoom(&z), "o resto do zip e as imagens dele"); err != nil {
					return nil, err
				}
			}
			if z.Status == ZipReceiving && z.Profile != profile {
				z.Profile = profile
				if err := s.saveZip(&z); err != nil {
					return nil, err
				}
			}
			return &z, nil
		}
	}
	if err := s.needSpace(2*size, "o zip e as imagens dele"); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.files.Abs(zipDir), 0o755); err != nil {
		return nil, err
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	now := s.now()
	z := &ZipUpload{
		ID: hex.EncodeToString(buf), Package: pkg, FileName: fileName, Size: size, Profile: profile,
		Status: ZipReceiving, ChunkSize: ZipChunkBytes, Errors: []string{}, CreatedBy: user, CreatedAt: now,
	}
	f, err := os.Create(s.zipPath(z.ID, ".zip"))
	if err != nil {
		return nil, err
	}
	f.Close()
	return z, s.saveZip(z)
}

// ErrZipOffset: a parte não começa onde o envio parou (a resposta diz onde).
var ErrZipOffset = errors.New("a parte não começa onde o envio parou")

// WriteZipChunk acrescenta uma parte, que tem que começar em offset (o que
// já chegou). Devolve o envio atualizado.
func (s *Service) WriteZipChunk(id string, offset int64, body io.Reader) (*ZipUpload, error) {
	s.zips.mu.Lock()
	defer s.zips.mu.Unlock()
	z, err := s.loadZip(id)
	if err != nil {
		return nil, err
	}
	if z.Status != ZipReceiving {
		return z, &InvalidZipError{"o envio já chegou inteiro"}
	}
	if offset != z.Received {
		return z, ErrZipOffset
	}
	// o disco pode encher no meio do envio (outro envio, outra coisa)
	if err := s.needSpace(min(maxZipChunk, z.Size-z.Received), "a próxima parte"); err != nil {
		return z, err
	}
	f, err := os.OpenFile(s.zipPath(id, ".zip"), os.O_WRONLY, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	// uma parte que chegou pela metade (conexão caiu) é descartada
	if err := f.Truncate(z.Received); err != nil {
		return nil, err
	}
	if _, err := f.Seek(z.Received, io.SeekStart); err != nil {
		return nil, err
	}
	n, err := io.Copy(f, io.LimitReader(body, min(maxZipChunk, z.Size-z.Received)+1))
	if err != nil {
		f.Truncate(z.Received)
		return nil, err
	}
	if z.Received+n > z.Size {
		f.Truncate(z.Received)
		return z, &InvalidZipError{"a parte passa do tamanho do arquivo"}
	}
	if err := f.Sync(); err != nil {
		return nil, err
	}
	z.Received += n
	return z, s.saveZip(z)
}

// InvalidZipError é um pedido que não dá para atender (vira 400).
type InvalidZipError struct{ Msg string }

func (e *InvalidZipError) Error() string { return e.Msg }

// FinishZip começa (ou continua) a gravar as imagens do zip, em segundo
// plano. O zip tem que ter chegado inteiro.
func (s *Service) FinishZip(id string) (*ZipUpload, error) {
	s.zips.mu.Lock()
	defer s.zips.mu.Unlock()
	z, err := s.loadZip(id)
	if err != nil {
		return nil, err
	}
	switch {
	case z.Received < z.Size:
		return z, &InvalidZipError{fmt.Sprintf("faltam %d bytes do zip", z.Size-z.Received)}
	case z.Status == ZipDone || z.Running:
		return z, nil
	}
	if err := s.needSpace(z.Size, "gravar as imagens do zip"); err != nil {
		return z, err
	}
	z.Status, z.Error = ZipProcessing, ""
	if err := s.saveZip(z); err != nil {
		return nil, err
	}
	if s.zips.running == nil {
		s.zips.running = map[string]bool{}
	}
	s.zips.running[id] = true
	z.Running = true
	go s.processZip(*z)
	return z, nil
}

// CancelZip apaga o envio e o zip (não desfaz as imagens já gravadas).
func (s *Service) CancelZip(id string) error {
	s.zips.mu.Lock()
	defer s.zips.mu.Unlock()
	z, err := s.loadZip(id)
	if err != nil {
		return err
	}
	if z.Running {
		return &InvalidZipError{"o zip está sendo gravado: espere terminar"}
	}
	os.Remove(s.zipPath(id, ".zip"))
	return os.Remove(s.zipPath(id, ".json"))
}

// Arquivos que não são imagem (as mesmas regras da tela no upload da pasta:
// a imagem criptografada tem extensão própria, então vai tudo que não é
// claramente outra coisa).
var notImage = regexp.MustCompile(`(?i)\.(csv|txt|ini|json|xml|pdf|log|md|zip|rar|7z|db|exe|lnk|url)$`)

func zipImage(f *zip.File) bool {
	if f.FileInfo().IsDir() || strings.HasPrefix(f.Name, "__MACOSX/") {
		return false
	}
	base := strings.ToLower(path.Base(strings.ReplaceAll(f.Name, "\\", "/")))
	return !strings.HasPrefix(base, ".") && base != "thumbs.db" && base != "desktop.ini" && !notImage.MatchString(base)
}

func (s *Service) processZip(z ZipUpload) {
	defer func() {
		if r := recover(); r != nil {
			z.Status, z.Error = ZipFailed, fmt.Sprintf("erro interno: %v", r)
			s.endZip(&z)
		}
	}()
	ctx := context.Background()
	fail := func(msg string) {
		z.Status, z.Error = ZipFailed, msg
		s.endZip(&z)
	}

	annot, err := s.ProfileOf(ctx, z.Package)
	if err != nil {
		fail(fmt.Sprintf("pacote %q: %v", z.Package, err))
		return
	}
	var profile *importer.Profile
	if z.Profile != "" {
		if profile, err = s.profiles.FindProfile(ctx, z.Profile); err != nil {
			fail(fmt.Sprintf("perfil de INI %q: %v", z.Profile, err))
			return
		}
	}
	// openZip (zipopen.go): também abre o zip64 com o "total de discos" 0
	r, closer, err := openZip(s.zipPath(z.ID, ".zip"))
	if err != nil {
		msg := "o arquivo não abriu como zip (corrompido ou outro formato?): " + err.Error()
		if zeroTail(s.zipPath(z.ID, ".zip")) {
			msg = "o zip chegou com o fim zerado: o disco do servidor provavelmente encheu durante o envio. " +
				"Libere espaço (Configurar → Armazenamento), cancele e envie de novo (" + err.Error() + ")"
		}
		fail(msg)
		return
	}
	defer closer.Close()

	var imgs []*zip.File
	skipped := 0
	for _, f := range r.File {
		if zipImage(f) {
			imgs = append(imgs, f)
		} else if !f.FileInfo().IsDir() {
			skipped++
		}
	}
	z.Total, z.Skipped = len(imgs), skipped
	last := time.Now()
	for i := z.Processed; i < len(imgs); i++ {
		res := s.ingestZipEntry(ctx, &z, annot, profile, imgs[i])
		switch res.Status {
		case "created":
			z.Created++
			if res.Decrypted != "" {
				z.Decrypted++
			}
			if res.INI {
				z.WithINI++
			}
			if res.Prefilled {
				z.Prefilled++
			}
		case "duplicate":
			z.Duplicates++
		default:
			z.ErrorCount++
			if len(z.Errors) < maxZipErrors {
				z.Errors = append(z.Errors, fmt.Sprintf("%s: %s", imgs[i].Name, res.Error))
			}
		}
		z.Processed = i + 1
		if time.Since(last) > zipSaveEvery {
			s.zips.mu.Lock()
			if err := s.saveZip(&z); err != nil {
				log.Printf("envio %s: %v", z.ID, err)
			}
			s.zips.mu.Unlock()
			last = time.Now()
		}
	}
	closer.Close() // antes de apagar (no Windows, aberto não apaga); o defer fecha de novo sem efeito
	z.Status = ZipDone
	os.Remove(s.zipPath(z.ID, ".zip")) // as imagens já estão no pacote
	s.endZip(&z)
}

func (s *Service) ingestZipEntry(ctx context.Context, z *ZipUpload, annot *annotation.Profile, profile *importer.Profile, f *zip.File) UploadResult {
	rc, err := f.Open()
	if err != nil {
		return UploadResult{File: f.Name, Status: "error", Error: "não abriu dentro do zip: " + err.Error()}
	}
	defer rc.Close()
	return s.ingestOne(ctx, z.Package, annot, profile, f.Name, rc)
}

func (s *Service) endZip(z *ZipUpload) {
	s.zips.mu.Lock()
	defer s.zips.mu.Unlock()
	delete(s.zips.running, z.ID)
	z.Running = false
	if err := s.saveZip(z); err != nil {
		log.Printf("envio %s: %v", z.ID, err)
	}
}

// ---------- HTTP ----------

func writeZipErr(w http.ResponseWriter, z *ZipUpload, err error) {
	var invalid *InvalidZipError
	var noSpace *NoSpaceError
	switch {
	case errors.Is(err, platform.ErrNotFound):
		platform.WriteError(w, http.StatusNotFound, "envio não encontrado (já terminou ou foi cancelado?)")
	case errors.Is(err, ErrZipOffset):
		platform.WriteJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "upload": z})
	case errors.As(err, &invalid):
		platform.WriteError(w, http.StatusBadRequest, invalid.Msg)
	case errors.As(err, &noSpace):
		platform.WriteError(w, http.StatusInsufficientStorage, noSpace.Msg)
	default:
		platform.ServerError(w, err)
	}
}

// POST /packages/{name}/zip-uploads  {"file_name", "size", "profile"}
func (s *Service) handleStartZip(w http.ResponseWriter, r *http.Request) {
	pkg := r.PathValue("name")
	if _, err := s.catalog.FindPackage(r.Context(), pkg); err != nil {
		if errors.Is(err, platform.ErrNotFound) {
			platform.WriteError(w, http.StatusNotFound, fmt.Sprintf("pacote %q não existe", pkg))
			return
		}
		platform.ServerError(w, err)
		return
	}
	var in struct {
		FileName string `json:"file_name"`
		Size     int64  `json:"size"`
		Profile  string `json:"profile"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in); err != nil {
		platform.WriteError(w, http.StatusBadRequest, "JSON inválido: "+err.Error())
		return
	}
	in.FileName = path.Base(strings.ReplaceAll(strings.TrimSpace(in.FileName), "\\", "/"))
	if !strings.EqualFold(path.Ext(in.FileName), ".zip") || in.Size <= 0 {
		platform.WriteError(w, http.StatusBadRequest, "informe um arquivo .zip (file_name) e o tamanho dele (size)")
		return
	}
	if _, ok := s.iniProfileFromRequest(w, r, pkg, in.Profile); !ok {
		return
	}
	z, err := s.StartZip(r.Context(), pkg, in.FileName, in.Size, in.Profile, platform.UserFrom(r))
	if err != nil {
		writeZipErr(w, z, err) // sem disco: 507
		return
	}
	platform.WriteJSON(w, http.StatusOK, z)
}

// GET /packages/{name}/zip-uploads: os envios do pacote (os que não
// terminaram primeiro não; o mais novo primeiro).
func (s *Service) handleListZips(w http.ResponseWriter, r *http.Request) {
	s.zips.mu.Lock()
	list, err := s.listZips(r.PathValue("name"))
	s.zips.mu.Unlock()
	if err != nil {
		platform.ServerError(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, map[string]any{"items": list})
}

// GET /zip-uploads/{id}
func (s *Service) handleGetZip(w http.ResponseWriter, r *http.Request) {
	s.zips.mu.Lock()
	z, err := s.loadZip(r.PathValue("id"))
	s.zips.mu.Unlock()
	if err != nil {
		writeZipErr(w, nil, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, z)
}

// PUT /zip-uploads/{id}/chunk?offset=N  (o corpo é a parte, cru)
func (s *Service) handleZipChunk(w http.ResponseWriter, r *http.Request) {
	offset, err := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
	if err != nil || offset < 0 {
		platform.WriteError(w, http.StatusBadRequest, "informe ?offset= (onde a parte começa)")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxZipChunk)
	z, err := s.WriteZipChunk(r.PathValue("id"), offset, r.Body)
	if err != nil {
		writeZipErr(w, z, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, z)
}

// POST /zip-uploads/{id}/finish
func (s *Service) handleFinishZip(w http.ResponseWriter, r *http.Request) {
	z, err := s.FinishZip(r.PathValue("id"))
	if err != nil {
		writeZipErr(w, z, err)
		return
	}
	platform.WriteJSON(w, http.StatusAccepted, z)
}

// DELETE /zip-uploads/{id}
func (s *Service) handleCancelZip(w http.ResponseWriter, r *http.Request) {
	if err := s.CancelZip(r.PathValue("id")); err != nil {
		writeZipErr(w, nil, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true})
}
