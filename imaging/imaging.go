package imaging

import (
	"fmt"
	"image"
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

var ScaleFactor = 100

func WriteImage(data []byte, scaleFactor int, showAsSixel bool) bool {
	img := image.NewPaletted(image.Rect(0, 0, WIDTH, HEIGHT), util.GetPalette())
	pixelIndex := 0

	for _, b := range data {
		img.Pix[pixelIndex] = b & 0x0F
		pixelIndex++
		img.Pix[pixelIndex] = (b >> 4) & 0x0F
		pixelIndex++
		if pixelIndex >= SIZE {
			break
		}
	}

	scaledWidth := float32(WIDTH) / float32(100) * float32(scaleFactor)

	scaledImage := resize.Resize(uint(scaledWidth), 0, img, resize.Bicubic)
	millisStr := strconv.FormatInt(time.Now().UnixMilli(), 10)
	if showAsSixel {
		dc := gg.NewContextForImage(scaledImage)
		sixel.NewEncoder(os.Stdout).Encode(dc.Image())
	} else {
		file, err := os.Create(fmt.Sprintf("%sultimate_screenshot_%s.png", config.GetConfig().ScreenshotFolder, millisStr))
		if err != nil {
			panic(err)
		}
		defer file.Close()

		png.Encode(file, scaledImage)
		fmt.Printf("Screenshot successfully written to %s%s%s\n", util.Green, config.GetConfig().ScreenshotFolder, util.Reset)
	}
	return true
}
