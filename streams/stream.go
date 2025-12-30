package streams

import (
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"drazil.de/go64u/config"
	"drazil.de/go64u/imaging"
	"drazil.de/go64u/network"
	"drazil.de/go64u/renderer"

	"drazil.de/go64u/util"
	"github.com/ebitengine/oto/v3"
)

const VIDEO_START = 0xff20
const AUDIO_START = 0xff21
const DEBUG_START = 0xff22

const VIDEO_STOP = 0xff30
const AUDIO_STOP = 0xff31
const DEBUG_STOP = 0xff32

func ReadAudioStream(otoCtx *oto.Context, renderer renderer.UpdateAudioSpectrum, port int, stopChan <-chan struct{}) {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", port))
	if err != nil {
		fmt.Println("Error resolving address:", err)
		return
	}

	socket, err := net.ListenUDP("udp", &net.UDPAddr{Port: addr.Port})
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			fmt.Println("Read timeout occurred. Maybe audio stream not started")
			return
		}
		panic(err)
	}
	defer socket.Close()
	pr, pw := io.Pipe()
	player := otoCtx.NewPlayer(pr)
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
				if renderer != nil {
					renderer(dataToWrite)
				}
				_, err := pw.Write(dataToWrite)
				writeDone <- err
			}()
			select {
			case <-stopChan:
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
	case <-stopChan:
	case <-done:
	}
	<-done
}

func ReadVideoStream(port int, renderer Renderer) {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", port))
	if err != nil {
		fmt.Println("Error resolving address:", err)
		return
	}

	socket, err := net.ListenUDP("udp", addr)
	socket.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			fmt.Println("Read timeout occurred. Maybe video stream not started")
			return
		}
		panic(err)
	}
	defer socket.Close()

	dataBuffer := make([]byte, 780)
	running := true
	count := 0
	offset := 0
	capture := false
	imageData := make([]byte, imaging.SIZE/2)
	for socket != nil && running {
		_, _, err := socket.ReadFromUDP(dataBuffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				fmt.Println("Read timeout occurred. Maybe video stream not started")
				return
			}
			panic(err)
		}
		var linenumber = util.GetWordFromArray(4, dataBuffer)
		if linenumber&0x8000 == 0x8000 {
			capture = true

			if count == 68 {
				capture = false

				if renderer.Render(imageData) {
					running = false
				}
				count = 0
			}
		}
		if capture {
			n := copy(imageData[offset:], dataBuffer[12:])
			offset += n
			count++
		}
	}
}

func AudioStart(device *config.Device) {
	startStream(AUDIO_START, fmt.Sprintf("%s:%d", network.GetOutboundIP().String(), device.AudioPort), device.IpAddress)
}

func VideoStart(device *config.Device) {
	startStream(VIDEO_START, fmt.Sprintf("%s:%d", network.GetOutboundIP().String(), device.VideoPort), device.IpAddress)
}

func DebugStart(device *config.Device) {
	startStream(DEBUG_START, fmt.Sprintf("%s:%d", network.GetOutboundIP().String(), device.DebugPort), device.IpAddress)
}

func AudioStop(device *config.Device) {
	stopStream(AUDIO_STOP, device.IpAddress)
}
func VideoStop(device *config.Device) {
	stopStream(VIDEO_STOP, device.IpAddress)
}
func DebugStop(device *config.Device) {
	stopStream(DEBUG_STOP, device.IpAddress)
}

func startStream(command uint16, source string, target string) {
	length := []byte{0x00, 0x00}
	duration := []byte{0x00, 0x00}
	t := []byte(source)
	length[0] = byte(len(t) + 2)
	payload := make([]byte, len(t)+6)
	copy(payload[:], util.GetWordArray(command))
	copy(payload[2:], length[:])
	copy(payload[4:], duration[:])
	copy(payload[6:], t[:])

	network.SendTcpData(payload, target)
}

func stopStream(command uint16, target string) {
	payload := make([]byte, 4)
	copy(payload[:], util.GetWordArray(command))
	network.SendTcpData(payload, target)
}
