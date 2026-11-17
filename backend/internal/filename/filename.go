package filename

import (
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Vehicle struct {
	Plate         string
	PlateType     string
	PlateCategory string
	Type          string
	Color         string
}

type ImageMeta struct {
	SHA1         string
	Version      string
	Equipment    string
	Camera       string
	Direction    string
	Resolution   string
	CaptureAngle string
	DayTime      string
	ImageRange   string
	Weather      string
	Conditions   []string
	CapturedAt   time.Time
	Vehicles     []Vehicle
}

// Padrão v2:
// <sha1>_v2_<equipamento>_<camera>_<dir>-<res>-<angle>-<daytime>-<range>-<weather>_<cond.cond>_<YYYYMMDDTHHMMSSZ>_<placa-tipo-cat-veic-cor>.<placa-...>.<ext>
// Split por "_" deve dar exatamente 8 partes.
//
// Campo DESCONHECIDO = posição vazia (só o sha1 e a versão são obrigatórios).
// Nenhum valor real é vazio (nomes cadastrados, códigos e placas têm pelo
// menos um caractere), então o vazio nunca é ambíguo. Exemplos:
//   sem câmera:          ..._8003-MF4739__T-08-...
//   sem data:            ..._01__ABC1D23-M-8-C-WHT.jpg
//   sem placa:           ..._-M-8-C-WHT.jpg
//   sem veículo nenhum:  ..._20260903T112523Z_.jpg
//   sem condições:       ..._T-08-L-N-E-S__2026...
// Um veículo com tudo desconhecido é "----" (diferente de não ter veículo).

// CurrentVersion é a versão do padrão que o Format gera.
const CurrentVersion = currentVersion

const (
	currentVersion = "v2"
	dateLayout     = "20060102T150405Z" // o "Z" é literal: a data sempre está em UTC
	partsCount     = 8
)

var (
	allowedExts = map[string]bool{".jpg": true, ".jpeg": true, ".png": true}
	nameRe      = regexp.MustCompile(`^[A-Za-z0-9]+(-[A-Za-z0-9]+)*$`) // sem "-" no começo/fim nem "--"
	plateRe     = regexp.MustCompile(`^[A-Z0-9]{7}$`)                  // ABC1234 (BR) ou ABC1D23 (Mercosul)
)

// IsAllowedExt diz se a extensão (ex: ".jpg") é aceita. Case-insensitive.
func IsAllowedExt(ext string) bool {
	return allowedExts[strings.ToLower(ext)]
}

// Parse lê um nome de arquivo no padrão v2 e devolve os metadados.
func Parse(name string) (ImageMeta, error) {
	ext := strings.ToLower(filepath.Ext(name))
	if !allowedExts[ext] {
		return ImageMeta{}, fmt.Errorf("extensão não suportada: %q", ext)
	}
	base := strings.TrimSuffix(name, filepath.Ext(name))

	parts := strings.Split(base, "_")
	if len(parts) != partsCount {
		return ImageMeta{}, fmt.Errorf("esperava %d partes separadas por \"_\", veio %d", partsCount, len(parts))
	}

	var m ImageMeta
	var err error

	if m.SHA1, err = parseSHA1(parts[0]); err != nil {
		return ImageMeta{}, err
	}
	if parts[1] != currentVersion {
		return ImageMeta{}, fmt.Errorf("versão desconhecida: %q", parts[1])
	}
	m.Version = parts[1]

	if m.Equipment, err = optionalName(parts[2], maxEquipmentLen, "equipamento"); err != nil {
		return ImageMeta{}, err
	}
	if m.Camera, err = optionalName(parts[3], maxCameraLen, "câmera"); err != nil {
		return ImageMeta{}, err
	}
	if err = parseImageAttrs(parts[4], &m); err != nil {
		return ImageMeta{}, err
	}
	if m.Conditions, err = parseConditions(parts[5]); err != nil {
		return ImageMeta{}, err
	}
	if parts[6] != "" { // vazio = data desconhecida (CapturedAt fica zero)
		if m.CapturedAt, err = time.Parse(dateLayout, parts[6]); err != nil {
			return ImageMeta{}, fmt.Errorf("data inválida %q: %w", parts[6], err)
		}
	}
	if m.Vehicles, err = parseVehicles(parts[7]); err != nil {
		return ImageMeta{}, err
	}
	return m, nil
}

func parseSHA1(s string) (string, error) {
	if len(s) != 40 {
		return "", fmt.Errorf("sha1 deve ter 40 caracteres, tem %d", len(s))
	}
	if _, err := hex.DecodeString(s); err != nil {
		return "", fmt.Errorf("sha1 não é hexadecimal: %q", s)
	}
	return strings.ToLower(s), nil
}

// optionalName: vazio = desconhecido; senão, as regras de nome.
func optionalName(s string, max int, what string) (string, error) {
	if s == "" {
		return "", nil
	}
	return validateName(s, max, what)
}

func validateName(s string, max int, what string) (string, error) {
	if len(s) > max {
		return "", fmt.Errorf("%s com mais de %d caracteres: %q", what, max, s)
	}
	if !nameRe.MatchString(s) {
		return "", fmt.Errorf("%s com caracteres inválidos: %q", what, s)
	}
	return s, nil
}

// lookup traduz um código usando a tabela, com erro claro se não existir.
// Código vazio = desconhecido.
func lookup(table map[string]string, code, what string) (string, error) {
	if code == "" {
		return "", nil
	}
	v, ok := table[code]
	if !ok {
		return "", fmt.Errorf("código de %s desconhecido: %q", what, code)
	}
	return v, nil
}

func parseImageAttrs(s string, m *ImageMeta) error {
	f := strings.Split(s, "-")
	if len(f) != 6 {
		return fmt.Errorf("atributos da imagem: esperava 6 campos, veio %d (%q)", len(f), s)
	}
	var err error
	if m.Direction, err = lookup(directionCodes, f[0], "direção"); err != nil {
		return err
	}
	if m.Resolution, err = lookup(resolutionCodes, f[1], "resolução"); err != nil {
		return err
	}
	if m.CaptureAngle, err = lookup(captureAngleCodes, f[2], "ângulo"); err != nil {
		return err
	}
	if m.DayTime, err = lookup(dayTimeCodes, f[3], "período"); err != nil {
		return err
	}
	if m.ImageRange, err = lookup(imageRangeCodes, f[4], "alcance"); err != nil {
		return err
	}
	if m.Weather, err = lookup(weatherCodes, f[5], "clima"); err != nil {
		return err
	}
	return nil
}

func parseConditions(s string) ([]string, error) {
	if s == "" {
		return []string{}, nil // nenhuma condição informada
	}
	codes := strings.Split(s, ".")
	out := make([]string, 0, len(codes))
	for _, c := range codes {
		if c == "" {
			return nil, fmt.Errorf("condição vazia no meio da lista: %q", s)
		}
		v, err := lookup(conditionCodes, c, "condição")
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func parseVehicles(s string) ([]Vehicle, error) {
	if s == "" {
		return []Vehicle{}, nil // nenhum veículo
	}
	items := strings.Split(s, ".")
	out := make([]Vehicle, 0, len(items))
	for i, item := range items {
		f := strings.Split(item, "-")
		if len(f) != 5 {
			return nil, fmt.Errorf("veículo %d: esperava 5 campos (placa-tipo-categoria-veículo-cor), veio %d (%q)", i+1, len(f), item)
		}
		if f[0] != "" && !plateRe.MatchString(f[0]) { // vazio = placa desconhecida
			return nil, fmt.Errorf("veículo %d: placa inválida %q", i+1, f[0])
		}
		v := Vehicle{Plate: f[0]}
		var err error
		if v.PlateType, err = lookup(plateTypeCodes, f[1], "tipo de placa"); err != nil {
			return nil, err
		}
		if v.PlateCategory, err = lookup(plateCategoryCodes, f[2], "categoria de placa"); err != nil {
			return nil, err
		}
		if v.Type, err = lookup(vehicleTypeCodes, f[3], "tipo de veículo"); err != nil {
			return nil, err
		}
		if v.Color, err = lookup(colorCodes, f[4], "cor"); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}
