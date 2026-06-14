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
	AudioContext *oto.Context
	Renderer     renderer.UpdateAudioSpectrum
	StopChan     <-chan struct{}
	WriteAudioFn func(data []byte)
	// ShouldPlay gates local oto playback. When it returns false the audio is
	// still read and forwarded to Renderer (waveform, recording, casting) but
	// not played through the speakers — used so only the selected device is
	// audible. nil means always play.
	ShouldPlay func() bool
}

func (ar *AudioReader) Read() {
	socket := ar.Device.AudioUdpConnection
	buffer := make([]byte, 770)

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

	pr, pw := io.Pipe()
	player := ar.AudioContext.NewPlayer(pr)

	player.SetBufferSize(19200)
	// Reusable zero buffer played while muted. oto reads every player's source
	// from its mixing callback; if a muted device simply stopped writing, that
	// source would block and stall the whole mix (including the audible
	// device). Feeding silence keeps the pipe flowing so the mix never blocks.
	silence := make([]byte, len(buffer))
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
				// Muted devices play silence (keeps the oto mix flowing) but
				// still forward audio above for waveform/recording/casting.
				out := dataToWrite
				if ar.ShouldPlay != nil && !ar.ShouldPlay() {
					out = silence[:len(dataToWrite)]
				}
				_, err := pw.Write(out)
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
