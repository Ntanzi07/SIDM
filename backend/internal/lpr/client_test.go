package lpr

import (
	"context"
	"net"
	"net/rpc"
	"testing"
	"time"
)

// fakeLPR faz o papel do lpr-class: o mesmo nome (LprRPC) e a mesma
// assinatura do GetObjects. A conexão (prazo, reconectar) é testada no
// modelrpc; aqui, que os tipos e o método batem.
type fakeLPR struct{}

func (fakeLPR) GetObjects(data []byte, reply *Objects) error {
	*reply = Objects{
		Plate:   []Box{{TopLeft: Point{10, 20}, BottomRight: Point{60, 40}}},
		Vehicle: []Box{{TopLeft: Point{0, 0}, BottomRight: Point{100, 80}, Type: VehicleTruck}},
		Character: []CharacterBox{
			{TopLeft: Point{30, 22}, Char: "B", Score: 0.9, Type: len(data)},
			{TopLeft: Point{12, 22}, Char: "A", Score: 0.8},
			{TopLeft: Point{40, 22}, Char: "X", Score: 0.1},
		},
	}
	return nil
}

func TestDetect(t *testing.T) {
	srv := rpc.NewServer()
	if err := srv.RegisterName("LprRPC", fakeLPR{}); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go srv.Accept(ln)

	c := NewClient(ln.Addr().String(), time.Second)
	defer c.Close()
	if err := c.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	o, err := c.Detect(context.Background(), []byte{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(o.Plate) != 1 || len(o.Vehicle) != 1 || o.Vehicle[0].Type != VehicleTruck || len(o.Character) != 3 || o.Character[0].Type != 3 {
		t.Fatalf("resposta: %+v", o)
	}
	if got := PlateTexts(o, 0.5); len(got) != 1 || got[0] != "AB" {
		t.Errorf("texto da placa (ordem por X, sem o score baixo): %q", got)
	}
}
