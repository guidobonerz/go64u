package streams

import (
	"fmt"
	"net"

	"drazil.de/go64u/config"
	"drazil.de/go64u/network"

	"drazil.de/go64u/util"
)

type StreamReader interface {
	Read()
}

const VIDEO_START = 0xff20
const AUDIO_START = 0xff21
const DEBUG_START = 0xff22

const VIDEO_STOP = 0xff30
const AUDIO_STOP = 0xff31
const DEBUG_STOP = 0xff32

func getUdpConnection(port int) (*net.UDPConn, error) {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", port))
	if err != nil {
		fmt.Println("Error resolving address:", err)
	}

	socket, err := net.ListenUDP("udp", &net.UDPAddr{Port: addr.Port})
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			fmt.Println("Read timeout occurred. Maybe stream not started")
		}
		panic(err)
	}
	//defer socket.Close()
	return socket, err
}
func AudioStart(device *config.Device) {
	startStream(AUDIO_START, device.AudioPort, device.IpAddress)
	device.AudioUdpConnection, _ = getUdpConnection(device.AudioPort)
}

func VideoStart(device *config.Device) {
	startStream(VIDEO_START, device.VideoPort, device.IpAddress)
	device.VideoUdpConnection, _ = getUdpConnection(device.VideoPort)
}

func DebugStart(device *config.Device) {
	startStream(DEBUG_START, device.DebugPort, device.IpAddress)
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

func startStream(command uint16, port int, targetAdress string) {
	length := []byte{0x00, 0x00}
	duration := []byte{0x00, 0x00}
	t := []byte(fmt.Sprintf("%s:%d", network.GetOutboundIP().String(), port))
	length[0] = byte(len(t) + 2)
	payload := make([]byte, len(t)+6)
	copy(payload[:], util.GetWordArray(command))
	copy(payload[2:], length[:])
	copy(payload[4:], duration[:])
	copy(payload[6:], t[:])
	network.SendTcpData(payload, targetAdress)
}

func stopStream(command uint16, target string) {
	payload := make([]byte, 4)
	copy(payload[:], util.GetWordArray(command))
	network.SendTcpData(payload, target)
}
