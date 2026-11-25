package lpr

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"testing"
	"time"
)

// Teste de INTEGRAÇÃO com o lpr-class de verdade: só roda com
// LPR_TEST_ADDR=127.0.0.1:54321. A imagem é sintética (sem placa): o que se
// confere é que o protocolo (gob, nomes dos campos) bate com o servidor.
func TestServidorDeVerdade(t *testing.T) {
	addr := os.Getenv("LPR_TEST_ADDR")
	if addr == "" {
		t.Skip("LPR_TEST_ADDR não definido")
	}
	img := image.NewRGBA(image.Rect(0, 0, 640, 480))
	for y := 0; y < 480; y++ {
		for x := 0; x < 640; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 90, 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	c := NewClient(addr, 30*time.Second)
	defer c.Close()
	o, err := c.Detect(context.Background(), buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("placas %d, veículos %d, caracteres %d", len(o.Plate), len(o.Vehicle), len(o.Character))
}
