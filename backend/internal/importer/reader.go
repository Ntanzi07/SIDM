package importer

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf8"
)

// Limites para um arquivo gigante (ou malicioso) não derrubar o servidor.
const (
	MaxRows    = 200_000
	maxColumns = 200
)

// Table é o arquivo já lido: cabeçalho + linhas como {coluna: valor}.
// Os leitores (csv hoje, ini/exif amanhã) só precisam produzir isto.
type Table struct {
	Columns []string
	Rows    []Row
}

// Row é uma linha. Line é o número da linha no arquivo (para o relatório).
type Row struct {
	Line   int
	Values map[string]string
}

// ReadCSV lê um CSV com cabeçalho na primeira linha.
func ReadCSV(r io.Reader, cfg ReaderConfig) (*Table, error) {
	r, err := decodeText(r, cfg.Encoding)
	if err != nil {
		return nil, err
	}

	cr := csv.NewReader(r)
	cr.Comma = ','
	if cfg.Delimiter != "" {
		cr.Comma, _ = utf8.DecodeRuneInString(cfg.Delimiter)
	}
	cr.FieldsPerRecord = -1 // linhas com menos colunas não quebram a leitura inteira
	cr.LazyQuotes = true    // Excel às vezes gera aspas "erradas"
	cr.TrimLeadingSpace = true

	header, err := cr.Read()
	if errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("arquivo vazio")
	}
	if err != nil {
		return nil, fmt.Errorf("ler cabeçalho: %w", err)
	}
	if len(header) > maxColumns {
		return nil, fmt.Errorf("arquivo com %d colunas (máx. %d)", len(header), maxColumns)
	}

	cols := make([]string, len(header))
	seen := map[string]bool{}
	for i, h := range header {
		h = strings.TrimSpace(h)
		if h == "" {
			h = fmt.Sprintf("coluna_%d", i+1)
		}
		if seen[h] {
			return nil, fmt.Errorf("coluna %q aparece duas vezes no cabeçalho", h)
		}
		seen[h] = true
		cols[i] = h
	}
	if len(cols) == 1 && cfg.Delimiter == "" && strings.ContainsAny(cols[0], ";\t|") {
		return nil, fmt.Errorf("só uma coluna encontrada (%q): o separador provavelmente não é \",\"; informe o delimiter", cols[0])
	}

	t := &Table{Columns: cols}
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		line, _ := cr.FieldPos(0)
		if err != nil {
			return nil, fmt.Errorf("linha %d: %w", line, err)
		}
		if isBlank(rec) {
			continue
		}
		if len(t.Rows) >= MaxRows {
			return nil, fmt.Errorf("arquivo com mais de %d linhas: divida em partes", MaxRows)
		}

		values := make(map[string]string, len(cols))
		for i, c := range cols {
			if i < len(rec) {
				values[c] = strings.TrimSpace(rec[i])
			}
		}
		t.Rows = append(t.Rows, Row{Line: line, Values: values})
	}
	return t, nil
}

func isBlank(rec []string) bool {
	for _, v := range rec {
		if strings.TrimSpace(v) != "" {
			return false
		}
	}
	return true
}

// decodeText converte para UTF-8 e remove o BOM que o Excel coloca no começo
// ("\xEF\xBB\xBF"), que senão gruda no nome da primeira coluna.
func decodeText(r io.Reader, encoding string) (io.Reader, error) {
	br := bufio.NewReader(r)
	if bom, _ := br.Peek(3); bytes.Equal(bom, []byte{0xEF, 0xBB, 0xBF}) {
		br.Discard(3)
	}

	switch strings.ToLower(encoding) {
	case "", "utf-8", "utf8":
		return br, nil
	case "latin1", "iso-8859-1":
		// Em Latin-1 cada byte É o código Unicode do caractere (0xE7 = 'ç').
		data, err := io.ReadAll(br)
		if err != nil {
			return nil, err
		}
		runes := make([]rune, len(data))
		for i, b := range data {
			runes[i] = rune(b)
		}
		return strings.NewReader(string(runes)), nil
	default:
		return nil, fmt.Errorf("encoding %q não suportado (use \"utf-8\" ou \"latin1\")", encoding)
	}
}

// Inspection é o "raio-x" de um arquivo, para quem vai escrever um perfil:
// as colunas, algumas linhas de exemplo e os valores distintos de cada coluna
// (é o que precisa ser traduzido no "map" do perfil).
type Inspection struct {
	Rows     int                       `json:"rows"`
	Columns  []string                  `json:"columns"`
	Sample   []map[string]string       `json:"sample"`
	Distinct map[string]DistinctValues `json:"distinct"`
}

type DistinctValues struct {
	Values    []ValueCount `json:"values"`    // os mais frequentes primeiro
	Total     int          `json:"total"`     // quantos valores distintos existem
	Truncated bool         `json:"truncated"` // true se Values não mostra todos
}

type ValueCount struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// Inspect resume a tabela. maxDistinct limita quantos valores distintos
// mostrar por coluna (uma coluna de placas teria milhares).
func Inspect(t *Table, sampleSize, maxDistinct int) Inspection {
	ins := Inspection{
		Rows:     len(t.Rows),
		Columns:  t.Columns,
		Sample:   []map[string]string{},
		Distinct: map[string]DistinctValues{},
	}
	for i := 0; i < len(t.Rows) && i < sampleSize; i++ {
		ins.Sample = append(ins.Sample, t.Rows[i].Values)
	}

	for _, col := range t.Columns {
		counts := map[string]int{}
		for _, row := range t.Rows {
			if v := row.Values[col]; v != "" {
				counts[v]++
			}
		}
		values := make([]ValueCount, 0, len(counts))
		for v, n := range counts {
			values = append(values, ValueCount{Value: v, Count: n})
		}
		sort.Slice(values, func(i, j int) bool {
			if values[i].Count != values[j].Count {
				return values[i].Count > values[j].Count
			}
			return values[i].Value < values[j].Value
		})

		d := DistinctValues{Total: len(values), Values: values}
		if len(values) > maxDistinct {
			d.Values, d.Truncated = values[:maxDistinct], true
		}
		ins.Distinct[col] = d
	}
	return ins
}
