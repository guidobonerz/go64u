package streams

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"drazil.de/go64u/config"
	"drazil.de/go64u/imaging"
)

const outputW = 1920
const outputH = 1080

type StreamRenderer struct {
	Fps            int
	ScaleFactor    int
	Url            string
	LogLevel       string
	RecordPath     string // if set, also record to this file
	RecordMode     string // "audio", "video", or "both"
	NoOverlay      string // disable overlay for: "stream", "record", or "both"
	cancel         context.CancelFunc
	pipeline       *OutputPipeline
	recordPipeline *OutputPipeline
	ctx            context.Context
	config         StreamConfig
	frameCount     int
	overlayImg     image.Image // loaded once in Init, nil if no overlay
	bgraBuf        []byte      // reused BGRA output buffer
	recordBuf      []byte      // reused BGRA buffer for clean recording frames (nil when not needed)
	nativeBuf      bool        // true when bgraBuf is at native resolution (no overlay)
	sigChan            chan os.Signal
	dualPipeline       bool // true when stream and record have different overlay settings
	streamWithOverlay  bool // whether the stream output gets the overlay
	recordWithOverlay  bool // whether the record output gets the overlay
	overlayEnabled     atomic.Bool // live toggle from GUI, read per-frame
	forceKeyframe      atomic.Bool // request IDR on next frame after overlay toggle
	zeroBuf            []byte      // pre-zeroed buffer for fast clear via copy()
	paletteLUT         [16]bgraColor // pre-built BGRA lookup table for C64 palette
	paletteLUTReady    bool
	reusableImg        *imaging.ReusablePalettedImage // zero-alloc frame decoder
}

type bgraColor struct{ b, g, r, a byte }

type StreamConfig struct {
	Width     int
	Height    int
	FPS       int
	Bitrate   string
	StreamKey string
}

func (d *StreamRenderer) Init() error {
	d.config = StreamConfig{
		Width:   outputW,
		Height:  outputH,
		FPS:     50,
		Bitrate: "3000k",
	}

	ctx, cancel := context.WithCancel(context.Background())
	d.ctx = ctx
	d.cancel = cancel

	d.sigChan = make(chan os.Signal, 1)
	signal.Notify(d.sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-d.sigChan
		fmt.Println("\nReceived interrupt, stopping stream gracefully...")
		d.Shutdown()
	}()

	if d.Url == "" && d.RecordPath == "" {
		return fmt.Errorf("no streaming target or recording path specified")
	}

	// Load overlay image if configured
	overlay := config.GetConfig().Overlay
	if overlay.ImagePath != "" {
		f, err := os.Open(overlay.ImagePath)
		if err != nil {
			return fmt.Errorf("failed to open overlay image: %w", err)
		}
		defer f.Close()
		img, err := png.Decode(f)
		if err != nil {
			return fmt.Errorf("failed to decode overlay PNG: %w", err)
		}
		d.overlayImg = img
		fmt.Printf("Overlay loaded: %s (%dx%d)\n", overlay.ImagePath,
			img.Bounds().Dx(), img.Bounds().Dy())
	}

	d.reusableImg = imaging.NewReusablePalettedImage()

	hasStream := d.Url != ""
	hasRecord := d.RecordPath != ""

	noOverlayStream := d.NoOverlay == "stream" || d.NoOverlay == "both"
	noOverlayRecord := d.NoOverlay == "record" || d.NoOverlay == "both"

	// Strip overlay image if disabled for all outputs
	if noOverlayStream && noOverlayRecord {
		d.overlayImg = nil
	}

	d.streamWithOverlay = d.overlayImg != nil && !noOverlayStream && hasStream
	d.recordWithOverlay = d.overlayImg != nil && !noOverlayRecord && hasRecord
	d.overlayEnabled.Store(d.streamWithOverlay || d.recordWithOverlay)

	// Need dual pipeline when streaming + recording have different overlay settings
	d.dualPipeline = hasStream && hasRecord && d.streamWithOverlay != d.recordWithOverlay

	// Allocate BGRA buffers: native resolution when no overlay, full 1920x1080 when overlay.
	// Native resolution is ~20x smaller (384x272 = 417KB vs 1920x1080 = 8MB).
	anyOverlay := d.streamWithOverlay || d.recordWithOverlay
	if anyOverlay {
		bufSize := outputW * outputH * 4
		d.bgraBuf = make([]byte, bufSize)
		d.zeroBuf = make([]byte, bufSize)
		d.nativeBuf = false
	} else {
		// Will be resized on first frame when we know the native dimensions
		d.nativeBuf = true
	}

	if d.dualPipeline {
		d.recordBuf = make([]byte, outputW*outputH*4)
		if err := d.initDualPipeline(); err != nil {
			return err
		}
	} else {
		if err := d.initSinglePipeline(hasStream, hasRecord); err != nil {
			return err
		}
	}

	fmt.Println("Stream initialization complete")
	fmt.Println("Ready to receive frames...")
	return nil
}

func (d *StreamRenderer) srcDims(withOverlay bool) (int, int) {
	if withOverlay {
		return outputW, outputH // overlay needs full resolution
	}
	// 16:9 buffer at native height (272) to preserve aspect ratio.
	// 272 * 16/9 = 483.6 → 484 (must be even for YUV420P).
	// The 384px game image is centered with black bars on each side.
	return 484, 272
}

func (d *StreamRenderer) initSinglePipeline(hasStream, hasRecord bool) error {
	var err error
	anyOverlay := d.streamWithOverlay || d.recordWithOverlay
	srcW, srcH := d.srcDims(anyOverlay)

	if hasStream && hasRecord {
		fmt.Printf("Streaming to: %s\n", d.Url)
		fmt.Printf("Recording to: %s (mode: %s)\n", d.RecordPath, d.RecordMode)

		d.pipeline, err = NewOutputPipeline(d.Url, "flv", d.config, "", srcW, srcH)
		if err != nil {
			return fmt.Errorf("failed to create stream pipeline: %w", err)
		}
		fmt.Println("Stream pipeline initialized")

		d.recordPipeline, err = NewOutputPipeline(d.RecordPath, "mp4", d.config, d.RecordMode, srcW, srcH)
		if err != nil {
			d.pipeline.Close()
			return fmt.Errorf("failed to create record pipeline: %w", err)
		}
		fmt.Println("Record pipeline initialized")
	} else if hasRecord {
		fmt.Printf("Recording to: %s (mode: %s)\n", d.RecordPath, d.RecordMode)

		d.pipeline, err = NewOutputPipeline(d.RecordPath, "mp4", d.config, d.RecordMode, srcW, srcH)
		if err != nil {
			return fmt.Errorf("failed to create record pipeline: %w", err)
		}
		fmt.Println("Record pipeline initialized")
	} else {
		fmt.Printf("Streaming to: %s\n", d.Url)

		d.pipeline, err = NewOutputPipeline(d.Url, "flv", d.config, "", srcW, srcH)
		if err != nil {
			return fmt.Errorf("failed to create stream pipeline: %w", err)
		}
		fmt.Println("Stream pipeline initialized")
	}

	return nil
}

func (d *StreamRenderer) initDualPipeline() error {
	overlayLabel := func(with bool) string {
		if with {
			return "with overlay"
		}
		return "without overlay"
	}
	fmt.Printf("Streaming to: %s (%s)\n", d.Url, overlayLabel(d.streamWithOverlay))
	fmt.Printf("Recording to: %s (mode: %s, %s)\n", d.RecordPath, d.RecordMode, overlayLabel(d.recordWithOverlay))

	var err error

	// Dual pipeline: both get full resolution since the overlay pipeline forces 1920x1080 buffers
	d.pipeline, err = NewOutputPipeline(d.Url, "flv", d.config, "", outputW, outputH)
	if err != nil {
		return fmt.Errorf("failed to create stream pipeline: %w", err)
	}
	fmt.Println("Stream pipeline initialized")

	d.recordPipeline, err = NewOutputPipeline(d.RecordPath, "mp4", d.config, d.RecordMode, outputW, outputH)
	if err != nil {
		d.pipeline.Close()
		return fmt.Errorf("failed to create record pipeline: %w", err)
	}
	fmt.Println("Record pipeline initialized")

	return nil
}

func (d *StreamRenderer) Render(data []byte) bool {
	select {
	case <-d.ctx.Done():
		fmt.Println("Context cancelled, stopping render")
		return false
	default:
		d.frameCount++

		if d.frameCount%90 == 1 {
			fmt.Printf("[Frame %d] Received %d bytes of data\n", d.frameCount, len(data))
		}

		img := d.reusableImg.Decode(data)

		if d.frameCount == 1 {
			bounds := img.Bounds()
			fmt.Printf("[Frame %d] Image size: %dx%d\n", d.frameCount, bounds.Dx(), bounds.Dy())
			// Lazy-alloc native buffer on first frame
			if d.nativeBuf && d.bgraBuf == nil {
				srcW, srcH := d.srcDims(false)
				bufSize := srcW * srcH * 4
				d.bgraBuf = make([]byte, bufSize)
				d.zeroBuf = make([]byte, bufSize)
				fmt.Printf("[Native mode] Buffer: %dx%d (%d bytes) -- swscale handles upscaling\n",
					srcW, srcH, bufSize)
			}
		}

		// Read overlay state once per frame (atomic, set by GUI thread)
		useOverlay := d.overlayEnabled.Load()

		// Force keyframe on overlay state change for clean scene transition
		if d.forceKeyframe.CompareAndSwap(true, false) {
			if d.pipeline != nil {
				d.pipeline.RequestKeyframe()
			}
			if d.recordPipeline != nil {
				d.recordPipeline.RequestKeyframe()
			}
		}

		if d.dualPipeline {
			d.composeFrameInto(d.recordBuf, img, useOverlay && d.recordWithOverlay)
			if err := d.recordPipeline.EncodeVideoFrame(d.recordBuf); err != nil {
				fmt.Printf("Record encode error: %v\n", err)
			}

			d.composeFrameInto(d.bgraBuf, img, useOverlay && d.streamWithOverlay)
			if err := d.pipeline.EncodeVideoFrame(d.bgraBuf); err != nil {
				fmt.Printf("Stream encode error: %v\n", err)
			}
		} else {
			d.composeFrameInto(d.bgraBuf, img, useOverlay && (d.streamWithOverlay || d.recordWithOverlay))
			if err := d.pipeline.EncodeVideoFrame(d.bgraBuf); err != nil {
				fmt.Printf("Encode error: %v\n", err)
			}
			if d.recordPipeline != nil {
				if err := d.recordPipeline.EncodeVideoFrame(d.bgraBuf); err != nil {
					fmt.Printf("Record encode error: %v\n", err)
				}
			}
		}

		if d.frameCount%d.config.FPS == 0 {
			fmt.Printf("=== Streamed %d seconds (%d frames) ===\n",
				d.frameCount/d.config.FPS, d.frameCount)
		}

		return true
	}
}

func (d *StreamRenderer) composeFrameInto(buf []byte, gameImg image.Image, withOverlay bool) {
	bounds := gameImg.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	// Build palette LUT once (palette never changes across frames)
	if pal, ok := gameImg.(*image.Paletted); ok && !d.paletteLUTReady {
		for i, c := range pal.Palette {
			if i >= 16 {
				break
			}
			r, g, b, a := c.RGBA()
			d.paletteLUT[i] = bgraColor{byte(b >> 8), byte(g >> 8), byte(r >> 8), byte(a >> 8)}
		}
		d.paletteLUTReady = true
	}

	// Native mode: no scaling, just palette->BGRA at native resolution.
	// swscale handles upscaling to 1920x1080 in the encoder pipeline.
	if d.nativeBuf && !withOverlay {
		d.composeNative(buf, gameImg)
		return
	}

	// Overlay/scaled mode: compose at 1920x1080
	copy(buf, d.zeroBuf)

	var dstX, dstY, dstW, dstH int

	if withOverlay && d.overlayImg != nil {
		overlay := config.GetConfig().Overlay
		dstX = overlay.X
		dstY = overlay.Y
		dstW = overlay.WITH
		dstH = overlay.HEIGHT
	} else {
		// No overlay: scale to max height, center horizontally
		dstH = outputH
		dstW = srcW * outputH / srcH
		dstX = (outputW - dstW) / 2
		dstY = 0
	}

	if dstX+dstW > outputW {
		dstW = outputW - dstX
	}
	if dstY+dstH > outputH {
		dstH = outputH - dstY
	}

	if pal, ok := gameImg.(*image.Paletted); ok {
		lut := &d.paletteLUT
		stride := pal.Stride
		pix := pal.Pix
		minX := bounds.Min.X
		minY := bounds.Min.Y

		for y := 0; y < dstH; y++ {
			srcY := y * srcH / dstH
			rowOff := (dstY + y) * outputW * 4
			srcRow := (minY + srcY) * stride
			for x := 0; x < dstW; x++ {
				srcX := x * srcW / dstW
				c := lut[pix[srcRow+minX+srcX]]
				off := rowOff + (dstX+x)*4
				buf[off+0] = c.b
				buf[off+1] = c.g
				buf[off+2] = c.r
				buf[off+3] = c.a
			}
		}
	} else {
		for y := 0; y < dstH; y++ {
			srcY := y * srcH / dstH
			rowOff := (dstY + y) * outputW * 4
			for x := 0; x < dstW; x++ {
				srcX := x * srcW / dstW
				r, g, b, a := gameImg.At(bounds.Min.X+srcX, bounds.Min.Y+srcY).RGBA()
				off := rowOff + (dstX+x)*4
				buf[off+0] = byte(b >> 8)
				buf[off+1] = byte(g >> 8)
				buf[off+2] = byte(r >> 8)
				buf[off+3] = byte(a >> 8)
			}
		}
	}

	// Draw overlay on top of the game
	if withOverlay && d.overlayImg != nil {
		blitToBGRA(buf, d.overlayImg, 0, 0, outputW, outputH, outputW)
	}
}

// composeNative converts paletted image to BGRA at native resolution (no scaling).
// swscale in the encoder pipeline handles upscaling to 1920x1080.
// This is ~20x less data than composing at 1920x1080 (417KB vs 8MB).
// composeNative converts paletted image to BGRA at native resolution, centered
// in a 16:9 buffer (484x272) with black bars on the sides for correct aspect ratio.
func (d *StreamRenderer) composeNative(buf []byte, gameImg image.Image) {
	// Clear buffer (black bars)
	copy(buf, d.zeroBuf)

	bounds := gameImg.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	// Buffer width from srcDims (484 for 16:9 at height 272)
	bufW := len(buf) / (srcH * 4)
	dstX := (bufW - srcW) / 2 // center horizontally

	if pal, ok := gameImg.(*image.Paletted); ok {
		lut := &d.paletteLUT
		stride := pal.Stride
		pix := pal.Pix
		minX := bounds.Min.X
		minY := bounds.Min.Y

		for y := 0; y < srcH; y++ {
			srcRow := (minY+y)*stride + minX
			rowOff := y * bufW * 4
			for x := 0; x < srcW; x++ {
				c := lut[pix[srcRow+x]]
				off := rowOff + (dstX+x)*4
				buf[off+0] = c.b
				buf[off+1] = c.g
				buf[off+2] = c.r
				buf[off+3] = c.a
			}
		}
	} else {
		for y := 0; y < srcH; y++ {
			rowOff := y * bufW * 4
			for x := 0; x < srcW; x++ {
				r, g, b, a := gameImg.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
				off := rowOff + (dstX+x)*4
				buf[off+0] = byte(b >> 8)
				buf[off+1] = byte(g >> 8)
				buf[off+2] = byte(r >> 8)
				buf[off+3] = byte(a >> 8)
			}
		}
	}
}

// blitToBGRA alpha-blends an image onto the BGRA buffer.
// Transparent pixels are skipped, semi-transparent pixels are blended.
func blitToBGRA(buf []byte, img image.Image, dstX, dstY, dstW, dstH, stride int) {
	bounds := img.Bounds()
	maxX := bounds.Dx()
	maxY := bounds.Dy()
	if maxX > dstW-dstX {
		maxX = dstW - dstX
	}
	if maxY > dstH-dstY {
		maxY = dstH - dstY
	}

	// Fast path for *image.NRGBA (common PNG decode result)
	if nrgba, ok := img.(*image.NRGBA); ok {
		for y := 0; y < maxY; y++ {
			srcOff := (bounds.Min.Y+y-nrgba.Rect.Min.Y)*nrgba.Stride + (bounds.Min.X-nrgba.Rect.Min.X)*4
			dstOff := ((dstY+y)*stride + dstX) * 4
			for x := 0; x < maxX; x++ {
				si := srcOff + x*4
				a := uint16(nrgba.Pix[si+3])
				if a == 0 {
					continue
				}
				di := dstOff + x*4
				if a == 255 {
					buf[di+0] = nrgba.Pix[si+2]
					buf[di+1] = nrgba.Pix[si+1]
					buf[di+2] = nrgba.Pix[si+0]
					buf[di+3] = 255
				} else {
					invA := 255 - a
					buf[di+0] = byte((a*uint16(nrgba.Pix[si+2]) + invA*uint16(buf[di+0])) / 255)
					buf[di+1] = byte((a*uint16(nrgba.Pix[si+1]) + invA*uint16(buf[di+1])) / 255)
					buf[di+2] = byte((a*uint16(nrgba.Pix[si+0]) + invA*uint16(buf[di+2])) / 255)
					buf[di+3] = 255
				}
			}
		}
		return
	}

	for y := 0; y < maxY; y++ {
		for x := 0; x < maxX; x++ {
			r, g, b, a := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			if a == 0 {
				continue
			}
			off := ((dstY+y)*stride + (dstX + x)) * 4
			a8 := byte(a >> 8)
			if a8 == 255 {
				buf[off+0] = byte(b >> 8)
				buf[off+1] = byte(g >> 8)
				buf[off+2] = byte(r >> 8)
				buf[off+3] = 255
			} else {
				sa := uint16(a8)
				invA := 255 - sa
				buf[off+0] = byte((sa*uint16(b>>8) + invA*uint16(buf[off+0])) / 255)
				buf[off+1] = byte((sa*uint16(g>>8) + invA*uint16(buf[off+1])) / 255)
				buf[off+2] = byte((sa*uint16(r>>8) + invA*uint16(buf[off+2])) / 255)
				buf[off+3] = 255
			}
		}
	}
}

func (d *StreamRenderer) WriteAudio(data []byte) {
	if d.pipeline != nil {
		if err := d.pipeline.EncodeAudio(data); err != nil {
			fmt.Printf("Audio encode error: %v\n", err)
		}
	}
	if d.recordPipeline != nil {
		if err := d.recordPipeline.EncodeAudio(data); err != nil {
			fmt.Printf("Record audio encode error: %v\n", err)
		}
	}
}

func (d *StreamRenderer) GetContext() context.Context {
	return d.ctx
}

func (d *StreamRenderer) Shutdown() {
	fmt.Println("\n=== Shutting down stream ===")
	// Unregister signal handler so Ctrl+C returns to normal after stream stops
	if d.sigChan != nil {
		signal.Stop(d.sigChan)
	}
	// Cancel context first so Render loop and AudioReader stop sending frames
	if d.cancel != nil {
		d.cancel()
	}
	if d.pipeline != nil {
		fmt.Println("Closing stream pipeline...")
		d.pipeline.Close()
		d.pipeline = nil
	}
	if d.recordPipeline != nil {
		fmt.Println("Closing record pipeline...")
		d.recordPipeline.Close()
		d.recordPipeline = nil
	}
	fmt.Println("Shutdown complete")
}

// SetOverlay toggles the overlay compositing at runtime.
// Thread-safe — uses atomic flag, applied at next frame boundary.
// Forces a keyframe on the next frame to prevent flicker during scene change.
func (d *StreamRenderer) SetOverlay(enabled bool) {
	d.overlayEnabled.Store(enabled && d.overlayImg != nil)
	d.forceKeyframe.Store(true)
}

func (d *StreamRenderer) GetFPS() int {
	return d.config.FPS
}

func (d *StreamRenderer) GetRunMode() RunMode {
	return Loop
}
