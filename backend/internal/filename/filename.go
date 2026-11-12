package filename

import (
	"errors"
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
// <sha1>_v2_<EQ>-<CAM>-<dir>-<res>-<angle>-<daytime>-<range>-<weather>_<cond.cond>_<YYYYMMDDTHHMMSSZ>_<placa-tipo-cat-veic-cor>.<placa-...>.<ext>
//
// TODO (seu): implementar.
// Dicas: filepath.Ext / strings.TrimSuffix, strings.Split por "_" (6 partes),
// depois cada parte com seu próprio Split. Layout de data: "20060102T150405Z".
func Parse(name string) (ImageMeta, error) {
	return ImageMeta{}, errors.New("not implemented")
}
