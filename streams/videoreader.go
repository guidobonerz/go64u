package streams

import (
	"fmt"
	"net"
	"sync"
	"time"

	"drazil.de/go64u/config"
	"drazil.de/go64u/imaging"
	"drazil.de/go64u/util"
)

var framePool = sync.Pool{
	New: func() any {
		return make([]byte, imaging.SIZE/2)
	},
}

type VideoReader struct {
	Device *config.Device
	Renderer
}

func (vr *VideoReader) Read() {
	socket := vr.Device.VideoUdpConnection

	dataBuffer := make([]byte, 780)
	count := 0
	offset := 0
	capture := false
	imageData := make([]byte, imaging.SIZE/2)

	frameChan := make(chan []byte, 8) // Buffer frames to absorb UDP timing jitter

	go func() {
		defer close(frameChan)

		frameNum := 0

		for socket != nil {
			_, _, err := socket.ReadFromUDP(dataBuffer)

			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					fmt.Println("Read timeout occurred. Maybe video stream not started")
					return
				}
				fmt.Printf("UDP read error: %v\n", err)
				return
			}

			linenumber := util.GetWordFromArray(4, dataBuffer)

			if linenumber&0x8000 == 0x8000 {

				if capture && count == 68 {
					frameCopy := framePool.Get().([]byte)
					copy(frameCopy[:offset], imageData[:offset])
					frameCopy = frameCopy[:offset]

					frameNum++

					select {
					case frameChan <- frameCopy:
						//fmt.Printf("[UDP] Frame %d sent to channel\n", frameNum)
					default:
						//fmt.Println("[UDP] Warning: Frame buffer full, dropping frame")
					}
				}

				capture = true
				count = 0
				offset = 0

				if offset+len(dataBuffer[12:]) <= len(imageData) {
					n := copy(imageData[offset:], dataBuffer[12:])
					offset += n
					count++
				}
				continue
			}

			if capture {

				if offset+len(dataBuffer[12:]) > len(imageData) {
					fmt.Printf("[UDP] Buffer overflow: offset=%d, adding=%d, capacity=%d\n",
						offset, len(dataBuffer[12:]), len(imageData))

					count = 0
					offset = 0
					capture = false
					continue
				}

				n := copy(imageData[offset:], dataBuffer[12:])
				offset += n
				count++
			}
		}

	}()

	runMode := vr.Renderer.GetRunMode()

	if runMode == OneShot {

		frame, ok := <-frameChan
		if !ok {

			return
		}
		if !vr.Renderer.Render(frame) {
			fmt.Println("OneShot: Render failed")
		} else {
			fmt.Println("OneShot: Render successful")
		}
		return
	}

	fps := vr.Renderer.GetFPS()
	ticker := time.NewTicker(time.Second / time.Duration(fps))
	defer ticker.Stop()

	frameNumber := 0
	tickCount := 0
	for {
		tickCount++
		<-ticker.C
		var latestFrame []byte
		gotFrame := false
		drainCount := 0

	drainLoop:
		for {
			select {
			case frame, ok := <-frameChan:
				if !ok {
					fmt.Println("Frame channel closed, stopping")
					return
				}
				// Return previously drained frame to pool
				if latestFrame != nil {
					framePool.Put(latestFrame[:cap(latestFrame)])
				}
				latestFrame = frame
				gotFrame = true
				drainCount++
			default:
				break drainLoop
			}
		}
		if gotFrame {
			if !vr.Renderer.Render(latestFrame) {
				framePool.Put(latestFrame[:cap(latestFrame)])
				fmt.Println("Render failed, stopping stream")
				return
			}
			framePool.Put(latestFrame[:cap(latestFrame)])
			latestFrame = nil
			frameNumber++
		}
	}
}
