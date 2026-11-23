package boxes

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"

	"SIDM/internal/annotation"
	"SIDM/internal/platform"
)

// Routes: as rotas do módulo.
//
//	GET    /tasks                     todas, com o resumo de cada uma
//	POST   /tasks                     cria {name, description, labels}
//	GET    /tasks/{name}              a tarefa e o resumo (pacotes, caixas por label)
//	PUT    /tasks/{name}              atualiza; mudar o nome leva junto caixas e perfis de export
//	DELETE /tasks/{name}              apaga a tarefa e as caixas dela
//	POST   /tasks/{name}/import       importa o COCO 1.0 do CVAT (ensaio por padrão)
//	GET    /images/{sha1}/boxes       as caixas de uma imagem, em todas as tarefas
//	GET    /packages/{name}/boxes     por tarefa: imagens do pacote anotadas nela
//	GET    /packages/{name}/cvat.zip  as imagens do pacote, para criar a task no CVAT
//	GET    /boxes/images.zip?package=a&package=b  as imagens de vários pacotes num zip só
//	GET    /boxes/packages            em quais tarefas cada pacote tem imagens
//	GET    /boxes/stats               labels das tarefas nas imagens de uma pesquisa (GET /images/stats)
func (s *Service) Routes() []platform.Route {
	return []platform.Route{
		{Method: "GET", Path: "/tasks", Handler: s.handleListTasks},
		{Method: "POST", Path: "/tasks", Handler: s.handleCreateTask},
		{Method: "GET", Path: "/tasks/{name}", Handler: s.handleGetTask},
		{Method: "PUT", Path: "/tasks/{name}", Handler: s.handleReplaceTask},
		{Method: "DELETE", Path: "/tasks/{name}", Handler: s.handleDeleteTask},
		{Method: "POST", Path: "/tasks/{name}/import", Handler: s.handleImportTask},
		{Method: "GET", Path: "/images/{sha1}/boxes", Handler: s.handleImageBoxes},
		{Method: "PUT", Path: "/images/{sha1}/boxes/{task}", Handler: s.handleUpdateAttributes},
		{Method: "GET", Path: "/packages/{name}/boxes", Handler: s.handlePackageBoxes},
		{Method: "GET", Path: "/boxes/stats", Handler: s.handleLabelStats},
		{Method: "GET", Path: "/boxes/packages", Handler: s.handlePackageTasks},
		{Method: "GET", Path: "/packages/{name}/cvat.zip", Handler: s.handleCVATZip},
		{Method: "GET", Path: "/boxes/images.zip", Handler: s.handleImagesZip},
	}
}

const maxTaskBytes = 64 << 10

var sha1Re = regexp.MustCompile(`^[0-9a-f]{40}$`)

// TaskView é a tarefa com o resumo.
type TaskView struct {
	Task
	Stats *Stats `json:"stats"`
}

// writeErr traduz os erros do serviço: validação 400, não existe 404,
// nome repetido 409; o resto é erro interno.
func writeErr(w http.ResponseWriter, err error, what string) {
	var invalid *InvalidError
	var bad *BadFileError
	switch {
	case errors.As(err, &invalid):
		platform.WriteError(w, http.StatusBadRequest, invalid.Msg)
	case errors.As(err, &bad):
		platform.WriteError(w, http.StatusBadRequest, bad.Error())
	case errors.Is(err, platform.ErrNotFound):
		platform.WriteError(w, http.StatusNotFound, fmt.Sprintf("tarefa %q não existe", what))
	case errors.Is(err, platform.ErrDuplicate):
		platform.WriteError(w, http.StatusConflict, fmt.Sprintf("já existe uma tarefa %q", what))
	default:
		platform.ServerError(w, err)
	}
}

func decodeTask(w http.ResponseWriter, r *http.Request) (TaskInput, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxTaskBytes)
	var in TaskInput
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		platform.WriteError(w, http.StatusBadRequest, "JSON inválido: "+err.Error())
		return in, false
	}
	return in, true
}

// GET /tasks
func (s *Service) handleListTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.repo.ListTasks(r.Context())
	if err != nil {
		platform.ServerError(w, err)
		return
	}
	items := make([]TaskView, 0, len(tasks))
	for _, t := range tasks {
		st, err := s.Stats(r.Context(), t.Name)
		if err != nil {
			platform.ServerError(w, err)
			return
		}
		items = append(items, TaskView{Task: t, Stats: st})
	}
	platform.WriteJSON(w, http.StatusOK, platform.Page[TaskView]{Total: int64(len(items)), Items: items})
}

// GET /tasks/{name}
func (s *Service) handleGetTask(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	t, err := s.repo.FindTask(r.Context(), name)
	if err != nil {
		writeErr(w, err, name)
		return
	}
	st, err := s.Stats(r.Context(), t.Name)
	if err != nil {
		platform.ServerError(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, TaskView{Task: *t, Stats: st})
}

// POST /tasks
func (s *Service) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	in, ok := decodeTask(w, r)
	if !ok {
		return
	}
	t, err := s.CreateTask(r.Context(), in, platform.UserFrom(r))
	if err != nil {
		writeErr(w, err, strings.TrimSpace(in.Name))
		return
	}
	platform.WriteJSON(w, http.StatusCreated, t)
}

// PUT /tasks/{name}
func (s *Service) handleReplaceTask(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := s.repo.FindTask(r.Context(), name); err != nil {
		writeErr(w, err, name)
		return
	}
	in, ok := decodeTask(w, r)
	if !ok {
		return
	}
	t, err := s.UpdateTask(r.Context(), name, in)
	if err != nil {
		what := strings.TrimSpace(in.Name)
		if errors.Is(err, platform.ErrNotFound) {
			what = name
		}
		writeErr(w, err, what)
		return
	}
	platform.WriteJSON(w, http.StatusOK, t)
}

// DELETE /tasks/{name}
func (s *Service) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	n, err := s.DeleteTask(r.Context(), name)
	if err != nil {
		writeErr(w, err, name)
		return
	}
	platform.WriteJSON(w, http.StatusOK, map[string]any{"deleted": name, "images": n})
}

// POST /tasks/{name}/import?dry_run=true&negatives=false   (multipart, campo "file")
// dry_run=true (padrão): só o relatório. dry_run=false: grava.
// negatives=true: imagens do arquivo sem nenhuma caixa entram como anotadas
// e vazias (ver match). Labels que a tarefa ainda não tem são acrescentadas.
func (s *Service) handleImportTask(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := s.repo.FindTask(r.Context(), name); err != nil {
		writeErr(w, err, name)
		return
	}
	q := r.URL.Query()
	fileName, data, ok := readUploadedFile(w, r, MaxFileBytes)
	if !ok {
		return
	}
	rep, err := s.Import(r.Context(), name, fileName, data, q.Get("dry_run") != "false", q.Get("negatives") == "true", platform.UserFrom(r))
	if err != nil {
		writeErr(w, err, name)
		return
	}
	platform.WriteJSON(w, http.StatusOK, rep)
}

// GET /images/{sha1}/boxes
// As caixas da imagem em cada tarefa ({"items": [...]}, vazio se nenhuma).
func (s *Service) handleImageBoxes(w http.ResponseWriter, r *http.Request) {
	sha := r.PathValue("sha1")
	if !sha1Re.MatchString(sha) {
		platform.WriteError(w, http.StatusBadRequest, "sha1 inválido")
		return
	}
	ok, err := s.images.Exists(r.Context(), sha)
	if err != nil {
		platform.ServerError(w, err)
		return
	}
	if !ok {
		platform.WriteError(w, http.StatusNotFound, "imagem não encontrada")
		return
	}
	sets, err := s.repo.FindImageSets(r.Context(), sha)
	if err != nil {
		platform.ServerError(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, map[string]any{"items": sets})
}

// PUT /images/{sha1}/boxes/{task}   (X-User obrigatório)
// {"version": "<updated_at ou imported_at lido>", "boxes": [{"attributes": {...}}, ...]}
// Edita os atributos das caixas da imagem na tarefa (uma entrada por caixa,
// na ordem do GET). 400 com "problems" se um atributo não passa no tipo de
// objeto do perfil; 409 se as caixas mudaram desde a leitura.
func (s *Service) handleUpdateAttributes(w http.ResponseWriter, r *http.Request) {
	sha, task := r.PathValue("sha1"), r.PathValue("task")
	user := r.Header.Get("X-User")
	if user == "" {
		platform.WriteError(w, http.StatusBadRequest, "informe quem está editando no header X-User")
		return
	}
	if !sha1Re.MatchString(sha) {
		platform.WriteError(w, http.StatusBadRequest, "sha1 inválido")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAttributesBytes)
	var in AttributesInput
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		platform.WriteError(w, http.StatusBadRequest, `JSON inválido: esperado {"version": "...", "boxes": [{"attributes": {...}}]} (`+err.Error()+")")
		return
	}
	set, err := s.UpdateAttributes(r.Context(), sha, task, in, user)
	var problems *annotation.InvalidError
	var invalid *InvalidError
	switch {
	case errors.As(err, &problems):
		platform.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": problems.Msg, "problems": problems.Problems})
	case errors.As(err, &invalid):
		platform.WriteError(w, http.StatusBadRequest, invalid.Msg)
	case errors.Is(err, platform.ErrNotFound):
		platform.WriteError(w, http.StatusNotFound, fmt.Sprintf("a imagem não tem caixas na tarefa %q", task))
	case errors.Is(err, ErrConflict):
		platform.WriteError(w, http.StatusConflict, "as caixas mudaram (outra edição ou um novo import do CVAT): recarregue e edite de novo")
	case err != nil:
		platform.ServerError(w, err)
	default:
		platform.WriteJSON(w, http.StatusOK, set)
	}
}

// maxAttributesBytes: os atributos de todas as caixas de uma imagem.
const maxAttributesBytes = 1 << 20

// GET /packages/{name}/boxes
// Por tarefa, quantas imagens do pacote estão anotadas nela.
func (s *Service) handlePackageBoxes(w http.ResponseWriter, r *http.Request) {
	pkg, ok := s.packages.PackageByName(w, r, r.PathValue("name"))
	if !ok {
		return
	}
	counts, err := s.repo.PackageCounts(r.Context(), pkg.Name)
	if err != nil {
		platform.ServerError(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, map[string]any{"package": pkg.Name, "tasks": counts})
}

// readUploadedFile lê o campo "file" de um multipart, com limite.
func readUploadedFile(w http.ResponseWriter, r *http.Request, limit int64) (string, []byte, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, limit+1<<20)
	mr, err := r.MultipartReader()
	if err != nil {
		platform.WriteError(w, http.StatusBadRequest, `esperado multipart/form-data com o campo "file"`)
		return "", nil, false
	}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			platform.WriteError(w, http.StatusBadRequest, `faltou o campo "file" com o export do CVAT (zip ou .json)`)
			return "", nil, false
		}
		if err != nil {
			platform.WriteError(w, http.StatusBadRequest, "erro lendo o upload: "+err.Error())
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
			platform.WriteError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("arquivo maior que %d MB", limit>>20))
			return "", nil, false
		}
		if err != nil {
			platform.WriteError(w, http.StatusBadRequest, "erro lendo o arquivo: "+err.Error())
			return "", nil, false
		}
		return name, data, true
	}
}

// GET /packages/{name}/cvat.zip
// As imagens do pacote, com os nomes guardados no SIDM (começam pelo sha1),
// para criar a task no CVAT. Sem compressão: JPEG já é comprimido, e assim o
// zip sai rápido. É gerado enquanto é baixado (não ocupa disco).
func (s *Service) handleCVATZip(w http.ResponseWriter, r *http.Request) {
	pkg, ok := s.packages.PackageByName(w, r, r.PathValue("name"))
	if !ok {
		return
	}
	s.writeZip(w, r, []string{pkg.Name}, pkg.Name+"-cvat.zip")
}

// GET /boxes/images.zip?package=a&package=b
// O mesmo zip, com as imagens de vários pacotes (para rotular de uma vez).
// Os nomes começam pelo sha1, então não se repetem entre pacotes.
func (s *Service) handleImagesZip(w http.ResponseWriter, r *http.Request) {
	var names []string
	for _, name := range r.URL.Query()["package"] {
		if name = strings.TrimSpace(name); name == "" || slices.Contains(names, name) {
			continue
		}
		pkg, ok := s.packages.PackageByName(w, r, name)
		if !ok {
			return
		}
		names = append(names, pkg.Name)
	}
	if len(names) == 0 {
		platform.WriteError(w, http.StatusBadRequest, "escolha pelo menos um pacote (?package=)")
		return
	}
	file := names[0] + "-imagens.zip"
	if len(names) > 1 {
		file = fmt.Sprintf("%d-pacotes-imagens.zip", len(names))
	}
	s.writeZip(w, r, names, file)
}

// writeZip manda as imagens dos pacotes num zip, gerado enquanto é baixado.
func (s *Service) writeZip(w http.ResponseWriter, r *http.Request, packages []string, file string) {
	ctx := r.Context()
	// Confere antes de começar a mandar: depois do primeiro byte, não dá
	// mais para responder com erro.
	found := false
	for _, name := range packages {
		if _, total, err := s.images.InPackage(ctx, name, 1, 0); err != nil {
			platform.ServerError(w, err)
			return
		} else if total > 0 {
			found = true
		}
	}
	if !found {
		msg := "o pacote não tem imagens"
		if len(packages) > 1 {
			msg = "os pacotes não têm imagens"
		}
		platform.WriteError(w, http.StatusNotFound, msg)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, file))
	zw := zip.NewWriter(w)
	var missing []string
	const page = 500
	for _, name := range packages {
		for skip := int64(0); ; skip += page {
			imgs, _, err := s.images.InPackage(ctx, name, page, skip)
			if err != nil {
				platform.LogError(fmt.Errorf("zip de %s: %w", name, err))
				missing = append(missing, "(erro listando as imagens de "+name+": o zip pode estar incompleto)")
				break
			}
			for _, img := range imgs {
				if err := s.addToZip(zw, img); err != nil {
					platform.LogError(fmt.Errorf("zip de %s: %w", name, err))
					missing = append(missing, img.Path)
				}
			}
			if len(imgs) < page {
				break
			}
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		if fw, err := zw.Create("FALTANDO.txt"); err == nil {
			fmt.Fprintf(fw, "Estas imagens não puderam ser incluídas:\r\n%s\r\n", strings.Join(missing, "\r\n"))
		}
	}
	if err := zw.Close(); err != nil {
		platform.LogError(fmt.Errorf("zip de %s: %w", file, err))
	}
}

func (s *Service) addToZip(zw *zip.Writer, img ImageRef) error {
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

// GET /boxes/packages
// {"items": {"<pacote>": ["<tarefa>", ...]}}: para a lista de pacotes filtrar
// e ordenar por tarefa sem um pedido por pacote.
func (s *Service) handlePackageTasks(w http.ResponseWriter, r *http.Request) {
	items, err := s.repo.PackageTasks(r.Context())
	if err != nil {
		platform.ServerError(w, err)
		return
	}
	platform.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}
