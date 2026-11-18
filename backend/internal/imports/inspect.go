package imports

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"SIDM/internal/crypt"
	"SIDM/internal/images"
	"SIDM/internal/importer"
	"SIDM/internal/platform"
)

// Limites do "raio-x" de INI (POST /imports/inspect com JPEGs).
const (
	MaxInspectINIFiles = 20
	maxInspectINIBytes = 200 << 20
)

// POST /imports/inspect?delimiter=semicolon&encoding=latin1   (multipart, campo "file")
// O "raio-x" do arquivo, para escrever um perfil novo: colunas, linhas de
// exemplo e os valores distintos de cada coluna. Não precisa de perfil.
func (s *Service) handleInspect(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("type") == importer.ReaderJPEGINI {
		inspectINI(w, r)
		return
	}
	delim, ok := delimiterParam(q.Get("delimiter"))
	if !ok {
		platform.WriteError(w, http.StatusBadRequest, `delimiter: use comma, semicolon, tab ou pipe`)
		return
	}
	cfg := importer.ReaderConfig{Type: "csv", Delimiter: delim, Encoding: q.Get("encoding")}

	table, ok := readImportFile(w, r, cfg)
	if !ok {
		return
	}
	platform.WriteJSON(w, http.StatusOK, importer.Inspect(table, 5, 50))
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
func readImportFile(w http.ResponseWriter, r *http.Request, cfg importer.ReaderConfig) (*importer.Table, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImportFileBytes)

	mr, err := r.MultipartReader()
	if err != nil {
		platform.WriteError(w, http.StatusBadRequest, `esperado multipart/form-data com o campo "file"`)
		return nil, false
	}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			platform.WriteError(w, http.StatusBadRequest, `faltou o campo "file" com o CSV`)
			return nil, false
		}
		if err != nil {
			platform.WriteError(w, http.StatusBadRequest, "erro lendo o upload: "+err.Error())
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
				platform.WriteError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("arquivo maior que %d MB: divida em partes", maxImportFileBytes>>20))
				return nil, false
			}
			platform.WriteError(w, http.StatusBadRequest, "erro lendo o CSV: "+err.Error())
			return nil, false
		}
		return table, true
	}
}

// inspectINI: POST /imports/inspect?type=jpeg-ini (multipart, um ou mais "file")
// O mesmo raio-x do CSV, mas lendo o INI embutido em algumas imagens de
// exemplo: cada imagem vira uma linha, e cada "Secao.Chave" vira uma coluna.
// É o que o perfil de INI usa como "coluna". Imagens criptografadas são
// abertas antes, igual ao upload. Nada é gravado.
func inspectINI(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxInspectINIBytes)
	mr, err := r.MultipartReader()
	if err != nil {
		platform.WriteError(w, http.StatusBadRequest, `esperado multipart/form-data com uma ou mais imagens no campo "file"`)
		return
	}

	var rows []importer.Row
	columns := map[string]bool{}
	files, withoutINI := 0, []string{}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			platform.WriteError(w, http.StatusBadRequest, "erro lendo o upload: "+err.Error())
			return
		}
		if part.FormName() != "file" {
			part.Close()
			continue
		}
		files++
		if files > MaxInspectINIFiles {
			part.Close()
			platform.WriteError(w, http.StatusBadRequest, fmt.Sprintf("mande no máximo %d imagens de exemplo", MaxInspectINIFiles))
			return
		}
		data, err := io.ReadAll(io.LimitReader(part, images.MaxINIScanBytes+1))
		name := part.FileName()
		part.Close()
		if err != nil {
			platform.WriteError(w, http.StatusBadRequest, "erro lendo "+name+": "+err.Error())
			return
		}
		if len(data) > images.MaxINIScanBytes {
			platform.WriteError(w, http.StatusBadRequest, fmt.Sprintf("%s: imagem maior que %d MB", name, images.MaxINIScanBytes>>20))
			return
		}
		if !crypt.IsJPEG(data) {
			if plain, _, err := crypt.Open(data); err == nil {
				data = plain
			}
		}
		values := importer.INIValues(importer.ParseINI(importer.ExtractJPEGTrailer(data)))
		if len(values) == 0 {
			withoutINI = append(withoutINI, name)
			continue
		}
		for c := range values {
			columns[c] = true
		}
		rows = append(rows, importer.Row{Line: len(rows) + 1, Values: values})
	}

	if files == 0 {
		platform.WriteError(w, http.StatusBadRequest, `faltou o campo "file" com pelo menos uma imagem`)
		return
	}
	if len(rows) == 0 {
		platform.WriteError(w, http.StatusBadRequest, "nenhuma das imagens tem INI embutido: "+strings.Join(withoutINI, ", "))
		return
	}

	cols := make([]string, 0, len(columns))
	for c := range columns {
		cols = append(cols, c)
	}
	sort.Strings(cols)
	platform.WriteJSON(w, http.StatusOK, importer.Inspect(&importer.Table{Columns: cols, Rows: rows}, 5, 50))
}
