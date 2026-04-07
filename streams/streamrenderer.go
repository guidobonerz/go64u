package streams

import (
	"bytes"
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

const debugMuxer = false

type StreamRenderer struct {
	Fps         int
	ScaleFactor int
	Url         string
	LogLevel    string
	RecordPath  string // if set, also record to this file
	cancel      context.CancelFunc
	stdin       io.WriteCloser
	muxer       *MkvMuxer
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
		Width:   imaging.WIDTH,
		Height:  imaging.HEIGHT,
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
		// Don't os.Exit — let FFmpeg finalize the recording file
	}()

	fmt.Printf("Stream URL: [%s]\n", d.Url)
	if d.Url == "" {
		return fmt.Errorf("streaming URL is empty — check StreamingTargets in .go64u.yaml")
	}

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

	if d.RecordPath != "" {
		// Use tee muxer: stream to platform AND record to file simultaneously
		fmt.Printf("Recording to: %s\n", d.RecordPath)
		args = append(args,
			"-f", "tee",
			"-map", "0:v", "-map", "0:a",
			fmt.Sprintf("[f=flv]%s|[f=mp4]%s", d.Url, d.RecordPath),
		)
	} else {
		args = append(args, "-f", "flv", d.Url)
	}

	d.cmd = exec.CommandContext(d.ctx, "ffmpeg", args...)

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

		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			fmt.Printf("[Frame %d] ERROR: Failed to encode PNG: %v\n", d.frameCount, err)
			return false
		}

		d.muxer.WriteVideoFrame(buf.Bytes())

		if d.frameCount%d.config.FPS == 0 {
			fmt.Printf("=== Streamed %d seconds (%d frames) ===\n",
				d.frameCount/d.config.FPS, d.frameCount)
		}

		return true
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
	// Wait for FFmpeg to finish writing (moov atom for MP4, etc.)
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
