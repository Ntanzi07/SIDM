package modelrpc

import (
	"context"
	"errors"
	"net"
	"net/rpc"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeServer faz o papel do servidor de um modelo (registrado como LprRPC,
// com a assinatura do GetObjects do lpr-class).
type fakeServer struct {
	delay time.Duration
	fail  string
	calls int
	mu    sync.Mutex
}

// Reply é o formato de resposta do servidor falso.
type Reply struct {
	N     int
	Bytes int
}

func (f *fakeServer) GetObjects(data []byte, out *Reply) error {
	f.mu.Lock()
	f.calls++
	delay, fail := f.delay, f.fail
	f.mu.Unlock()
	time.Sleep(delay)
	if fail != "" {
		return errors.New(fail)
	}
	*out = Reply{N: 3, Bytes: len(data)} // Bytes prova que a imagem chegou
	return nil
}

// startFake sobe o servidor falso numa porta livre. close derruba as
// conexões abertas (como um restart do container).
func startFake(t *testing.T, f *fakeServer) (addr string, dropConns func()) {
	t.Helper()
	srv := rpc.NewServer()
	if err := srv.RegisterName("LprRPC", f); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var conns []net.Conn
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, conn)
			mu.Unlock()
			go srv.ServeConn(conn)
		}
	}()
	t.Cleanup(func() { ln.Close() })
	return ln.Addr().String(), func() {
		mu.Lock()
		defer mu.Unlock()
		for _, c := range conns {
			c.Close()
		}
		conns = nil
	}
}

func detect(ctx context.Context, c *Client, img []byte) (*Reply, error) {
	var out *Reply
	err := c.Call(ctx, "LprRPC.GetObjects", img, func() any { out = &Reply{}; return out })
	return out, err
}

func TestCall(t *testing.T) {
	f := &fakeServer{}
	addr, _ := startFake(t, f)
	c := NewClient("Teste", addr, time.Second)
	defer c.Close()

	o, err := detect(context.Background(), c, []byte{7, 1, 2})
	if err != nil {
		t.Fatal(err)
	}
	if o.N != 3 || o.Bytes != 3 {
		t.Fatalf("resposta: %+v", o)
	}
	if err := c.Ping(context.Background()); err != nil {
		t.Errorf("ping: %v", err)
	}
}

// O servidor reiniciou (as conexões caíram): a próxima chamada reconecta
// sozinha.
func TestCallReconecta(t *testing.T) {
	f := &fakeServer{}
	addr, dropConns := startFake(t, f)
	c := NewClient("Teste", addr, time.Second)
	defer c.Close()
	if _, err := detect(context.Background(), c, nil); err != nil {
		t.Fatal(err)
	}
	dropConns()
	time.Sleep(20 * time.Millisecond)
	if _, err := detect(context.Background(), c, nil); err != nil {
		t.Fatalf("depois de cair a conexão: %v", err)
	}
}

func TestCallErros(t *testing.T) {
	f := &fakeServer{fail: "imagem inválida"}
	addr, _ := startFake(t, f)
	c := NewClient("Teste", addr, 200*time.Millisecond)
	defer c.Close()

	// erro do método: volta a mensagem, sem tentar de novo
	_, err := detect(context.Background(), c, nil)
	if err == nil || !strings.Contains(err.Error(), "imagem inválida") {
		t.Errorf("erro do servidor: %v", err)
	}
	if f.calls != 1 {
		t.Errorf("erro do método não repete: %d chamadas", f.calls)
	}

	// prazo
	f.mu.Lock()
	f.fail, f.delay = "", time.Second
	f.mu.Unlock()
	if _, err := detect(context.Background(), c, nil); !errors.Is(err, ErrTimeout) {
		t.Errorf("prazo: %v", err)
	}

	// cancelado
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := detect(ctx, c, nil); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelado: %v", err)
	}

	// sem servidor
	off := NewClient("Teste", "127.0.0.1:1", time.Second)
	if err := off.Ping(context.Background()); err == nil {
		t.Error("ping sem servidor deveria falhar")
	}
	if _, err := detect(context.Background(), off, nil); err == nil {
		t.Error("detect sem servidor deveria falhar")
	}
}
