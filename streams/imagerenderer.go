package streams

import (
	"drazil.de/go64u/imaging"
)

type ImageRenderer struct {
	ScaleFactor int
	ImageFormat imaging.ImageFormat
	Quality     int
}
type SixelRenderer struct{}

func (d *ImageRenderer) Init() {
	d.ImageFormat = imaging.JPG
	d.Quality = 90

}

func (d *ImageRenderer) Render(data []byte) bool {
	return imaging.WriteImage(data, d.ScaleFactor, d.ImageFormat)
}

func (d *ImageRenderer) GetRunMode() RunMode {
	return OneShot
}
