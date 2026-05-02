package streams

import (
	"io"
	"log"
	"net"
	"time"

	"drazil.de/go64u/config"
	"drazil.de/go64u/renderer"
	"github.com/ebitengine/oto/v3"
)

type AudioReader struct {
	Device       *config.Device
	AudioContext *oto.Context                 // set for local playback mode
	Renderer     renderer.UpdateAudioSpectrum // optional spectrum callback
	StopChan     <-chan struct{}
	WriteAudioFn func(data []byte) // set for streaming mode — forwards audio to encoder pipeline
}

func (ar *AudioReader) Read() {
	socket := ar.Device.AudioUdpConnection
	buffer := make([]byte, 770)

	// Streaming mode: just forward audio to the encoder pipeline, no oto playback
	if ar.WriteAudioFn != nil {
		for {
			select {
			case <-ar.StopChan:
				return
			default:
			}
			socket.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			n, _, err := socket.ReadFromUDP(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				log.Println("Audio stream UDP read error:", err)
				return
			}
			ar.WriteAudioFn(buffer[2:n])
		}
	}

	// Local playback mode: play through oto
	pr, pw := io.Pipe()
	player := ar.AudioContext.NewPlayer(pr)
	// 100 ms @ 48 kHz stereo S16: 48000 * 2 * 2 / 10 = 19200 bytes. The
	// previous 770*4 (~16 ms) was tight enough to cause underruns on Linux
	// PipeWire/ALSA — anything below ~50 ms there crackles. Windows WASAPI
	// and macOS CoreAudio handle it fine, but the larger buffer is harmless
	// on those (just ~80 ms extra latency, imperceptible for a stream).
	player.SetBufferSize(19200)
	done := make(chan struct{})
	go func() {
		defer pw.Close()
		defer close(done)
		for {
			socket.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			n, _, err := socket.ReadFromUDP(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				log.Println("UDP read error:", err)
				break
			}
			dataToWrite := buffer[2:n]
			writeDone := make(chan error, 1)
			go func() {
				if ar.Renderer != nil {
					ar.Renderer(dataToWrite)
				}
				_, err := pw.Write(dataToWrite)
				writeDone <- err
			}()
			select {
			case <-ar.StopChan:
				return
			case err := <-writeDone:
				if err != nil {
					log.Println("Pipe write error:", err)
					return
				}
			case <-time.After(500 * time.Millisecond):
				// With a 100 ms player buffer, a write can legitimately block
				// for a few hundred ms during normal back-pressure. Only flag
				// this as truly stalled after 500 ms.
				log.Println("Write timeout - player may be stalled")
				if !player.IsPlaying() {
					log.Println("player stopped. Restart")
				}
			}
		}
	}()
	player.Play()
	select {
	case <-ar.StopChan:
	case <-done:
	}
	<-done
}
