package streams

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"drazil.de/go64u/config"
	"drazil.de/go64u/imaging"
)

const debugMuxer = false

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
	stdin          io.WriteCloser
	muxer          *MkvMuxer
	cmd            *exec.Cmd
	recordStdin    io.WriteCloser // separate recording pipeline (nil when not needed)
	recordMuxer    *MkvMuxer      // separate recording muxer (nil when not needed)
	recordCmd      *exec.Cmd      // separate recording FFmpeg (nil when not needed)
	ctx            context.Context
	config         StreamConfig
	frameCount     int
	overlayImg     image.Image // loaded once in Init, nil if no overlay
	bgraBuf        []byte      // reused BGRA output buffer (outputW*outputH*4)
	recordBuf      []byte      // reused BGRA buffer for clean recording frames (nil when not needed)
	dualPipeline       bool // true when stream and record have different overlay settings
	streamWithOverlay  bool // whether the stream output gets the overlay
	recordWithOverlay  bool // whether the record output gets the overlay
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

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
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
		fmt.Printf("✓ Overlay loaded: %s (%dx%d)\n", overlay.ImagePath,
			img.Bounds().Dx(), img.Bounds().Dy())
	}

	bufSize := outputW * outputH * 4
	d.bgraBuf = make([]byte, bufSize)
	d.zeroBuf = make([]byte, bufSize)
	d.reusableImg = imaging.NewReusablePalettedImage()

	hasStream := d.Url != ""
	hasRecord := d.RecordPath != ""

	noOverlayStream := d.NoOverlay == "stream" || d.NoOverlay == "both"
	noOverlayRecord := d.NoOverlay == "record" || d.NoOverlay == "both"

	// Strip overlay image if disabled for all outputs
	if noOverlayStream && noOverlayRecord {
		d.overlayImg = nil
	}

	d.streamWithOverlay = d.overlayImg != nil && !noOverlayStream
	d.recordWithOverlay = d.overlayImg != nil && !noOverlayRecord

	// Need dual pipeline when streaming + recording have different overlay settings
	d.dualPipeline = hasStream && hasRecord && d.streamWithOverlay != d.recordWithOverlay

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

	fmt.Println("✓ Stream initialization complete")
	fmt.Println("Ready to receive frames...")
	return nil
}

func (d *StreamRenderer) baseFFmpegArgs() []string {
	return []string{
		"-loglevel", d.LogLevel,
		"-f", "matroska",
		"-i", "pipe:0",
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-tune", "zerolatency",
		"-b:v", d.config.Bitrate,
		"-maxrate", d.config.Bitrate,
		"-bufsize", "6000k",
		"-pix_fmt", "yuv420p",
		"-g", fmt.Sprintf("%d", d.config.FPS*2),
		"-c:a", "aac",
		"-b:a", "128k",
		"-ar", "44100",
	}
}

func (d *StreamRenderer) initSinglePipeline(hasStream, hasRecord bool) error {
	args := d.baseFFmpegArgs()

	if hasStream && hasRecord {
		fmt.Printf("Streaming to: %s\n", d.Url)
		fmt.Printf("Recording to: %s (mode: %s)\n", d.RecordPath, d.RecordMode)

		var recordOutput string
		switch d.RecordMode {
		case "audio":
			recordOutput = "[f=mp4:select=\\'a\\']" + d.RecordPath
		case "video":
			recordOutput = "[f=mp4:select=\\'v\\']" + d.RecordPath
		default:
			recordOutput = "[f=mp4]" + d.RecordPath
		}

		args = append(args,
			"-f", "tee",
			"-map", "0:v", "-map", "0:a",
			fmt.Sprintf("[f=flv]%s|%s", d.Url, recordOutput),
		)
	} else if hasRecord {
		fmt.Printf("Recording to: %s (mode: %s)\n", d.RecordPath, d.RecordMode)

		switch d.RecordMode {
		case "audio":
			args = append(args, "-map", "0:a", "-f", "mp4", d.RecordPath)
		case "video":
			args = append(args, "-map", "0:v", "-an", "-f", "mp4", d.RecordPath)
		default:
			args = append(args, "-f", "mp4", d.RecordPath)
		}
	} else {
		fmt.Printf("Streaming to: %s\n", d.Url)
		args = append(args, "-f", "flv", d.Url)
	}

	var err error
	d.cmd, d.stdin, d.muxer, err = d.startFFmpeg(args, "ffmpeg.log")
	if err != nil {
		return err
	}
	d.monitorFFmpeg(d.cmd)
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

	// Stream pipeline (with overlay)
	streamArgs := d.baseFFmpegArgs()
	streamArgs = append(streamArgs, "-f", "flv", d.Url)

	var err error
	d.cmd, d.stdin, d.muxer, err = d.startFFmpeg(streamArgs, "ffmpeg_stream.log")
	if err != nil {
		return err
	}
	d.monitorFFmpeg(d.cmd)

	// Record pipeline (without overlay)
	recordArgs := d.baseFFmpegArgs()
	switch d.RecordMode {
	case "audio":
		recordArgs = append(recordArgs, "-map", "0:a", "-f", "mp4", d.RecordPath)
	case "video":
		recordArgs = append(recordArgs, "-map", "0:v", "-an", "-f", "mp4", d.RecordPath)
	default:
		recordArgs = append(recordArgs, "-f", "mp4", d.RecordPath)
	}

	d.recordCmd, d.recordStdin, d.recordMuxer, err = d.startFFmpeg(recordArgs, "ffmpeg_record.log")
	if err != nil {
		d.stdin.Close()
		d.cancel()
		return err
	}
	d.monitorFFmpeg(d.recordCmd)

	return nil
}

func (d *StreamRenderer) startFFmpeg(args []string, logName string) (*exec.Cmd, io.WriteCloser, *MkvMuxer, error) {
	cmd := exec.CommandContext(d.ctx, "ffmpeg", args...)

	logFile, err := os.Create(logName)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create log file %s: %w", logName, err)
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	fmt.Printf("✓ FFmpeg log: %s\n", logName)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	fmt.Println("Starting FFmpeg process...")
	if err = cmd.Start(); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to start ffmpeg: %w", err)
	}
	fmt.Println("✓ FFmpeg process started (PID:", cmd.Process.Pid, ")")

	var muxWriter io.Writer = stdin
	if debugMuxer {
		debugFile, derr := os.Create("debug_" + logName + ".mkv")
		if derr == nil {
			fmt.Printf("✓ Debug file: debug_%s.mkv\n", logName)
			muxWriter = io.MultiWriter(stdin, debugFile)
		}
	}
	muxer, err := NewMkvMuxer(muxWriter, d.config.Width, d.config.Height, d.config.FPS, true)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create muxer: %w", err)
	}
	fmt.Println("✓ Matroska muxer initialized")

	return cmd, stdin, muxer, nil
}

func (d *StreamRenderer) monitorFFmpeg(cmd *exec.Cmd) {
	go func() {
		fmt.Printf("Monitoring FFmpeg process (PID: %d)...\n", cmd.Process.Pid)
		err := cmd.Wait()
		if err != nil {
			fmt.Printf("\n!!! FFmpeg (PID: %d) exited with error: %v\n", cmd.Process.Pid, err)
		} else {
			fmt.Printf("\nFFmpeg (PID: %d) exited normally\n", cmd.Process.Pid)
		}
		if d.cancel != nil {
			d.cancel()
		}
	}()
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
		}

		if d.dualPipeline {
			// Record pipeline
			d.composeFrameInto(d.recordBuf, img, d.recordWithOverlay)
			d.recordMuxer.WriteVideoFrame(d.recordBuf)

			// Stream pipeline
			d.composeFrameInto(d.bgraBuf, img, d.streamWithOverlay)
			d.muxer.WriteVideoFrame(d.bgraBuf)
		} else {
			d.composeFrameInto(d.bgraBuf, img, d.streamWithOverlay || d.recordWithOverlay)
			d.muxer.WriteVideoFrame(d.bgraBuf)
		}

		if d.frameCount%d.config.FPS == 0 {
			fmt.Printf("=== Streamed %d seconds (%d frames) ===\n",
				d.frameCount/d.config.FPS, d.frameCount)
		}

		return true
	}
}

func (d *StreamRenderer) composeFrameInto(buf []byte, gameImg image.Image, withOverlay bool) {
	// Fast clear via copy from pre-zeroed buffer
	copy(buf, d.zeroBuf)

	bounds := gameImg.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

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

	// Clamp destination rect to buffer bounds
	if dstX+dstW > outputW {
		dstW = outputW - dstX
	}
	if dstY+dstH > outputH {
		dstH = outputH - dstY
	}

	// Nearest-neighbor scale game directly into BGRA buffer
	if pal, ok := gameImg.(*image.Paletted); ok {
		// Build palette LUT once (palette never changes across frames)
		if !d.paletteLUTReady {
			for i, c := range pal.Palette {
				if i >= 16 {
					break
				}
				r, g, b, a := c.RGBA()
				d.paletteLUT[i] = bgraColor{byte(b >> 8), byte(g >> 8), byte(r >> 8), byte(a >> 8)}
			}
			d.paletteLUTReady = true
		}
		lut := &d.paletteLUT
		stride := pal.Stride
		pix := pal.Pix
		minX := bounds.Min.X
		minY := bounds.Min.Y

		// No per-pixel bounds check needed — dstW/dstH are clamped above
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
		// Generic fallback for non-paletted images
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
	if d.muxer == nil {
		return
	}
	d.muxer.WriteAudio(data)
}

func (d *StreamRenderer) GetMuxer() *MkvMuxer {
	return d.muxer
}

func (d *StreamRenderer) GetRecordMuxer() *MkvMuxer {
	return d.recordMuxer
}

func (d *StreamRenderer) GetContext() context.Context {
	return d.ctx
}

func (d *StreamRenderer) Shutdown() {
	fmt.Println("\n=== Shutting down stream ===")
	if d.stdin != nil {
		fmt.Println("Closing stream stdin pipe...")
		d.stdin.Close()
		d.stdin = nil
	}
	if d.recordStdin != nil {
		fmt.Println("Closing record stdin pipe...")
		d.recordStdin.Close()
		d.recordStdin = nil
	}
	if d.cmd != nil && d.cmd.Process != nil {
		fmt.Println("Waiting for stream FFmpeg to finish...")
		d.cmd.Wait()
	}
	if d.recordCmd != nil && d.recordCmd.Process != nil {
		fmt.Println("Waiting for record FFmpeg to finish...")
		d.recordCmd.Wait()
	}
	if d.cancel != nil {
		d.cancel()
	}
	fmt.Println("Shutdown complete")
}

func (d *StreamRenderer) GetFPS() int {
	return d.config.FPS
}

func (d *StreamRenderer) GetRunMode() RunMode {
	return Loop
}
