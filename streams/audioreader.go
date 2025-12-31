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
}

func (ar *AudioReader) Read() {
	socket := ar.Device.AudioUdpConnection
	pr, pw := io.Pipe()
	player := ar.AudioContext.NewPlayer(pr)
	player.SetBufferSize(770 * 4)
	done := make(chan struct{})
	go func() {
		defer pw.Close()
		defer close(done)
		buffer := make([]byte, 770)
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
			case <-time.After(100 * time.Millisecond):
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
