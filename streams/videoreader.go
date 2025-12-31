package streams

import (
	"fmt"
	"net"

	"drazil.de/go64u/config"
	"drazil.de/go64u/imaging"
	"drazil.de/go64u/util"
)

type VideoReader struct {
	Device         *config.Device
	RendererConfig ImageRendererConfig
}

func (vr *VideoReader) Read() {
	socket := vr.Device.VideoUdpConnection

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

				if vr.RendererConfig.Render(imageData) {
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
