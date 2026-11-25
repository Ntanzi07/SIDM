package processing

import (
	"context"
	"fmt"
	"math"

	"SIDM/internal/axle"
	"SIDM/internal/boxes"
)

// AxleDetector é o que o adaptador usa do cliente do contador de eixos (o
// *axle.Client).
type AxleDetector interface {
	Ping(ctx context.Context) error
	Detect(ctx context.Context, img []byte) (*axle.Result, error)
}

// Axle é o modelo do contador de eixos da Splice (o container
// axlecounter.software, um RF-DETR).
type Axle struct{ client AxleDetector }

// NewAxle liga o modelo ao cliente do servidor do contador de eixos.
func NewAxle(client AxleDetector) *Axle { return &Axle{client: client} }

// AxleLabels: as classes do servidor (class_names do config/server.json do
// axlecounter). Uma classe fora da lista chega como class_<id>.
func AxleLabels() []string {
	return []string{"axle", "class_a", "class_b", "class_c", "class_d", "class_e", "class_f", "class_g", "class_h"}
}

func (m *Axle) Info() ModelInfo {
	return ModelInfo{
		Key:             "axle",
		Label:           "Eixos",
		Description:     "Contador de eixos da Splice (RF-DETR): acha cada eixo do veículo (axle) e a classe do veículo (class_a a class_h).",
		DefaultTask:     "pre-eixos",
		Labels:          AxleLabels(),
		DefaultMinScore: 0,
		ScoreHelp:       "De 0 a 1. O servidor já tem o corte dele; aqui dá para ser mais exigente (as detecções de score menor ficam de fora).",
	}
}

func (m *Axle) Ping(ctx context.Context) error { return m.client.Ping(ctx) }

func (m *Axle) Detect(ctx context.Context, img []byte, opt Options) ([]boxes.Box, error) {
	r, err := m.client.Detect(ctx, img)
	if err != nil {
		return nil, err
	}
	return AxleBoxes(r, opt.MinScore), nil
}

// AxleBoxes converte as detecções em caixas (em pixels): a label é o nome da
// classe (class_<id> se o servidor não souber o nome), com o atributo score.
// As de score abaixo de minScore ficam de fora.
func AxleBoxes(r *axle.Result, minScore float64) []boxes.Box {
	out := []boxes.Box{}
	for _, d := range r.Detections {
		score := float64(d.Score)
		if score < minScore {
			continue
		}
		label := d.ClassName
		if label == "" || label == "unknown" {
			label = fmt.Sprintf("class_%d", d.ClassID)
		}
		out = append(out, boxes.Box{
			Label:      label,
			X:          float64(d.BboxX1),
			Y:          float64(d.BboxY1),
			W:          float64(max(d.BboxX2-d.BboxX1, 0)),
			H:          float64(max(d.BboxY2-d.BboxY1, 0)),
			Attributes: map[string]any{"score": math.Round(score*1000) / 1000},
		})
	}
	return out
}
