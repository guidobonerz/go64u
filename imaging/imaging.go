package imaging

import (
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"strconv"
	"time"

	"drazil.de/go64u/config"
	"drazil.de/go64u/util"
	"github.com/fogleman/gg"
	"github.com/mattn/go-sixel"
	"github.com/nfnt/resize"
)

const WIDTH = 384
const HEIGHT = 272
const SIZE = WIDTH * HEIGHT

type ImageFormat int

const (
	PNG ImageFormat = iota
	JPG
	SIXEL
)

var ScaleFactor = 100

func WriteImage(data []byte, scaleFactor int, format ImageFormat) bool {
	image := GetImageFromBytes(data, scaleFactor)
	millisStr := strconv.FormatInt(time.Now().UnixMilli(), 10)
	switch format {
	case PNG:
		{
			file, err := os.Create(fmt.Sprintf("%sultimate_screenshot_%s.png", config.GetConfig().ScreenshotFolder, millisStr))
			if err != nil {
				panic(err)
			}
			defer file.Close()

			png.Encode(file, image)
			fmt.Printf("Screenshot successfully written to %s%s%s\n", util.Green, config.GetConfig().ScreenshotFolder, util.Reset)
		}
	case JPG:
		{
			file, err := os.Create(fmt.Sprintf("%sultimate_screenshot_%s.jpg", config.GetConfig().ScreenshotFolder, millisStr))
			if err != nil {
				panic(err)
			}
			defer file.Close()
			options := &jpeg.Options{
				Quality: 90,
			}
			jpeg.Encode(file, image, options)
			fmt.Printf("Screenshot successfully written to %s%s%s\n", util.Green, config.GetConfig().ScreenshotFolder, util.Reset)
		}
	case SIXEL:
		{
			dc := gg.NewContextForImage(image)
			sixel.NewEncoder(os.Stdout).Encode(dc.Image())
		}
	}

	return true
}

func GetImageFromBytes(data []byte, scaleFactor int) image.Image {
	image := image.NewPaletted(image.Rect(0, 0, WIDTH, HEIGHT), util.GetPalette())
	pixelIndex := 0

	for _, b := range data {
		image.Pix[pixelIndex] = b & 0x0F
		pixelIndex++
		image.Pix[pixelIndex] = (b >> 4) & 0x0F
		pixelIndex++
		if pixelIndex >= SIZE {
			break
		}
	}

	if scaleFactor != 100 {
		scaledWidth := float32(WIDTH) / float32(100) * float32(scaleFactor)
		return resize.Resize(uint(scaledWidth), 0, image, resize.Bicubic)

	} else {
		return image
	}
}
