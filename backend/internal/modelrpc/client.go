// Package modelrpc é o cliente comum dos servidores dos modelos de IA da
// Splice (o LPR, o contador de eixos...): todos são o net/rpc padrão do Go
// (gob sobre TCP), atendem uma imagem por vez e recebem os bytes do arquivo.
// Cada modelo tem o seu pacote (lpr, axle) com os tipos da resposta e o
// método; este pacote só cuida da conexão.
//
// Biblioteca: sem banco e sem HTTP.
package modelrpc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/rpc"
	"sync"
	"time"
)

// ErrTimeout: o servidor não respondeu no prazo.
var ErrTimeout = errors.New("o servidor do modelo não respondeu no prazo")

// Client fala com o servidor de um modelo. Guarda uma conexão aberta e
// reconecta quando ela cai. Pode ser usado por várias goroutines (o
// servidor atende uma de cada vez).
type Client struct {
	Name    string        // nas mensagens de erro (ex.: "LPR")
	Addr    string        // host:porta (ex.: 127.0.0.1:54321)
	Timeout time.Duration // prazo de uma chamada; zero = 30 s

	mu   sync.Mutex
	conn *rpc.Client
}

// NewClient cria o cliente (não conecta: a conexão abre na primeira chamada).
func NewClient(name, addr string, timeout time.Duration) *Client {
	return &Client{Name: name, Addr: addr, Timeout: timeout}
}

// Ping confere se o servidor aceita conexão (só abre e fecha o TCP).
func (c *Client) Ping(ctx context.Context) error {
	d := net.Dialer{Timeout: time.Second}
	conn, err := d.DialContext(ctx, "tcp", c.Addr)
	if err != nil {
		return fmt.Errorf("servidor %s em %s não responde: %w", c.Name, c.Addr, err)
	}
	return conn.Close()
}

// Close fecha a conexão (a próxima chamada abre outra).
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

// Call faz a chamada com prazo. Erro de conexão (qualquer um que não seja
// erro do método no servidor) fecha a conexão e tenta mais uma vez numa
// nova: os métodos de detecção só leem, repetir não faz mal. newReply dá uma resposta
// nova a cada tentativa: a de uma chamada abandonada pode ainda estar sendo
// escrita pela conexão velha.
func (c *Client) Call(ctx context.Context, method string, args any, newReply func() any) error {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		var conn *rpc.Client
		if conn, err = c.client(ctx); err != nil {
			return err
		}
		call := conn.Go(method, args, newReply(), make(chan *rpc.Call, 1))
		timer := time.NewTimer(timeout)
		select {
		case <-call.Done:
			timer.Stop()
			err = call.Error
		case <-ctx.Done():
			timer.Stop()
			c.drop(conn)
			return ctx.Err()
		case <-timer.C:
			c.drop(conn)
			return ErrTimeout
		}
		if err == nil {
			return nil
		}
		var serverErr rpc.ServerError
		if errors.As(err, &serverErr) {
			return fmt.Errorf("servidor %s: %s", c.Name, string(serverErr))
		}
		c.drop(conn)
	}
	return fmt.Errorf("conexão com o servidor %s em %s: %w", c.Name, c.Addr, err)
}

// client devolve a conexão aberta, ou abre uma.
func (c *Client) client(ctx context.Context) (*rpc.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		return c.conn, nil
	}
	d := net.Dialer{Timeout: 3 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", c.Addr)
	if err != nil {
		return nil, fmt.Errorf("servidor %s em %s não responde: %w", c.Name, c.Addr, err)
	}
	c.conn = rpc.NewClient(conn)
	return c.conn, nil
}

// drop fecha a conexão, se ainda for a guardada.
func (c *Client) drop(conn *rpc.Client) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == conn {
		c.conn = nil
	}
	conn.Close()
}
