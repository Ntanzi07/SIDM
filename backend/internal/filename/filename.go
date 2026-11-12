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

func Parse(name string) (ImageMeta, error) {
	return ImageMeta{}, errors.New("not implemented")
}
