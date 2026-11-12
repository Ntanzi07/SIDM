package filename

import (
	"fmt"
	"strings"
)

// Tabelas invertidas ("front" -> "F"), geradas uma vez a partir das originais.
var (
	directionRev     = invert(directionCodes)
	resolutionRev    = invert(resolutionCodes)
	captureAngleRev  = invert(captureAngleCodes)
	dayTimeRev       = invert(dayTimeCodes)
	imageRangeRev    = invert(imageRangeCodes)
	weatherRev       = invert(weatherCodes)
	conditionRev     = invert(conditionCodes)
	plateTypeRev     = invert(plateTypeCodes)
	plateCategoryRev = invert(plateCategoryCodes)
	vehicleTypeRev   = invert(vehicleTypeCodes)
	colorRev         = invert(colorCodes)
)

func invert(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[v] = k
	}
	return out
}

// Format monta o nome do arquivo (SEM extensão) a partir dos metadados.
// É o caminho inverso do Parse: Parse(Format(m) + ".jpg") == m.
// Valida tudo antes, para nunca gerar um nome que o Parse recusaria.
func Format(m ImageMeta) (string, error) {
	sha1, err := parseSHA1(m.SHA1)
	if err != nil {
		return "", err
	}
	if _, err := validateName(m.Equipment, maxEquipmentLen, "equipamento"); err != nil {
		return "", err
	}
	if _, err := validateName(m.Camera, maxCameraLen, "câmera"); err != nil {
		return "", err
	}
	if len(m.Conditions) == 0 {
		return "", fmt.Errorf("imagem sem condições: use ao menos \"no_damage\"")
	}
	if len(m.Vehicles) == 0 {
		return "", fmt.Errorf("imagem sem veículos")
	}

	// Coletamos o primeiro erro e seguimos; no fim, se houve erro, retornamos ele.
	var firstErr error
	code := func(rev map[string]string, value, what string) string {
		c, ok := rev[value]
		if !ok && firstErr == nil {
			firstErr = fmt.Errorf("valor de %s desconhecido: %q", what, value)
		}
		return c
	}

	attrs := strings.Join([]string{
		code(directionRev, m.Direction, "direção"),
		code(resolutionRev, m.Resolution, "resolução"),
		code(captureAngleRev, m.CaptureAngle, "ângulo"),
		code(dayTimeRev, m.DayTime, "período"),
		code(imageRangeRev, m.ImageRange, "alcance"),
		code(weatherRev, m.Weather, "clima"),
	}, "-")

	conds := make([]string, 0, len(m.Conditions))
	for _, c := range m.Conditions {
		conds = append(conds, code(conditionRev, c, "condição"))
	}

	vehicles := make([]string, 0, len(m.Vehicles))
	for _, v := range m.Vehicles {
		if !plateRe.MatchString(v.Plate) && firstErr == nil {
			firstErr = fmt.Errorf("placa inválida: %q", v.Plate)
		}
		vehicles = append(vehicles, strings.Join([]string{
			v.Plate,
			code(plateTypeRev, v.PlateType, "tipo de placa"),
			code(plateCategoryRev, v.PlateCategory, "categoria de placa"),
			code(vehicleTypeRev, v.Type, "tipo de veículo"),
			code(colorRev, v.Color, "cor"),
		}, "-"))
	}

	if firstErr != nil {
		return "", firstErr
	}

	return strings.Join([]string{
		sha1,
		currentVersion,
		m.Equipment,
		m.Camera,
		attrs,
		strings.Join(conds, "."),
		m.CapturedAt.UTC().Format(dateLayout),
		strings.Join(vehicles, "."),
	}, "_"), nil
}
