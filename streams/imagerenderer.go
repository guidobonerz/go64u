package streams

import (
	"drazil.de/go64u/imaging"
)

type ImageRendererConfig struct {
	ScaleFactor int
	ImageFormat imaging.ImageFormat
	Quality     int
}

func (d *ImageRendererConfig) Run() {

}

func (d *ImageRendererConfig) Render(data []byte) bool {
	return imaging.WriteImage(data, d.ScaleFactor, d.ImageFormat)
}

func (d *ImageRendererConfig) GetRunMode() RunMode {
	return OneShot
}
