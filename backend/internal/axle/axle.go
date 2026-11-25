// Package axle é o cliente do contador de eixos da Splice (o container
// axlecounter.software, instalado pelo .deb axlecounter.software; um RF-DETR).
//
// O servidor é o net/rpc padrão do Go (gob sobre TCP, porta 54323) e atende
// uma imagem por vez. O método usado é AxleRPC.GetAxlesFull: recebe os bytes
// da imagem e devolve as detecções (classe, score e caixa) e quantos eixos
// achou.
//
// Os tipos abaixo são cópias dos do servidor (goserver/result): o gob casa
// os campos pelo NOME, então os nomes não podem mudar. O corte de score e os
// nomes das classes são do servidor (config/server.json e axle_runtime.json);
// o SIDM não chama o ConfigureAxle (vale para o servidor inteiro).
//
// Biblioteca: sem banco e sem HTTP. A conexão é a do modelrpc; quem converte
// o resultado em caixas é o módulo processing.
package axle

import (
	"context"
	"errors"
	"time"

	"SIDM/internal/modelrpc"
)

// Detection é uma caixa achada, em pixels da imagem (x1,y1 = canto de cima
// à esquerda; x2,y2 = canto de baixo à direita).
type Detection struct {
	ClassID   int
	ClassName string
	Score     float32
	BboxX1    int
	BboxY1    int
	BboxX2    int
	BboxY2    int
}

// Result é a resposta do GetAxlesFull. Count = detecções da classe "axle".
// Image só vem se o servidor estiver com return_image (o SIDM não usa).
type Result struct {
	Count      int
	Detections []Detection
	TotalTime  int64
	Image      []byte
}

// ErrInactive: o servidor está desligado (active=false no runtime config).
// Nesse caso ele devolve uma resposta vazia sem erro; sem esta conferência,
// toda imagem pareceria "sem nenhum eixo".
var ErrInactive = errors.New("o contador de eixos está desligado no servidor (active=false)")

// Client fala com o servidor do contador de eixos.
type Client struct{ rpc *modelrpc.Client }

// NewClient cria o cliente (não conecta: a conexão abre na primeira chamada).
func NewClient(addr string, timeout time.Duration) *Client {
	return &Client{rpc: modelrpc.NewClient("de eixos", addr, timeout)}
}

// Ping confere se o servidor aceita conexão.
func (c *Client) Ping(ctx context.Context) error { return c.rpc.Ping(ctx) }

// Close fecha a conexão.
func (c *Client) Close() error { return c.rpc.Close() }

// Detect manda os bytes da imagem e devolve as detecções.
func (c *Client) Detect(ctx context.Context, img []byte) (*Result, error) {
	var out *Result
	err := c.rpc.Call(ctx, "AxleRPC.GetAxlesFull", img, func() any { out = &Result{}; return out })
	if err != nil {
		return nil, err
	}
	// Desligado, o servidor nem roda o modelo: a resposta volta zerada (sem
	// detecção e sem tempo). Uma inferência de verdade sempre leva algum tempo.
	if len(out.Detections) == 0 && out.TotalTime == 0 {
		return nil, ErrInactive
	}
	out.Image = nil
	return out, nil
}
