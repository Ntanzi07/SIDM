// Package lpr é o cliente do detector de placas da Splice (o container
// lpr-class, da imagem visao_computacional, instalado pelo .deb splice-lpr).
//
// O servidor é o net/rpc padrão do Go (gob sobre TCP, porta 54321) e atende
// uma imagem por vez. O método usado é LprRPC.GetObjects: recebe os bytes da
// imagem e devolve os veículos, as placas e os caracteres achados.
//
// Os tipos abaixo são cópias dos do servidor (goserver/result): o gob casa
// os campos pelo NOME, então os nomes não podem mudar. O SIDM não chama o
// ConfigureLPR (ele escolhe os modelos e vale para o servidor inteiro).
//
// Biblioteca: sem banco e sem HTTP. A conexão é a do modelrpc; quem converte
// o resultado em caixas é o módulo processing.
package lpr

import (
	"context"
	"sort"
	"time"

	"SIDM/internal/modelrpc"
)

// Point é um ponto em pixels da imagem.
type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// Box é uma caixa achada (placa ou veículo).
type Box struct {
	TopLeft     Point `json:"top_left"`
	BottomRight Point `json:"bottom_right"`
	Type        int   `json:"type"`
}

// CharacterBox é um caractere de uma placa. PlateIndex é a posição da placa
// em Objects.Plate.
type CharacterBox struct {
	TopLeft     Point   `json:"top_left"`
	BottomRight Point   `json:"bottom_right"`
	Type        int     `json:"type"`
	Char        string  `json:"char"`
	Score       float64 `json:"score"`
	PlateIndex  int     `json:"plate_index"`
}

// Objects é a resposta do GetObjects.
type Objects struct {
	Plate     []Box          `json:"plate"`
	Vehicle   []Box          `json:"vehicle"`
	Character []CharacterBox `json:"character"`
}

// Os tipos de veículo do detector.
const (
	VehicleBus        = 0
	VehicleCar        = 1
	VehicleMotorcycle = 2
	VehicleTruck      = 3
)

// PlateTexts monta o texto de cada placa: os caracteres dela (PlateIndex)
// com score >= minScore, da esquerda para a direita. A posição no resultado
// é a da placa em o.Plate.
func PlateTexts(o *Objects, minScore float64) []string {
	byPlate := make([][]CharacterBox, len(o.Plate))
	for _, c := range o.Character {
		if c.PlateIndex >= 0 && c.PlateIndex < len(byPlate) && c.Score >= minScore {
			byPlate[c.PlateIndex] = append(byPlate[c.PlateIndex], c)
		}
	}
	out := make([]string, len(o.Plate))
	for i, chars := range byPlate {
		sort.SliceStable(chars, func(a, b int) bool { return chars[a].TopLeft.X < chars[b].TopLeft.X })
		text := ""
		for _, c := range chars {
			text += c.Char
		}
		out[i] = text
	}
	return out
}

// Client fala com o servidor LPR (o lpr-class).
type Client struct{ rpc *modelrpc.Client }

// NewClient cria o cliente (não conecta: a conexão abre na primeira chamada).
func NewClient(addr string, timeout time.Duration) *Client {
	return &Client{rpc: modelrpc.NewClient("LPR", addr, timeout)}
}

// Ping confere se o servidor aceita conexão.
func (c *Client) Ping(ctx context.Context) error { return c.rpc.Ping(ctx) }

// Close fecha a conexão.
func (c *Client) Close() error { return c.rpc.Close() }

// Detect manda os bytes da imagem e devolve o que o detector achou.
func (c *Client) Detect(ctx context.Context, img []byte) (*Objects, error) {
	var out *Objects
	err := c.rpc.Call(ctx, "LprRPC.GetObjects", img, func() any { out = &Objects{}; return out })
	if err != nil {
		return nil, err
	}
	return out, nil
}
