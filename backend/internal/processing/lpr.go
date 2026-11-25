package processing

import (
	"context"
	"math"

	"SIDM/internal/boxes"
	"SIDM/internal/lpr"
)

// LPRDetector é o que o adaptador usa do cliente do LPR (o *lpr.Client).
type LPRDetector interface {
	Ping(ctx context.Context) error
	Detect(ctx context.Context, img []byte) (*lpr.Objects, error)
}

// LPR é o modelo do detector de placas da Splice (o container lpr-class).
type LPR struct{ client LPRDetector }

// NewLPR liga o modelo ao cliente do servidor LPR.
func NewLPR(client LPRDetector) *LPR { return &LPR{client: client} }

// Os nomes das labels dos veículos, pelo tipo do detector.
var lprVehicleLabels = map[int]string{
	lpr.VehicleBus:        "bus",
	lpr.VehicleCar:        "car",
	lpr.VehicleMotorcycle: "motorcycle",
	lpr.VehicleTruck:      "truck",
}

// LPRLabels: as labels que o LPR gera (as da tarefa): os veículos, a placa
// e cada caractere (0-9, A-Z).
func LPRLabels() []string {
	out := []string{"bus", "car", "motorcycle", "truck", "plate"}
	for c := '0'; c <= '9'; c++ {
		out = append(out, string(c))
	}
	for c := 'A'; c <= 'Z'; c++ {
		out = append(out, string(c))
	}
	return out
}

func (m *LPR) Info() ModelInfo {
	return ModelInfo{
		Key:             "lpr",
		Label:           "LPR (placas)",
		Description:     "Detector de placas da Splice: acha veículos (ônibus, carro, moto, caminhão), placas e cada caractere da placa.",
		DefaultTask:     "pre-lpr",
		Labels:          LPRLabels(),
		DefaultMinScore: 0.5,
		ScoreHelp:       "De 0 a 1. Vale para os caracteres da placa: os de score menor ficam de fora (e fora do texto da placa).",
	}
}

func (m *LPR) Ping(ctx context.Context) error { return m.client.Ping(ctx) }

func (m *LPR) Detect(ctx context.Context, img []byte, opt Options) ([]boxes.Box, error) {
	o, err := m.client.Detect(ctx, img)
	if err != nil {
		return nil, err
	}
	return LPRBoxes(o, opt.MinScore), nil
}

// LPRBoxes converte o resultado do detector em caixas (em pixels):
//   - veículo: label pelo tipo (bus, car, motorcycle, truck);
//   - placa: label plate, com o atributo text (os caracteres dela, da
//     esquerda para a direita);
//   - caractere: label = o caractere, com o atributo score. Os de score
//     abaixo de minScore ficam de fora (e fora do text da placa).
func LPRBoxes(o *lpr.Objects, minScore float64) []boxes.Box {
	out := []boxes.Box{}
	for _, v := range o.Vehicle {
		label, ok := lprVehicleLabels[v.Type]
		if !ok {
			label = "vehicle"
		}
		out = append(out, lprBox(label, v.TopLeft, v.BottomRight))
	}
	texts := lpr.PlateTexts(o, minScore)
	for i, p := range o.Plate {
		b := lprBox("plate", p.TopLeft, p.BottomRight)
		if texts[i] != "" {
			b.Attributes = map[string]any{"text": texts[i]}
		}
		out = append(out, b)
	}
	for _, c := range o.Character {
		if c.Score < minScore || c.Char == "" {
			continue
		}
		b := lprBox(c.Char, c.TopLeft, c.BottomRight)
		b.Attributes = map[string]any{"score": math.Round(c.Score*1000) / 1000}
		out = append(out, b)
	}
	return out
}

func lprBox(label string, tl, br lpr.Point) boxes.Box {
	return boxes.Box{
		Label: label,
		X:     float64(tl.X),
		Y:     float64(tl.Y),
		W:     float64(max(br.X-tl.X, 0)),
		H:     float64(max(br.Y-tl.Y, 0)),
	}
}
