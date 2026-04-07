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
	Fps         int
	ScaleFactor int
	Url         string
	LogLevel    string
	RecordPath  string // if set, also record to this file
	RecordMode  string // "audio", "video", or "both"
	cancel      context.CancelFunc
	stdin       io.WriteCloser
	muxer       *MkvMuxer
	cmd         *exec.Cmd
	ctx         context.Context
	config      StreamConfig
	frameCount  int
	overlayImg image.Image // loaded once in Init, nil if no overlay
	bgraBuf    []byte      // reused BGRA output buffer (outputW*outputH*4)
}

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
		FPS:     30,
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

	// Reusable compositing context and fast PNG encoder
	d.bgraBuf = make([]byte, outputW*outputH*4)

	args := []string{
		"-loglevel", d.LogLevel,

		// Single Matroska input carrying both video and audio
		"-f", "matroska",
		"-i", "pipe:0",

		// Video encoding
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-tune", "zerolatency",
		"-b:v", d.config.Bitrate,
		"-maxrate", d.config.Bitrate,
		"-bufsize", "6000k",
		"-pix_fmt", "yuv420p",
		"-g", fmt.Sprintf("%d", d.config.FPS*2),

		// Audio encoding
		"-c:a", "aac",
		"-b:a", "128k",
		"-ar", "44100",
	}

	hasStream := d.Url != ""
	hasRecord := d.RecordPath != ""

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

	d.cmd = exec.CommandContext(d.ctx, "ffmpeg", args...)

	logFile, err := os.Create("ffmpeg.log")
	if err != nil {
		return fmt.Errorf("failed to create ffmpeg log file: %w", err)
	}
	d.cmd.Stdout = logFile
	d.cmd.Stderr = logFile
	fmt.Println("✓ FFmpeg log: ffmpeg.log")

	d.stdin, err = d.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}
	fmt.Println("✓ Got stdin pipe")

	fmt.Println("Starting FFmpeg process...")
	if err = d.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}
	fmt.Println("✓ FFmpeg process started (PID:", d.cmd.Process.Pid, ")")

	// Create Matroska muxer writing to FFmpeg's stdin
	var muxWriter io.Writer = d.stdin
	if debugMuxer {
		debugFile, derr := os.Create("debug_stream.mkv")
		if derr == nil {
			fmt.Println("✓ Debug file: debug_stream.mkv")
			muxWriter = io.MultiWriter(d.stdin, debugFile)
		}
	}
	d.muxer, err = NewMkvMuxer(muxWriter, d.config.Width, d.config.Height, d.config.FPS, true)
	if err != nil {
		d.cancel()
		return fmt.Errorf("failed to create muxer: %w", err)
	}
	fmt.Println("✓ Matroska muxer initialized")

	go func() {
		fmt.Println("Monitoring FFmpeg process...")
		err := d.cmd.Wait()
		if err != nil {
			fmt.Printf("\n!!! FFmpeg exited with error: %v\n", err)
		} else {
			fmt.Println("\nFFmpeg exited normally")
		}
		if d.cancel != nil {
			d.cancel()
		}
	}()

	fmt.Println("✓ Stream initialization complete")
	fmt.Println("Ready to receive frames...")
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

		img := imaging.GetImageFromBytes(data, d.ScaleFactor)
		if img == nil {
			fmt.Printf("[Frame %d] ERROR: Failed to get image from bytes\n", d.frameCount)
			return false
		}

		if d.frameCount == 1 {
			bounds := img.Bounds()
			fmt.Printf("[Frame %d] Image size: %dx%d\n", d.frameCount, bounds.Dx(), bounds.Dy())
		}

		// Compose 1920x1080 output frame and send raw BGRA pixels
		d.composeFrame(img)
		d.muxer.WriteVideoFrame(d.bgraBuf)

		if d.frameCount%d.config.FPS == 0 {
			fmt.Printf("=== Streamed %d seconds (%d frames) ===\n",
				d.frameCount/d.config.FPS, d.frameCount)
		}

		return true
	}
}

func (d *StreamRenderer) composeFrame(gameImg image.Image) {
	buf := d.bgraBuf

	// Clear to black
	for i := range buf {
		buf[i] = 0
	}

	bounds := gameImg.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	var dstX, dstY, dstW, dstH int

	if d.overlayImg != nil {
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

	// Nearest-neighbor scale game directly into BGRA buffer
	for y := 0; y < dstH; y++ {
		srcY := y * srcH / dstH
		for x := 0; x < dstW; x++ {
			srcX := x * srcW / dstW
			r, g, b, a := gameImg.At(bounds.Min.X+srcX, bounds.Min.Y+srcY).RGBA()
			off := ((dstY+y)*outputW + (dstX + x)) * 4
			if off+3 < len(buf) {
				buf[off+0] = byte(b >> 8)
				buf[off+1] = byte(g >> 8)
				buf[off+2] = byte(r >> 8)
				buf[off+3] = byte(a >> 8)
			}
		}
	}

	// Draw overlay on top of the game
	if d.overlayImg != nil {
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

func (d *StreamRenderer) GetContext() context.Context {
	return d.ctx
}

func (d *StreamRenderer) Shutdown() {
	fmt.Println("\n=== Shutting down stream ===")
	if d.stdin != nil {
		fmt.Println("Closing stdin pipe (FFmpeg will finalize output)...")
		d.stdin.Close()
		d.stdin = nil
	}
	if d.cmd != nil && d.cmd.Process != nil {
		fmt.Println("Waiting for FFmpeg to finish...")
		d.cmd.Wait()
	}
	if d.cancel != nil {
		d.cancel()
	}
	fmt.Println("Shutdown complete")
}

func (d *StreamRenderer) GetRunMode() RunMode {
	return Loop
}
