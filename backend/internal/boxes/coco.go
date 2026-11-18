// Package boxes lê as caixas (bounding boxes) anotadas fora do SIDM, no CVAT.
//
// O formato aceito é o COCO 1.0 que o CVAT exporta: um zip com
// annotations/instances_default.json (sem as imagens), ou o .json direto.
// Cada imagem do arquivo vira um Image, com as caixas em PIXELS da imagem
// original (x, y = canto superior esquerdo; w, h = largura e altura).
package boxes

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
)

// Box é uma caixa. Os atributos que o CVAT sempre manda (occluded, rotation)
// viram campos; os criados pela equipe (ex.: "caractere": "A") vão em
// Attributes, do jeito que vieram.
type Box struct {
	Label      string         `json:"label" bson:"label"`
	X          float64        `json:"x" bson:"x"`
	Y          float64        `json:"y" bson:"y"`
	W          float64        `json:"w" bson:"w"`
	H          float64        `json:"h" bson:"h"`
	Occluded   bool           `json:"occluded,omitempty" bson:"occluded,omitempty"`
	Rotation   float64        `json:"rotation,omitempty" bson:"rotation,omitempty"` // graus, sentido horário (CVAT)
	Attributes map[string]any `json:"attributes,omitempty" bson:"attributes,omitempty"`
}

// Image são as caixas de uma imagem do arquivo.
type Image struct {
	FileName string // como está no arquivo (pode ter pasta: "pasta/x.jpg")
	Width    int
	Height   int
	Boxes    []Box
}

// File é o arquivo lido.
type File struct {
	Images   []Image
	Boxes    int            // total de caixas
	Labels   map[string]int // caixas por label
	Warnings []string       // caixas ignoradas e por quê
}

// Limites de leitura: um JSON de anotação raramente passa de dezenas de MB.
const (
	MaxFileBytes = 256 << 20
	maxWarnings  = 50
)

// Read aceita o zip do CVAT ou o .json direto.
func Read(data []byte) (*File, error) {
	if bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		jsonData, err := jsonFromZip(data)
		if err != nil {
			return nil, err
		}
		data = jsonData
	}
	return ParseCOCO(bytes.NewReader(data))
}

// jsonFromZip acha o JSON das anotações dentro do zip do CVAT. Nada é
// extraído para o disco: o arquivo é lido em memória, com limite.
func jsonFromZip(data []byte) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("zip inválido: %w", err)
	}
	var candidates []*zip.File
	for _, f := range zr.File {
		if !f.FileInfo().IsDir() && strings.EqualFold(path.Ext(f.Name), ".json") {
			candidates = append(candidates, f)
		}
	}
	if len(candidates) == 0 {
		return nil, errors.New("o zip não tem nenhum .json: exporte a task do CVAT no formato COCO 1.0")
	}
	// O CVAT grava annotations/instances_default.json; se houver mais de um
	// (subsets), usa o de "instances" primeiro e avisa que só um foi lido.
	sort.SliceStable(candidates, func(i, j int) bool {
		return strings.Contains(candidates[i].Name, "instances") && !strings.Contains(candidates[j].Name, "instances")
	})
	if len(candidates) > 1 {
		names := make([]string, len(candidates))
		for i, f := range candidates {
			names[i] = f.Name
		}
		return nil, fmt.Errorf("o zip tem mais de um .json (%s): exporte a task sem subsets, ou mande só o .json", strings.Join(names, ", "))
	}
	f := candidates[0]
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("abrir %s no zip: %w", f.Name, err)
	}
	defer rc.Close()
	out, err := io.ReadAll(io.LimitReader(rc, MaxFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("ler %s no zip: %w", f.Name, err)
	}
	if len(out) > MaxFileBytes {
		return nil, fmt.Errorf("%s passa de %d MB", f.Name, MaxFileBytes>>20)
	}
	return out, nil
}

// Estruturas do COCO (só o que usamos).
type cocoFile struct {
	Images      []cocoImage      `json:"images"`
	Annotations []cocoAnnotation `json:"annotations"`
	Categories  []cocoCategory   `json:"categories"`
}

type cocoImage struct {
	ID       int64  `json:"id"`
	FileName string `json:"file_name"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
}

type cocoAnnotation struct {
	ID         int64          `json:"id"`
	ImageID    int64          `json:"image_id"`
	CategoryID int64          `json:"category_id"`
	BBox       []float64      `json:"bbox"`
	Attributes map[string]any `json:"attributes"`
}

type cocoCategory struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// ParseCOCO lê o JSON do COCO. Caixas com problema (label desconhecida,
// tamanho zero, fora da imagem) são ignoradas e vão para Warnings; um
// arquivo que nem é COCO é erro.
func ParseCOCO(r io.Reader) (*File, error) {
	var c cocoFile
	dec := json.NewDecoder(io.LimitReader(r, MaxFileBytes))
	dec.UseNumber()
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("JSON inválido: %w", err)
	}
	if c.Images == nil || c.Categories == nil {
		return nil, errors.New(`não parece COCO: faltam "images" ou "categories" (exporte a task do CVAT no formato COCO 1.0)`)
	}

	out := &File{Labels: map[string]int{}}
	warn := func(format string, args ...any) {
		if len(out.Warnings) < maxWarnings {
			out.Warnings = append(out.Warnings, fmt.Sprintf(format, args...))
		}
	}

	labels := make(map[int64]string, len(c.Categories))
	for _, cat := range c.Categories {
		labels[cat.ID] = strings.TrimSpace(cat.Name)
	}

	index := make(map[int64]int, len(c.Images))
	out.Images = make([]Image, 0, len(c.Images))
	for _, img := range c.Images {
		if img.FileName == "" {
			warn("imagem id %d sem file_name: ignorada", img.ID)
			continue
		}
		if _, dup := index[img.ID]; dup {
			warn("imagem id %d repetida: ignorada a segunda", img.ID)
			continue
		}
		index[img.ID] = len(out.Images)
		out.Images = append(out.Images, Image{FileName: img.FileName, Width: img.Width, Height: img.Height})
	}

	for _, a := range c.Annotations {
		i, ok := index[a.ImageID]
		if !ok {
			warn("caixa %d: imagem id %d não existe no arquivo", a.ID, a.ImageID)
			continue
		}
		img := &out.Images[i]
		label, ok := labels[a.CategoryID]
		if !ok || label == "" {
			warn("caixa %d (%s): categoria %d desconhecida", a.ID, img.FileName, a.CategoryID)
			continue
		}
		if len(a.BBox) != 4 {
			warn("caixa %d (%s): bbox deveria ter 4 números, tem %d", a.ID, img.FileName, len(a.BBox))
			continue
		}
		b := Box{Label: label, X: a.BBox[0], Y: a.BBox[1], W: a.BBox[2], H: a.BBox[3]}
		if b.W <= 0 || b.H <= 0 {
			warn("caixa %d (%s): largura/altura zero", a.ID, img.FileName)
			continue
		}
		b.Occluded, b.Rotation, b.Attributes = splitAttributes(a.Attributes)
		// Só avisa (a caixa fica): meio pixel de folga, porque o CVAT
		// arredonda as bordas; caixa rotacionada pode passar da borda.
		const slack = 0.5
		if img.Width > 0 && img.Height > 0 && b.Rotation == 0 &&
			(b.X < -slack || b.Y < -slack || b.X+b.W > float64(img.Width)+slack || b.Y+b.H > float64(img.Height)+slack) {
			warn("caixa %d (%s): passa da borda da imagem (%dx%d); mantida", a.ID, img.FileName, img.Width, img.Height)
		}
		img.Boxes = append(img.Boxes, b)
		out.Boxes++
		out.Labels[label]++
	}
	return out, nil
}

// splitAttributes separa occluded e rotation (sempre presentes no CVAT) dos
// atributos criados pela equipe.
func splitAttributes(attrs map[string]any) (occluded bool, rotation float64, rest map[string]any) {
	for k, v := range attrs {
		switch k {
		case "occluded":
			occluded, _ = v.(bool)
		case "rotation":
			if n, ok := v.(json.Number); ok {
				rotation, _ = n.Float64()
			}
		default:
			if rest == nil {
				rest = map[string]any{}
			}
			if n, ok := v.(json.Number); ok { // json.Number não vai bem para o BSON
				if f, err := n.Float64(); err == nil {
					v = f
				}
			}
			rest[k] = v
		}
	}
	return occluded, rotation, rest
}
