package export

import (
	"path"
	"sort"
	"strings"

	"SIDM/internal/filename"
)

// Layouts de saída.
const (
	LayoutSplit   = "split"   // train/, valid/, test/ (padrão)
	LayoutFolders = "folders" // pastas montadas a partir dos metadados de cada imagem (só VOC)
)

// FolderLevel é um nível da árvore de pastas: de qual metadado sai o nome da
// pasta, e como cada valor é escrito (ex.: "rear" -> "Traseiro").
type FolderLevel struct {
	Field string `json:"field" bson:"field"`
	// Names: valor guardado no SIDM -> nome da pasta. Valor sem nome aqui
	// usa o padrão do campo (o próprio valor; na resolução, "01-640x480").
	Names map[string]string `json:"names,omitempty" bson:"names,omitempty"`
	// Unknown: pasta das imagens sem esse metadado (vazio = "Desconhecido").
	Unknown string `json:"unknown,omitempty" bson:"unknown,omitempty"`
}

// DefaultUnknown é a pasta das imagens sem o metadado do nível.
const DefaultUnknown = "Desconhecido"

// FolderFields são os metadados que podem virar nível de pasta. Os do
// veículo vêm do PRIMEIRO veículo da imagem. "split" é o grupo
// (train/valid/test) da divisão do perfil.
var FolderFields = []string{
	"vehicle_type", "plate_type", "plate_category", "vehicle_color",
	"resolution", "direction", "day_time", "weather", "capture_angle", "image_range",
	"equipment", "camera", "package", "split",
}

// codeTable: campos com valores fechados (os de filename.Codes).
var codeTable = map[string]string{
	"vehicle_type": "vehicle_type", "plate_type": "plate_type", "plate_category": "plate_category",
	"vehicle_color": "vehicle_color", "resolution": "resolution", "direction": "direction",
	"day_time": "day_time", "weather": "weather", "capture_angle": "capture_angle", "image_range": "image_range",
}

// Meta são os metadados de uma imagem que podem virar pasta (campo -> valor).
type Meta map[string]string

// FolderValues devolve os valores possíveis de um campo, com o nome padrão
// da pasta de cada um (para a tela montar a tabela de nomes). Campos abertos
// (equipamento, câmera, pacote) devolvem nil.
func FolderValues(field string) map[string]string {
	if field == "split" {
		return map[string]string{SplitTrain: SplitTrain, SplitValid: SplitValid, SplitTest: SplitTest}
	}
	table, ok := codeTable[field]
	if !ok {
		return nil
	}
	out := map[string]string{}
	for code, value := range filename.Codes()[table] {
		out[value] = defaultFolderName(field, code, value)
	}
	return out
}

// defaultFolderName: o próprio valor; na resolução, "<código>-<valor>"
// ("01-640x480"), que mantém as pastas em ordem.
func defaultFolderName(field, code, value string) string {
	if field == "resolution" {
		return code + "-" + value
	}
	return value
}

var resolutionCode = func() map[string]string {
	out := map[string]string{}
	for code, value := range filename.Codes()["resolution"] {
		out[value] = code
	}
	return out
}()

// Dir monta a pasta da imagem a partir dos níveis do perfil.
func (p *Profile) Dir(m Meta, split string) string {
	parts := make([]string, 0, len(p.Folders))
	for _, lvl := range p.Folders {
		value := m[lvl.Field]
		if lvl.Field == "split" {
			value = split
		}
		parts = append(parts, folderName(lvl, value))
	}
	return path.Join(parts...)
}

func folderName(lvl FolderLevel, value string) string {
	if value == "" {
		if lvl.Unknown != "" {
			return lvl.Unknown
		}
		return DefaultUnknown
	}
	if name, ok := lvl.Names[value]; ok && name != "" {
		return name
	}
	if lvl.Field == "resolution" {
		if code, ok := resolutionCode[value]; ok {
			return defaultFolderName(lvl.Field, code, value)
		}
	}
	return sanitizeFolder(value)
}

// sanitizeFolder: um valor livre (equipamento, câmera) vira nome de pasta
// válido no Windows e no zip.
func sanitizeFolder(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < 32 {
			return '_'
		}
		return r
	}, strings.TrimSpace(s))
	s = strings.TrimRight(s, ". ")
	if s == "" || s == "." || s == ".." {
		return "_"
	}
	return s
}

// validFolderName: nome escrito pelo usuário (Names, Unknown).
func validFolderName(s string) bool {
	return s != "" && sanitizeFolder(s) == s && len([]rune(s)) <= 64
}

func (p *Profile) normalizeFolders() {
	p.Layout = strings.ToLower(strings.TrimSpace(p.Layout))
	if p.Layout == "" {
		p.Layout = LayoutSplit
	}
	for i := range p.Folders {
		lvl := &p.Folders[i]
		lvl.Field = strings.TrimSpace(lvl.Field)
		lvl.Unknown = strings.TrimSpace(lvl.Unknown)
		if len(lvl.Names) > 0 {
			clean := make(map[string]string, len(lvl.Names))
			for value, name := range lvl.Names {
				if value, name = strings.TrimSpace(value), strings.TrimSpace(name); value != "" && name != "" {
					clean[value] = name
				}
			}
			lvl.Names = clean
		}
	}
	if p.Layout == LayoutSplit {
		p.Folders = nil
	}
}

// usesSplit: a divisão importa? (layout por divisão, ou um nível "split").
func (p *Profile) usesSplit() bool {
	if p.Layout != LayoutFolders {
		return true
	}
	for _, lvl := range p.Folders {
		if lvl.Field == "split" {
			return true
		}
	}
	return false
}

func (p *Profile) validateFolders(add func(string, ...any)) {
	switch p.Layout {
	case LayoutSplit, "":
		return
	case LayoutFolders:
	default:
		add("layout: use %q (train/valid/test) ou %q (pastas pelos metadados)", LayoutSplit, LayoutFolders)
		return
	}
	if p.Format != FormatVOC {
		add("layout: pastas pelos metadados só no formato %q (COCO e YOLO precisam de train/valid/test)", FormatVOC)
	}
	if len(p.Folders) == 0 {
		add("folders: escolha pelo menos um nível de pasta")
	}
	if len(p.Folders) > len(FolderFields) {
		add("folders: no máximo %d níveis", len(FolderFields))
	}
	seen := map[string]bool{}
	for i, lvl := range p.Folders {
		if !contains(FolderFields, lvl.Field) {
			add("folders[%d]: campo desconhecido %q (use: %s)", i, lvl.Field, strings.Join(FolderFields, ", "))
			continue
		}
		if seen[lvl.Field] {
			add("folders[%d]: %q repetido", i, lvl.Field)
		}
		seen[lvl.Field] = true
		if lvl.Unknown != "" && !validFolderName(lvl.Unknown) {
			add(`folders[%d].unknown: %q não serve de nome de pasta (sem / \ : * ? " < > |, até 64 caracteres)`, i, lvl.Unknown)
		}
		known := FolderValues(lvl.Field)
		values := make([]string, 0, len(lvl.Names))
		for value := range lvl.Names {
			values = append(values, value)
		}
		sort.Strings(values)
		for _, value := range values {
			if known != nil {
				if _, ok := known[value]; !ok {
					add("folders[%d].names: valor desconhecido %q para %s", i, value, lvl.Field)
				}
			}
			if !validFolderName(lvl.Names[value]) {
				add(`folders[%d].names[%s]: %q não serve de nome de pasta (sem / \ : * ? " < > |, até 64 caracteres)`, i, value, lvl.Names[value])
			}
		}
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
