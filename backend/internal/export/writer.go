package export

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"path"
	"sort"
	"strings"
	"time"

	"SIDM/internal/boxes"
)

// Item é uma imagem pronta para entrar no dataset: as caixas já com o nome
// final das classes, e o grupo (train/valid/test) já decidido.
type Item struct {
	SHA1     string
	FileName string // nome do arquivo no dataset (o guardado no SIDM; começa pelo sha1, então não repete)
	Width    int    // tamanho da imagem (as caixas são em pixels dela)
	Height   int
	Split    string
	Boxes    []boxes.Box
	Open     func() (io.ReadCloser, error) // o arquivo da imagem
}

// Result é o que foi escrito.
type Result struct {
	Images  map[string]int `json:"images"` // por grupo
	Boxes   map[string]int `json:"boxes"`  // por classe
	Missing []string       `json:"missing,omitempty"`
}

// Write escreve o dataset no zip. classes define os ids (a ordem importa).
// Uma imagem que não abre não derruba o export: vai para Missing, e o zip
// ganha um FALTANDO.txt.
func Write(zw *zip.Writer, p Profile, items []Item, classes []string, info any) (*Result, error) {
	ids := make(map[string]int, len(classes))
	for i, name := range classes {
		ids[name] = i
	}
	res := &Result{Images: map[string]int{}, Boxes: map[string]int{}}

	var w datasetWriter
	switch p.Format {
	case FormatCOCO:
		w = newCOCOWriter(zw, p.Split, classes)
	case FormatYOLO:
		w = &yoloWriter{zw: zw, split: p.Split, classes: classes}
	default:
		return nil, fmt.Errorf("formato desconhecido %q", p.Format)
	}

	for _, it := range items {
		ok, err := copyImage(zw, w.imagePath(it), it.Open)
		if err != nil {
			return res, err // erro no zip em si (conexão caiu): não adianta seguir
		}
		if !ok {
			res.Missing = append(res.Missing, it.FileName)
			continue
		}
		if err := w.add(it, ids); err != nil {
			return res, err
		}
		res.Images[it.Split]++
		for _, b := range it.Boxes {
			res.Boxes[b.Label]++
		}
	}
	if err := w.finish(); err != nil {
		return res, err
	}
	if len(res.Missing) > 0 {
		sort.Strings(res.Missing)
		if err := writeFile(zw, "FALTANDO.txt", []byte("Estas imagens não puderam ser incluídas (arquivo não encontrado):\r\n"+strings.Join(res.Missing, "\r\n")+"\r\n")); err != nil {
			return res, err
		}
	}
	data, _ := json.MarshalIndent(map[string]any{"info": info, "classes": classes, "result": res}, "", "  ")
	return res, writeFile(zw, "export_info.json", data)
}

type datasetWriter interface {
	imagePath(it Item) string
	add(it Item, ids map[string]int) error
	finish() error
}

// copyImage copia o arquivo para dentro do zip (sem compressão: JPEG já é
// comprimido). ok=false: o arquivo não abriu (fica de fora).
func copyImage(zw *zip.Writer, name string, open func() (io.ReadCloser, error)) (ok bool, err error) {
	f, err := open()
	if err != nil {
		return false, nil
	}
	defer f.Close()
	fw, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store, Modified: time.Now()})
	if err != nil {
		return false, err
	}
	_, err = io.Copy(fw, f)
	return err == nil, err
}

func writeFile(zw *zip.Writer, name string, data []byte) error {
	// Com data: sem ela, o zip mostra 1980.
	fw, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Now()})
	if err != nil {
		return err
	}
	_, err = fw.Write(data)
	return err
}

// rect devolve a caixa alinhada aos eixos, dentro da imagem. Uma caixa
// rotacionada (CVAT) vira o menor retângulo reto que a contém: COCO e YOLO
// (detecção) só têm caixa reta.
func rect(b boxes.Box, width, height int) (x, y, w, h float64) {
	x0, y0, x1, y1 := b.X, b.Y, b.X+b.W, b.Y+b.H
	if b.Rotation != 0 {
		cx, cy := b.X+b.W/2, b.Y+b.H/2
		rad := b.Rotation * math.Pi / 180
		cos, sin := math.Cos(rad), math.Sin(rad)
		x0, y0, x1, y1 = math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
		for _, c := range [][2]float64{{-b.W / 2, -b.H / 2}, {b.W / 2, -b.H / 2}, {b.W / 2, b.H / 2}, {-b.W / 2, b.H / 2}} {
			px := cx + c[0]*cos - c[1]*sin
			py := cy + c[0]*sin + c[1]*cos
			x0, y0, x1, y1 = math.Min(x0, px), math.Min(y0, py), math.Max(x1, px), math.Max(y1, py)
		}
	}
	if width > 0 && height > 0 {
		x0, y0 = math.Max(0, x0), math.Max(0, y0)
		x1, y1 = math.Min(float64(width), x1), math.Min(float64(height), y1)
	}
	return x0, y0, math.Max(0, x1-x0), math.Max(0, y1-y0)
}

// round2: duas casas (pixels); round6: seis (YOLO, normalizado).
func round2(v float64) float64 { return math.Round(v*100) / 100 }

// ---------- COCO (layout do Roboflow, o que o RF-DETR lê) ----------
//
//	train/_annotations.coco.json + imagens
//	valid/_annotations.coco.json + imagens
//	test/_annotations.coco.json  + imagens

type cocoWriter struct {
	zw         *zip.Writer
	splits     []string
	categories []map[string]any
	data       map[string]*cocoSplit
}

type cocoSplit struct {
	Images      []map[string]any `json:"images"`
	Annotations []map[string]any `json:"annotations"`
	nextAnn     int
}

func newCOCOWriter(zw *zip.Writer, s Split, classes []string) *cocoWriter {
	// As três pastas sempre existem (o layout do Roboflow, que o RF-DETR lê,
	// espera train, valid e test), mesmo que um grupo fique vazio.
	w := &cocoWriter{zw: zw, splits: []string{SplitTrain, SplitValid, SplitTest}, data: map[string]*cocoSplit{}}
	for i, name := range classes {
		w.categories = append(w.categories, map[string]any{"id": i + 1, "name": name, "supercategory": "none"})
	}
	for _, sp := range w.splits {
		w.data[sp] = &cocoSplit{Images: []map[string]any{}, Annotations: []map[string]any{}}
	}
	return w
}

func (w *cocoWriter) imagePath(it Item) string { return path.Join(it.Split, it.FileName) }

func (w *cocoWriter) add(it Item, ids map[string]int) error {
	d := w.data[it.Split]
	imageID := len(d.Images) + 1
	d.Images = append(d.Images, map[string]any{"id": imageID, "file_name": it.FileName, "width": it.Width, "height": it.Height})
	for _, b := range it.Boxes {
		x, y, bw, bh := rect(b, it.Width, it.Height)
		if bw <= 0 || bh <= 0 {
			continue
		}
		d.nextAnn++
		d.Annotations = append(d.Annotations, map[string]any{
			"id": d.nextAnn, "image_id": imageID, "category_id": ids[b.Label] + 1,
			"bbox": []float64{round2(x), round2(y), round2(bw), round2(bh)},
			"area": round2(bw * bh), "iscrowd": 0, "segmentation": []any{},
		})
	}
	return nil
}

func (w *cocoWriter) finish() error {
	for _, sp := range w.splits {
		d := w.data[sp]
		data, err := json.Marshal(map[string]any{
			"info":        map[string]any{"description": "SIDM", "version": "1.0"},
			"licenses":    []any{},
			"categories":  w.categories,
			"images":      d.Images,
			"annotations": d.Annotations,
		})
		if err != nil {
			return err
		}
		if err := writeFile(w.zw, path.Join(sp, "_annotations.coco.json"), data); err != nil {
			return err
		}
	}
	return nil
}

// ---------- YOLO (Ultralytics) ----------
//
//	data.yaml
//	images/{train,val,test}/<imagem>
//	labels/{train,val,test}/<imagem>.txt   "classe cx cy w h" normalizados (0..1)

type yoloWriter struct {
	zw      *zip.Writer
	split   Split
	classes []string
}

// O Ultralytics chama a validação de "val".
func yoloSplit(s string) string {
	if s == SplitValid {
		return "val"
	}
	return s
}

func (w *yoloWriter) imagePath(it Item) string {
	return path.Join("images", yoloSplit(it.Split), it.FileName)
}

func (w *yoloWriter) add(it Item, ids map[string]int) error {
	var sb strings.Builder
	if it.Width > 0 && it.Height > 0 {
		W, H := float64(it.Width), float64(it.Height)
		for _, b := range it.Boxes {
			x, y, bw, bh := rect(b, it.Width, it.Height)
			if bw <= 0 || bh <= 0 {
				continue
			}
			fmt.Fprintf(&sb, "%d %.6f %.6f %.6f %.6f\n", ids[b.Label], (x+bw/2)/W, (y+bh/2)/H, bw/W, bh/H)
		}
	}
	base := strings.TrimSuffix(it.FileName, path.Ext(it.FileName))
	return writeFile(w.zw, path.Join("labels", yoloSplit(it.Split), base+".txt"), []byte(sb.String()))
}

func (w *yoloWriter) finish() error {
	// Sem "path:" de propósito: com um path relativo, o Ultralytics procura a
	// partir da pasta de datasets DELE; sem path, a partir da pasta do
	// data.yaml (que é o que queremos: descompactou, treinou).
	var sb strings.Builder
	sb.WriteString("# Gerado pelo SIDM. Treino: yolo train data=data.yaml\n")
	for _, sp := range w.split.Splits() {
		fmt.Fprintf(&sb, "%s: images/%s\n", yoloSplit(sp), yoloSplit(sp))
		// As pastas existem mesmo que o grupo fique vazio (zip não guarda
		// pasta vazia sem uma entrada própria).
		for _, dir := range []string{"images/", "labels/"} {
			if _, err := w.zw.CreateHeader(&zip.FileHeader{Name: dir + yoloSplit(sp) + "/", Modified: time.Now()}); err != nil {
				return err
			}
		}
	}
	fmt.Fprintf(&sb, "nc: %d\nnames:\n", len(w.classes))
	for i, name := range w.classes {
		quoted, _ := json.Marshal(name) // string JSON é uma string YAML válida (aspas, acentos)
		fmt.Fprintf(&sb, "  %d: %s\n", i, quoted)
	}
	return writeFile(w.zw, "data.yaml", []byte(sb.String()))
}
