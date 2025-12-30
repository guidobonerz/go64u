package streams

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"drazil.de/go64u/config"
	"drazil.de/go64u/imaging"
)

type TwitchRenderer struct {
	Fps         int
	ScaleFactor int
}

var synchronizer sync.RWMutex
var img image.Image

type StreamConfig struct {
	Width     int
	Height    int
	FPS       int
	Bitrate   string
	StreamKey string
}

func (d *TwitchRenderer) Run() {

	conf := StreamConfig{
		Width:     1920,
		Height:    1080,
		FPS:       30,
		Bitrate:   "3000k",
		StreamKey: config.GetConfig().TwitchStreamKey,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nReceived interrupt, stopping stream...")
		cancel()
	}()

	if err := StreamGeneratedImages(ctx, conf); err != nil && err != context.Canceled {
		log.Fatal(err)
	}

	fmt.Println("Stream stopped successfully")
}

func StreamGeneratedImages(ctx context.Context, config StreamConfig) error {
	rtmpURL := fmt.Sprintf("rtmp://live.twitch.tv/app/%s", config.StreamKey)

	// FFmpeg command to accept PNG images via stdin
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-f", "image2pipe",
		"-vcodec", "png",
		"-r", fmt.Sprintf("%d", config.FPS),
		"-video_size", fmt.Sprintf("%dx%d", config.Width, config.Height),
		"-i", "-", // Video input from stdin

		// Audio input options (must come before -i)
		"-f", "lavfi",
		"-i", "anullsrc=channel_layout=stereo:sample_rate=44100",

		// Output options
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-tune", "zerolatency",
		"-b:v", config.Bitrate,
		"-maxrate", config.Bitrate,
		"-bufsize", "6000k",
		"-pix_fmt", "yuv420p",
		"-g", fmt.Sprintf("%d", config.FPS*2),
		"-r", fmt.Sprintf("%d", config.FPS),
		"-c:a", "aac",
		"-b:a", "128k",
		"-f", "flv",
		rtmpURL,
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	fmt.Println("Stream started, generating frames...")

	frameNum := 0
	ticker := time.NewTicker(time.Second / time.Duration(config.FPS))
	defer ticker.Stop()

	errChan := make(chan error, 1)

	go func() {
		errChan <- cmd.Wait()
	}()

	for {
		select {
		case <-ctx.Done():
			stdin.Close()
			return ctx.Err()
		case err := <-errChan:
			return err
		case <-ticker.C:

			if img != nil {
				if err := png.Encode(stdin, img); err != nil {
					stdin.Close()
					return fmt.Errorf("failed to encode frame: %w", err)
				}

				frameNum++

				// Log progress
				if frameNum%config.FPS == 0 {
					fmt.Printf("Streamed %d frames (%d seconds)\n", frameNum, frameNum/config.FPS)
				}
			}
		}
	}
}

func (d *TwitchRenderer) Render(data []byte) bool {
	synchronizer.Lock()
	img = imaging.GetImageFromBytes(data, 100)
	fmt.Printf("length is:%d", len(data))
	defer synchronizer.Unlock()
	return true
}

func (d *TwitchRenderer) GetRunMode() RunMode {
	return Loop
}
