package importer

import (
	"bufio"
	"bytes"
	"sort"
	"strings"
	"unicode/utf8"
)

// Equipamentos de fiscalização costumam gravar um INI DEPOIS do fim do
// JPEG (marcador FF D9). Visualizadores ignoram o que vem depois do fim, então
// a imagem abre normal e o texto fica "escondido" no mesmo arquivo:
//
//	FF D8 ... (imagem) ... FF D9 \n[Metrologico]\nData=18/09/2026\n...
//
// O arquivo NUNCA é alterado pelo SIDM (o INI costuma ser assinado
// digitalmente). Aqui só lemos uma cópia do texto.

const (
	ReaderJPEGINI = "jpeg-ini"

	maxINIBytes = 256 << 10 // um INI maior que isso não é um INI
)

// Chaves que não vão para o banco. A assinatura é grande e só faz sentido
// junto com o arquivo original, que continua intacto no disco.
var skippedINIKeys = map[string]bool{"assinatura": true}

// INIEntry é uma linha do INI. Guardamos como lista (e não como mapa) porque
// a ordem importa para quem lê, e porque chaves com "." ou "$" seriam
// problema como nome de campo no Mongo.
type INIEntry struct {
	Section string `json:"section" bson:"s"`
	Key     string `json:"key" bson:"k"`
	Value   string `json:"value" bson:"v"`
}

// ExtractJPEGTrailer devolve o texto gravado depois do fim da imagem, ou nil
// se não houver nada (ou se o arquivo nem for JPEG).
//
// Alguns equipamentos gravam DUAS imagens no mesmo arquivo, cada uma com o
// seu bloco de INI:
//
//	[JPEG 1] FF D9 [Metrologico]...Assinatura=... [Imagem2] [JPEG 2] FF D9 [NMetrologico]...
//
// Por isso o texto depois de CADA fim de imagem é lido, e os blocos são
// juntados na ordem do arquivo. Um bloco vai até o próximo byte FF (texto,
// em UTF-8 ou Latin-1, nunca tem FF) e só conta se for texto com cara de INI:
// o que vem depois do fim de uma miniatura (EXIF) é binário e fica de fora.
func ExtractJPEGTrailer(data []byte) []byte {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return nil
	}
	var out []byte
	for i := 0; i+1 < len(data); i++ {
		if data[i] != 0xFF || data[i+1] != 0xD9 {
			continue
		}
		rest := data[i+2:]
		if n := bytes.IndexByte(rest, 0xFF); n >= 0 {
			rest = rest[:n]
		}
		i += 1 + len(rest)
		block := iniBlock(rest)
		if block == nil {
			continue
		}
		if len(out) > 0 {
			out = append(out, '\n')
		}
		out = append(out, block...)
		if len(out) > maxINIBytes {
			return nil
		}
	}
	return out
}

// iniBlock limpa um bloco de texto depois de um fim de imagem: nil se ele
// estiver vazio ou não parecer INI (binário, sem "=" nem "[").
func iniBlock(b []byte) []byte {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || !bytes.ContainsAny(b, "=[") {
		return nil
	}
	for _, c := range b {
		if c < 0x20 && c != '\n' && c != '\r' && c != '\t' { // byte de controle: é binário
			return nil
		}
	}
	if !utf8.Valid(b) { // equipamento antigo gravando em Latin-1
		runes := make([]rune, len(b))
		for i, c := range b {
			runes[i] = rune(c)
		}
		b = []byte(string(runes))
	}
	return b
}

// ParseINI lê "[Secao]" e "Chave = Valor" (com ou sem espaços em volta do =).
// Espaços sobrando nos valores ("AUTOMOVEL     ") são removidos. Linhas de
// comentário (; ou #) e linhas sem "=" são ignoradas.
func ParseINI(text []byte) []INIEntry {
	var out []INIEntry
	section := ""

	sc := bufio.NewScanner(bytes.NewReader(text))
	sc.Buffer(make([]byte, 64<<10), maxINIBytes)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "" || line[0] == ';' || line[0] == '#':
			continue
		case line[0] == '[' && line[len(line)-1] == ']':
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" || skippedINIKeys[strings.ToLower(key)] {
			continue
		}
		out = append(out, INIEntry{Section: section, Key: key, Value: strings.TrimSpace(value)})
	}
	return out
}

// INIValues transforma as entradas em "colunas" no formato "Secao.Chave",
// para o perfil usar igual a uma coluna de CSV. Chave sem seção fica só "Chave".
// Chave repetida na mesma seção: vale a última.
func INIValues(entries []INIEntry) map[string]string {
	out := make(map[string]string, len(entries))
	for _, e := range entries {
		name := e.Key
		if e.Section != "" {
			name = e.Section + "." + e.Key
		}
		out[name] = e.Value
	}
	return out
}

// INITable monta uma tabela de uma linha (uma imagem) a partir do INI, com
// uma coluna extra de identificação, para passar pelo mesmo Transform do CSV.
func INITable(keyColumn, key string, entries []INIEntry) *Table {
	values := INIValues(entries)
	values[keyColumn] = key

	cols := make([]string, 0, len(values))
	for c := range values {
		cols = append(cols, c)
	}
	sort.Strings(cols)
	return &Table{Columns: cols, Rows: []Row{{Line: 1, Values: values}}}
}
