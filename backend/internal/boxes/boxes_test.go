package boxes

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// Mesmo formato do export COCO 1.0 do CVAT (conferido com um export real).
const cocoCVAT = `{
 "licenses": [{"name": "", "id": 0, "url": ""}],
 "info": {"contributor": "", "date_created": "", "description": "", "url": "", "version": "", "year": ""},
 "categories": [
  {"id": 1, "name": "carro", "supercategory": ""},
  {"id": 2, "name": "placa", "supercategory": ""},
  {"id": 3, "name": "caractere", "supercategory": ""}
 ],
 "images": [
  {"id": 1, "width": 1280, "height": 720, "file_name": "a.jpg", "license": 0, "flickr_url": "", "coco_url": "", "date_captured": 0},
  {"id": 2, "width": 640, "height": 480, "file_name": "pasta/b.jpg", "license": 0, "flickr_url": "", "coco_url": "", "date_captured": 0},
  {"id": 3, "width": 640, "height": 480, "file_name": "vazia.jpg", "license": 0, "flickr_url": "", "coco_url": "", "date_captured": 0}
 ],
 "annotations": [
  {"id": 1, "image_id": 1, "category_id": 1, "segmentation": [], "area": 1000, "bbox": [1.35, 0.0, 746.65, 480.46], "iscrowd": 0, "attributes": {"occluded": false, "rotation": 0.0}},
  {"id": 2, "image_id": 1, "category_id": 2, "segmentation": [], "area": 10, "bbox": [500, 400, 120, 40], "iscrowd": 0, "attributes": {"occluded": true, "rotation": 12.5}},
  {"id": 3, "image_id": 2, "category_id": 3, "segmentation": [], "area": 10, "bbox": [10, 10, 8, 16], "iscrowd": 0, "attributes": {"occluded": false, "rotation": 0.0, "valor": "A", "confianca": 3}},
  {"id": 4, "image_id": 2, "category_id": 9, "segmentation": [], "area": 10, "bbox": [1, 1, 1, 1], "iscrowd": 0, "attributes": {}},
  {"id": 5, "image_id": 2, "category_id": 2, "segmentation": [], "area": 0, "bbox": [1, 1, 0, 5], "iscrowd": 0, "attributes": {}},
  {"id": 6, "image_id": 7, "category_id": 2, "segmentation": [], "area": 1, "bbox": [1, 1, 1, 1], "iscrowd": 0, "attributes": {}},
  {"id": 7, "image_id": 2, "category_id": 2, "segmentation": [], "area": 1, "bbox": [600, 1, 100, 10], "iscrowd": 0, "attributes": {}}
 ]
}`

func zipWith(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(content))
	}
	zw.Close()
	return buf.Bytes()
}

func TestReadCOCO(t *testing.T) {
	for name, data := range map[string][]byte{
		"json direto": []byte(cocoCVAT),
		"zip do CVAT": zipWith(t, map[string]string{"annotations/instances_default.json": cocoCVAT}),
	} {
		t.Run(name, func(t *testing.T) {
			f, err := Read(data)
			if err != nil {
				t.Fatal(err)
			}
			if len(f.Images) != 3 {
				t.Fatalf("imagens = %d, want 3 (inclusive a sem caixa)", len(f.Images))
			}
			if f.Boxes != 4 || f.Labels["placa"] != 2 || f.Labels["carro"] != 1 || f.Labels["caractere"] != 1 {
				t.Errorf("boxes=%d labels=%v", f.Boxes, f.Labels)
			}

			a := f.Images[0]
			if a.FileName != "a.jpg" || a.Width != 1280 || a.Height != 720 || len(a.Boxes) != 2 {
				t.Fatalf("a = %+v", a)
			}
			if b := a.Boxes[0]; b.Label != "carro" || b.X != 1.35 || b.W != 746.65 || b.Occluded || b.Rotation != 0 || b.Attributes != nil {
				t.Errorf("caixa 1 = %+v", b)
			}
			if b := a.Boxes[1]; !b.Occluded || b.Rotation != 12.5 {
				t.Errorf("caixa 2 (occluded/rotation) = %+v", b)
			}
			if b := f.Images[1].Boxes[0]; b.Attributes["valor"] != "A" || b.Attributes["confianca"] != 3.0 {
				t.Errorf("atributos da equipe = %#v", b.Attributes)
			}
			if len(f.Images[2].Boxes) != 0 {
				t.Error("a imagem sem anotação vem sem caixas")
			}

			all := strings.Join(f.Warnings, "\n")
			for _, want := range []string{"categoria 9 desconhecida", "largura/altura zero", "imagem id 7 não existe", "passa da borda"} {
				if !strings.Contains(all, want) {
					t.Errorf("faltou o aviso %q em:\n%s", want, all)
				}
			}
		})
	}
}

func TestReadErros(t *testing.T) {
	cases := map[string][]byte{
		"não é json":        []byte("isso não é json"),
		"json sem coco":     []byte(`{"foo": 1}`),
		"zip sem json":      zipWith(t, map[string]string{"images/a.jpg": "x"}),
		"zip com dois json": zipWith(t, map[string]string{"annotations/instances_Train.json": cocoCVAT, "annotations/instances_Validation.json": cocoCVAT}),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Read(data); err == nil {
				t.Error("deveria dar erro")
			}
		})
	}
}
