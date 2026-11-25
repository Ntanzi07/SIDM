package axle

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"net"
	"net/rpc"
	"os"
	"testing"
	"time"
)

// fakeAxle faz o papel do axlecounter: o mesmo nome (AxleRPC) e a mesma
// assinatura do GetAxlesFull. inactive imita o active=false (resposta zerada).
type fakeAxle struct{ inactive bool }

func (f *fakeAxle) GetAxlesFull(data []byte, reply *Result) error {
	*reply = Result{}
	if f.inactive {
		return nil
	}
	reply.Detections = []Detection{
		{ClassID: 0, ClassName: "axle", Score: 0.9, BboxX1: 10, BboxY1: 20, BboxX2: 40, BboxY2: 60},
		{ClassID: 3, ClassName: "class_c", Score: 0.5, BboxX1: 0, BboxY1: 0, BboxX2: len(data), BboxY2: 10},
	}
	reply.Count, reply.TotalTime, reply.Image = 1, 12, []byte{1, 2}
	return nil
}

func start(t *testing.T, f *fakeAxle) string {
	t.Helper()
	srv := rpc.NewServer()
	if err := srv.RegisterName("AxleRPC", f); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go srv.Accept(ln)
	return ln.Addr().String()
}

func TestDetect(t *testing.T) {
	f := &fakeAxle{}
	c := NewClient(start(t, f), time.Second)
	defer c.Close()
	r, err := c.Detect(context.Background(), []byte{1, 2, 3, 4})
	if err != nil {
		t.Fatal(err)
	}
	if r.Count != 1 || len(r.Detections) != 2 || r.Detections[1].BboxX2 != 4 || r.Detections[0].ClassName != "axle" {
		t.Fatalf("resposta: %+v", r)
	}
	if r.Image != nil {
		t.Error("a imagem de volta não é guardada")
	}

	f.inactive = true
	if _, err := c.Detect(context.Background(), []byte{1}); !errors.Is(err, ErrInactive) {
		t.Errorf("desligado: %v", err)
	}
}

// Teste de INTEGRAÇÃO com o axlecounter de verdade: só roda com
// AXLE_TEST_ADDR=127.0.0.1:54323. A imagem é sintética: o que se confere é
// que o protocolo (gob, nomes dos campos) bate com o servidor.
func TestServidorDeVerdade(t *testing.T) {
	addr := os.Getenv("AXLE_TEST_ADDR")
	if addr == "" {
		t.Skip("AXLE_TEST_ADDR não definido")
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
	r, err := c.Detect(context.Background(), buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("eixos %d, detecções %d, %d ms", r.Count, len(r.Detections), r.TotalTime)
}
