package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"SIDM/internal/filename"
	"SIDM/internal/importer"
	"SIDM/internal/store"
)

// Limites de tamanho dos arquivos de import.
const (
	maxImportFileBytes = 20 << 20  // 20 MB de CSV
	maxProfileBytes    = 256 << 10 // 256 KB de JSON de perfil
	maxListedKeys      = 100       // quantas chaves "não encontradas" listar no relatório
)

// ---------- perfis ----------

// POST /import-profiles  (corpo: o perfil em JSON)
func (s *Server) handleCreateProfile(w http.ResponseWriter, r *http.Request) {
	p, ok := decodeProfile(w, r)
	if !ok {
		return
	}
	p.CreatedBy = userFrom(r)
	p.UpdatedAt = time.Now().UTC()

	if err := s.store.CreateProfile(r.Context(), p); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			writeError(w, http.StatusConflict, fmt.Sprintf("perfil %q já existe: para alterar, use PUT /import-profiles/%s", p.Name, p.Name))
			return
		}
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// PUT /import-profiles/{name}
// O fluxo típico: o ensaio mostrou "Garoa" sem tradução, você adiciona no
// map e manda o perfil de novo.
func (s *Server) handleReplaceProfile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	p, ok := decodeProfile(w, r)
	if !ok {
		return
	}
	if p.Name != name {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("o nome no corpo (%q) é diferente do nome na URL (%q)", p.Name, name))
		return
	}
	p.UpdatedAt = time.Now().UTC()

	if err := s.store.ReplaceProfile(r.Context(), p); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, fmt.Sprintf("perfil %q não existe: crie com POST /import-profiles", name))
			return
		}
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// PATCH /import-profiles/{name}   {"name": "novo-nome"}
// Só renomeia. Para mudar o conteúdo, use o PUT com o JSON inteiro.
func (s *Server) handleRenameProfile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
	var body struct {
		Name string `json:"name"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, `JSON inválido: esperado {"name": "..."} (para mudar o conteúdo, use o PUT)`)
		return
	}
	newName := strings.TrimSpace(body.Name)
	if !importer.ValidProfileName(newName) {
		writeError(w, http.StatusBadRequest, `name: use só letras, números e "-" (ex: csv-equipamento-8003)`)
		return
	}
	if newName == name {
		writeError(w, http.StatusBadRequest, "o nome novo é igual ao atual")
		return
	}

	err := s.store.RenameProfile(r.Context(), name, newName, time.Now().UTC())
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, fmt.Sprintf("perfil %q não existe", name))
		return
	case errors.Is(err, store.ErrDuplicate):
		writeError(w, http.StatusConflict, fmt.Sprintf("já existe um perfil %q", newName))
		return
	case err != nil:
		s.serverError(w, err)
		return
	}
	p, ok := s.profileByName(w, r, newName)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// DELETE /import-profiles/{name}
// Nenhuma imagem depende do perfil: os rascunhos que ele já preencheu ficam.
func (s *Server) handleDeleteProfile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	err := s.store.DeleteProfile(r.Context(), name)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("perfil %q não existe", name))
		return
	}
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "deleted": true})
}

// decodeProfile lê e VALIDA o perfil. Os erros de validação vão todos juntos,
// para a pessoa corrigir o JSON de uma vez.
func decodeProfile(w http.ResponseWriter, r *http.Request) (*importer.Profile, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxProfileBytes)
	var p importer.Profile
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields() // "colum" em vez de "column" vira erro, não é ignorado
	if err := dec.Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "JSON inválido: "+err.Error())
		return nil, false
	}
	if err := p.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return nil, false
	}
	return &p, true
}

// GET /import-profiles?q=&limit=&skip=
func (s *Server) handleListProfiles(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	profiles, total, err := s.store.ListProfiles(r.Context(), strings.TrimSpace(q.Get("q")),
		intParam(q, "limit", 20, 1, 100),
		intParam(q, "skip", 0, 0, 1<<31))
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page[importer.Profile]{Total: total, Items: profiles})
}

// GET /import-profiles/{name}
func (s *Server) handleGetProfile(w http.ResponseWriter, r *http.Request) {
	p, ok := s.profileByName(w, r, r.PathValue("name"))
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) profileByName(w http.ResponseWriter, r *http.Request, name string) (*importer.Profile, bool) {
	p, err := s.store.FindProfile(r.Context(), name)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("perfil %q não existe", name))
		return nil, false
	}
	if err != nil {
		s.serverError(w, err)
		return nil, false
	}
	return p, true
}

// ---------- inspeção ----------

// POST /imports/inspect?delimiter=semicolon&encoding=latin1   (multipart, campo "file")
// O "raio-x" do arquivo, para escrever um perfil novo: colunas, linhas de
// exemplo e os valores distintos de cada coluna. Não precisa de perfil.
func (s *Server) handleInspect(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	delim, ok := delimiterParam(q.Get("delimiter"))
	if !ok {
		writeError(w, http.StatusBadRequest, `delimiter: use comma, semicolon, tab ou pipe`)
		return
	}
	cfg := importer.ReaderConfig{Type: "csv", Delimiter: delim, Encoding: q.Get("encoding")}

	table, ok := s.readImportFile(w, r, cfg)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, importer.Inspect(table, 5, 50))
}

// delimiterParam aceita o separador por NOME na URL.
//
// Por que não "?delimiter=;"? Desde o Go 1.17, um ";" cru na query string é
// inválido e a variável é descartada sem aviso. Os nomes funcionam em
// qualquer URL. (Com o caractere codificado, "%3B", também funciona.)
func delimiterParam(v string) (string, bool) {
	switch strings.ToLower(v) {
	case "", "comma", ",":
		return "", true // padrão do leitor: ","
	case "semicolon", ";":
		return ";", true
	case "tab", "\t":
		return "\t", true
	case "pipe", "|":
		return "|", true
	}
	return "", false
}

// readImportFile lê o campo "file" do multipart como tabela.
func (s *Server) readImportFile(w http.ResponseWriter, r *http.Request, cfg importer.ReaderConfig) (*importer.Table, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImportFileBytes)

	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, `esperado multipart/form-data com o campo "file"`)
		return nil, false
	}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			writeError(w, http.StatusBadRequest, `faltou o campo "file" com o CSV`)
			return nil, false
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, "erro lendo o upload: "+err.Error())
			return nil, false
		}
		if part.FormName() != "file" {
			part.Close()
			continue
		}

		table, err := importer.ReadCSV(part, cfg)
		part.Close()
		if err != nil {
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("arquivo maior que %d MB: divida em partes", maxImportFileBytes>>20))
				return nil, false
			}
			writeError(w, http.StatusBadRequest, "erro lendo o CSV: "+err.Error())
			return nil, false
		}
		return table, true
	}
}

// ---------- import ----------

// importReport é a resposta do import (e do ensaio).
type importReport struct {
	DryRun    bool   `json:"dry_run"`
	Package   string `json:"package"`
	Profile   string `json:"profile"`
	Overwrite bool   `json:"overwrite"`

	importer.Report // linhas lidas, valores sem tradução, erros de data...

	WithoutINI      int      `json:"without_ini"`      // perfil de INI: imagens pendentes sem INI embutido
	Matched         int      `json:"matched"`          // linhas que acharam a imagem
	NotFound        []string `json:"not_found"`        // chaves sem imagem no pacote (até 100)
	NotFoundCount   int      `json:"not_found_count"`  //
	Ambiguous       []string `json:"ambiguous"`        // nome original repetido no pacote: não aplicado
	SkippedVerified int      `json:"skipped_verified"` // já verificadas: o import não mexe
	SkippedClaimed  int      `json:"skipped_claimed"`  // alguém está anotando agora
	Unchanged       int      `json:"unchanged"`        // o import não tinha nada novo
	Applied         int      `json:"applied"`          // rascunhos atualizados (no ensaio: "seriam")
	Conflicts       int      `json:"conflicts"`        // alterada por alguém durante o import

	// Equipamentos e câmeras do CSV que ainda não existiam no cadastro.
	// Ao aplicar, são CRIADOS automaticamente; no ensaio, só listados.
	NewEquipments []string `json:"new_equipments"`
	NewCameras    []string `json:"new_cameras"`
	// Nomes que não dá para sanitizar (ex: "8003/MF"): não são criados, e o
	// campo fica vazio na imagem. Corrija no CSV ou use um "map" no perfil.
	InvalidNames []RowName `json:"invalid_names"`
}

type RowName struct {
	Line  int    `json:"line"`
	Field string `json:"field"`
	Value string `json:"value"`
	Error string `json:"error"`
}

// POST /packages/{name}/import?profile=X&dry_run=true&overwrite=false
// (multipart, campo "file"; com perfil do tipo jpeg-ini, SEM arquivo: usa o
// INI já guardado de cada imagem pendente do pacote)
//
// Casa cada linha do CSV com uma imagem do pacote e preenche o RASCUNHO.
// Nunca verifica: uma pessoa continua conferindo pela fila.
// Equipamentos e câmeras novos do CSV são cadastrados automaticamente.
//
//	dry_run=true (PADRÃO): só o relatório, nada é gravado. Rode primeiro assim.
//	overwrite=false (padrão): só preenche campos vazios; o que alguém já
//	                          anotou é mantido.
//	overwrite=true: o valor do CSV substitui o do rascunho.
func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	pkg := r.PathValue("name")
	dryRun := q.Get("dry_run") != "false" // por segurança, só grava se pedir explicitamente
	overwrite := q.Get("overwrite") == "true"

	if q.Get("profile") == "" {
		writeError(w, http.StatusBadRequest, "informe o perfil: ?profile=<nome>")
		return
	}
	if _, err := s.store.FindPackage(r.Context(), pkg); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, fmt.Sprintf("pacote %q não existe", pkg))
			return
		}
		s.serverError(w, err)
		return
	}
	profile, ok := s.profileByName(w, r, q.Get("profile"))
	if !ok {
		return
	}

	rep := importReport{
		DryRun: dryRun, Package: pkg, Profile: profile.Name, Overwrite: overwrite,
		Report:   importer.Report{Unmapped: map[string]map[string]int{}, Errors: []importer.RowError{}},
		NotFound: []string{}, Ambiguous: []string{},
		NewEquipments: []string{}, NewCameras: []string{}, InvalidNames: []RowName{},
	}

	var records []importer.Record
	var byKey map[string][]store.Image

	if profile.Reader.Type == importer.ReaderJPEGINI {
		// Perfil de INI: sem arquivo; usa o INI já guardado de cada imagem pendente.
		if records, byKey, ok = s.iniRecords(w, r, pkg, profile, &rep); !ok {
			return
		}
	} else {
		table, ok := s.readImportFile(w, r, profile.Reader)
		if !ok {
			return
		}
		if len(table.Rows) > 0 && !hasColumn(table, profile.Match.Column) {
			writeError(w, http.StatusBadRequest, fmt.Sprintf(
				"o CSV não tem a coluna %q que o perfil usa para achar a imagem (colunas: %s)",
				profile.Match.Column, strings.Join(table.Columns, ", ")))
			return
		}

		records, rep.Report = importer.Transform(profile, table)

		// Busca todas as imagens de uma vez (em lotes), não uma consulta por linha.
		keys := make([]string, len(records))
		for i, rec := range records {
			keys[i] = rec.Key
		}
		imgs, err := s.store.FindImagesByKeys(r.Context(), pkg, profile.Match.By, keys)
		if err != nil {
			s.serverError(w, err)
			return
		}
		byKey = map[string][]store.Image{}
		for _, img := range imgs {
			k := img.OriginalName
			if profile.Match.By == importer.MatchBySHA1 {
				k = img.SHA1
			}
			byKey[k] = append(byKey[k], img)
		}
	}

	user := userFrom(r)
	changedBy := fmt.Sprintf("%s via import:%s", user, profile.Name)
	now := time.Now().UTC()
	equipments, cameras := map[string]bool{}, map[string]bool{}

	// 1ª passada: decide o que muda em cada imagem, SEM gravar nada.
	type write struct{ before, after store.Image }
	var writes []write

	for _, rec := range records {
		found := byKey[rec.Key]
		switch {
		case len(found) == 0:
			rep.NotFoundCount++
			if len(rep.NotFound) < maxListedKeys {
				rep.NotFound = append(rep.NotFound, rec.Key)
			}
			continue
		case len(found) > 1:
			rep.Ambiguous = append(rep.Ambiguous, rec.Key)
			continue
		}
		rep.Matched++
		img := found[0]

		if img.Status == store.StatusVerified {
			rep.SkippedVerified++
			continue
		}
		if img.ClaimedByOther(user, now) {
			rep.SkippedClaimed++
			continue
		}

		ann := rec.Annotation
		ann.Equipment = rep.sanitizeName(rec.Line, "equipment", ann.Equipment, filename.SanitizeEquipment)
		ann.Camera = rep.sanitizeName(rec.Line, "camera", ann.Camera, filename.SanitizeCamera)

		before := img
		if !mergeAnnotation(&img, ann, overwrite) {
			rep.Unchanged++
			continue
		}
		if img.Equipment != "" {
			equipments[img.Equipment] = true
		}
		if img.Camera != "" {
			cameras[img.Camera] = true
		}
		writes = append(writes, write{before: before, after: img})
	}

	// 2ª passada: equipamentos e câmeras que ainda não existem.
	// Ao aplicar, são criados ANTES das imagens, para que nenhuma imagem
	// fique apontando para um cadastro que não existe.
	var ok2 bool
	if rep.NewEquipments, ok2 = s.registerMissing(w, r, sortedSet(equipments), dryRun,
		s.store.EquipmentExists, s.store.CreateEquipment); !ok2 {
		return
	}
	if rep.NewCameras, ok2 = s.registerMissing(w, r, sortedSet(cameras), dryRun,
		s.store.CameraExists, s.store.CreateCamera); !ok2 {
		return
	}

	if dryRun {
		rep.Applied = len(writes)
		writeJSON(w, http.StatusOK, rep)
		return
	}

	// 3ª passada: grava as imagens.
	for _, wr := range writes {
		img := wr.after
		img.UpdatedAt = now
		if err := s.store.ReplaceImage(r.Context(), &img, wr.before.UpdatedAt); err != nil {
			if errors.Is(err, store.ErrConflict) {
				rep.Conflicts++
				continue
			}
			s.serverError(w, err)
			return
		}
		rep.Applied++
		before := wr.before
		if err := s.store.InsertHistory(r.Context(), diffHistory(&before, &img, changedBy)); err != nil {
			s.logError(err)
		}
	}

	writeJSON(w, http.StatusOK, rep)
}

// sanitizeName passa o nome de equipamento/câmera do CSV pela mesma
// sanitização do cadastro manual ("8003 MF4739" -> "8003-MF4739").
// Nome impossível de sanitizar: anota no relatório e devolve "".
func (rep *importReport) sanitizeName(line int, field, value string, sanitize func(string) (string, error)) string {
	if value == "" {
		return ""
	}
	name, err := sanitize(value)
	if err != nil {
		if len(rep.InvalidNames) < maxListedKeys {
			rep.InvalidNames = append(rep.InvalidNames, RowName{Line: line, Field: field, Value: value, Error: err.Error()})
		}
		return ""
	}
	return name
}

// registerMissing devolve os nomes que ainda não estão cadastrados e, fora
// do ensaio, cadastra cada um. ErrDuplicate é ignorado: outra pessoa pode
// ter cadastrado o mesmo nome no meio do import, e o resultado é o mesmo.
func (s *Server) registerMissing(
	w http.ResponseWriter, r *http.Request, names []string, dryRun bool,
	exists func(ctx context.Context, name string) (bool, error),
	create func(ctx context.Context, name string) error,
) ([]string, bool) {
	missing := []string{}
	for _, name := range names {
		ok, err := exists(r.Context(), name)
		if err != nil {
			s.serverError(w, err)
			return nil, false
		}
		if ok {
			continue
		}
		missing = append(missing, name)
		if dryRun {
			continue
		}
		if err := create(r.Context(), name); err != nil && !errors.Is(err, store.ErrDuplicate) {
			s.serverError(w, err)
			return nil, false
		}
	}
	return missing, true
}

func hasColumn(t *importer.Table, col string) bool {
	for _, c := range t.Columns {
		if c == col {
			return true
		}
	}
	return false
}

func sortedSet(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// mergeAnnotation aplica o que o import descobriu no rascunho da imagem.
// overwrite=false: só preenche o que está vazio (respeita quem já anotou).
// Devolve true se algo mudou.
func mergeAnnotation(img *store.Image, a importer.Annotation, overwrite bool) bool {
	changed := false
	set := func(dst *string, v string) {
		if v != "" && *dst != v && (overwrite || *dst == "") {
			*dst = v
			changed = true
		}
	}
	set(&img.Equipment, a.Equipment)
	set(&img.Camera, a.Camera)
	set(&img.Direction, a.Direction)
	set(&img.Resolution, a.Resolution)
	set(&img.CaptureAngle, a.CaptureAngle)
	set(&img.DayTime, a.DayTime)
	set(&img.ImageRange, a.ImageRange)
	set(&img.Weather, a.Weather)

	if len(a.Conditions) > 0 && (overwrite || len(img.Conditions) == 0) && !equalStrings(img.Conditions, a.Conditions) {
		img.Conditions = append([]string(nil), a.Conditions...)
		changed = true
	}
	if a.CapturedAt != nil && (overwrite || img.CapturedAt == nil) &&
		(img.CapturedAt == nil || !img.CapturedAt.Equal(*a.CapturedAt)) {
		t := *a.CapturedAt
		img.CapturedAt = &t
		changed = true
	}
	if len(a.Vehicles) > 0 && (overwrite || len(img.Vehicles) == 0) {
		vehicles := make([]store.Vehicle, len(a.Vehicles))
		for i, v := range a.Vehicles {
			vehicles[i] = store.Vehicle{
				Plate: v.Plate, PlateType: v.PlateType, PlateCategory: v.PlateCategory,
				Type: v.Type, Color: v.Color,
			}
		}
		if !equalVehicles(img.Vehicles, vehicles) {
			img.Vehicles = vehicles
			changed = true
		}
	}
	return changed
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalVehicles(a, b []store.Vehicle) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
