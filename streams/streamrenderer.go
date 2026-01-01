package streams

import (
	"context"
	"fmt"
	"image/png"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"drazil.de/go64u/imaging"
)

type StreamRenderer struct {
	Fps         int
	ScaleFactor int
	Url         string
	LogLevel    string
	cancel      context.CancelFunc
	stdin       io.WriteCloser
	cmd         *exec.Cmd
	ctx         context.Context
	config      StreamConfig
	frameCount  int
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
		Width:   1920,
		Height:  1080,
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
		fmt.Println("\nReceived interrupt, stopping stream...")
		d.Shutdown()
		os.Exit(0)
	}()

	d.cmd = exec.CommandContext(d.ctx, "ffmpeg",
		"-re", // Read input at native frame rate
		"-loglevel", d.LogLevel,

		// Video input from stdin
		"-f", "image2pipe",
		"-vcodec", "png",
		"-r", fmt.Sprintf("%d", d.config.FPS),
		"-video_size", fmt.Sprintf("%dx%d", d.config.Width, d.config.Height),
		"-i", "pipe:0",

		// Audio input (silent)
		"-f", "lavfi",
		"-i", "anullsrc=channel_layout=stereo:sample_rate=44100",

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

		// Output
		"-f", "flv",
		d.Url,
	)

	var err error
	d.stdin, err = d.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}
	fmt.Println("✓ Got stdin pipe")

	d.cmd.Stdout = os.Stdout
	d.cmd.Stderr = os.Stderr

	fmt.Println("Starting FFmpeg process...")
	if err = d.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}
	fmt.Println("✓ FFmpeg process started (PID:", d.cmd.Process.Pid, ")")

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

	fmt.Println("✓ FFmpeg stream initialization complete")
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

		img := imaging.GetImageFromBytes(data, 100)
		if img == nil {
			fmt.Printf("[Frame %d] ERROR: Failed to get image from bytes\n", d.frameCount)
			return false
		}

		if d.frameCount == 1 {
			bounds := img.Bounds()
			fmt.Printf("[Frame %d] Image size: %dx%d\n", d.frameCount, bounds.Dx(), bounds.Dy())
		}

		if err := png.Encode(d.stdin, img); err != nil {
			fmt.Printf("[Frame %d] ERROR: Failed to encode: %v\n", d.frameCount, err)
			return false
		}

		if d.frameCount%d.config.FPS == 0 {
			fmt.Printf("=== Streamed %d seconds (%d frames) ===\n",
				d.frameCount/d.config.FPS, d.frameCount)
		}

		return true
	}
}

func (d *StreamRenderer) GetContext() context.Context {
	return d.ctx
}

func (d *StreamRenderer) Shutdown() {
	fmt.Println("\n=== Shutting down stream ===")
	if d.stdin != nil {
		fmt.Println("Closing stdin pipe...")
		d.stdin.Close()
	}
	if d.cancel != nil {
		fmt.Println("Cancelling context...")
		d.cancel()
	}
	fmt.Println("Shutdown complete")
}

func (d *StreamRenderer) GetRunMode() RunMode {
	return Loop
}
