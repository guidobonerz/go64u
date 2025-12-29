package commands

import (
	"bufio"
	"fmt"
	"image"
	"image/png"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"time"

	"drazil.de/go64u/config"
	"drazil.de/go64u/network"
	"drazil.de/go64u/renderer"
	"drazil.de/go64u/util"

	"github.com/ebitengine/oto/v3"
	"github.com/fogleman/gg"
	"github.com/mattn/go-sixel"
	"github.com/nfnt/resize"
	"github.com/spf13/cobra"
)

var scaleFactor = 100
var showAsSixel = false
var lastStreamId = -1

const WIDTH = 384
const HEIGHT = 272
const SIZE = WIDTH * HEIGHT
const VIDEO_START = 0xff20
const AUDIO_START = 0xff21
const DEBUG_START = 0xff22

const VIDEO_STOP = 0xff30
const AUDIO_STOP = 0xff31
const DEBUG_STOP = 0xff32

type Device struct {
	Name  string
	Index int
}

var stopChan chan struct{}
var otoCtx *oto.Context
var audioInitialized = false

func VideoStreamCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "video [command]",
		Short:   "Starts/Stops the video stream",
		Long:    "Starts/Stops the video stream",
		GroupID: "stream",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			stream("video", args[0])
		},
	}
}

func AudioStreamCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "audio [command]",
		Short:   "Starts/Stops the audio stream",
		Long:    "Starts/Stops the audio stream",
		GroupID: "stream",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			stream("audio", args[0])
		},
	}
}

func AudioStreamControllerCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "asc",
		Short:   "controller for audio streams",
		Long:    "controller for audio streams",
		GroupID: "stream",
		Args:    cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			AudioController()
		},
	}
}

func DebugStreamCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "debug [command]",
		Short:   "Starts/Stops the debug stream",
		Long:    "Starts/Stops the debug stream, audio and video streams will be then stopped",
		GroupID: "stream",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			stream("debug", args[0])
		},
	}
}

func ScreenshotCommand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "screenshot [format]",
		Short:   "Makes a screenshot of the current screen",
		Long:    "Makes a screenshot of the current screen",
		GroupID: "vic",
		Args:    cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			showAsSixel, _ = cmd.Flags().GetBool("sixel")
			deviceName := config.GetConfig().SelectedDevice
			device := config.GetConfig().Devices[deviceName]
			stream("video", "start")
			ReadVideoStream(device.VideoPort)
			//stream("video", "stop")
			fmt.Printf("screenshot taken from device: %s", deviceName)
		},
	}
	cmd.Flags().IntVarP(&scaleFactor, "scale", "s", 100, "scale factor in percent(%)")
	cmd.Flags().Bool("sixel", false, "show screenshot as sixel graphic in terminal if supported")
	return cmd
}

func AudioController() {
	if !audioInitialized {
		op := &oto.NewContextOptions{
			SampleRate:   48000,
			ChannelCount: 2,
			Format:       oto.FormatSignedInt16LE,
		}

		var readyChan chan struct{}
		var err error
		otoCtx, readyChan, err = oto.NewContext(op)
		if err != nil {
			panic(err)
		}
		audioInitialized = true
		<-readyChan
	}
	devices := []Device{}

	i := 1
	fmt.Println("select stream number to play")
	for deviceName := range config.GetConfig().Devices {
		device := config.GetConfig().Devices[deviceName]
		fmt.Printf("[%d] - %s <%s:%d>\n", i, device.Description, device.IpAddress, device.AudioPort)
		devices = append(devices, Device{Name: deviceName, Index: i})
		i++
	}
	fmt.Println("[R] - start recording audio stream (not yet implemented)")
	fmt.Println("[S] - stop recording audio stream (not yet implemented)")
	fmt.Println("[Q] - quit player")
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print(": ")
	for {
		if !scanner.Scan() {
			break
		}
		command := scanner.Text()
		if command == "q" {
			fmt.Print("\033[1A\033[0G\033[2K")
			fmt.Println("exit audio player")
			lastStreamId = -1
			StopStreamChannel()
			//stopStream()
			break
		}
		fmt.Print("\033[1A\033[0G\033[2K: ")
		if isNumber(command) {
			i, _ := strconv.Atoi(command)
			if i > 0 && i <= len(devices) {
				device := config.GetConfig().Devices[devices[i-1].Name]
				port := device.AudioPort
				if lastStreamId != i-1 || len(devices) == 1 {
					lastStreamId = i - 1
					StopStreamChannel()
					stopChan = make(chan struct{})
					StartStream(AUDIO_START, fmt.Sprintf("%s:%d", GetOutboundIP().String(), port), device.IpAddress)
					go ReadAudioStream(otoCtx, nil, port, stopChan)
				}
			}
		}
	}
}

func StopStreamChannel() {
	if stopChan != nil {
		close(stopChan)
		stopChan = nil
	}
}

func isNumber(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

func stream(name string, command string) {
	deviceName := config.GetConfig().SelectedDevice
	device := config.GetConfig().Devices[deviceName]
	switch command {
	case "start":
		{
			switch name {
			case "video":
				StartStream(VIDEO_START, fmt.Sprintf("%s:%d", GetOutboundIP().String(), device.VideoPort), device.IpAddress)
			case "audio":
				StartStream(AUDIO_START, fmt.Sprintf("%s:%d", GetOutboundIP().String(), device.AudioPort), device.IpAddress)
			case "debug":
				StartStream(DEBUG_START, fmt.Sprintf("%s:%d", GetOutboundIP().String(), device.DebugPort), device.IpAddress)
			}
		}
	case "stop":
		{
			switch name {
			case "video":
				StopStream(VIDEO_STOP, device.IpAddress)
			case "audio":
				StopStream(AUDIO_STOP, device.IpAddress)
			case "debug":
				StopStream(DEBUG_STOP, device.IpAddress)
			}
		}
	}
	//var url = fmt.Sprintf("streams/%s:%s?ip=%s:%d", name, command, getOutboundIP().String(), port)
	//network.Execute(url, http.MethodPut, nil)
}

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

func ReadVideoStream(port int) {

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
	imageData := make([]byte, WIDTH*HEIGHT/2)
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
				if writeImage(imageData, scaleFactor) {
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

func writeImage(data []byte, scaleFactor int) bool {
	img := image.NewPaletted(image.Rect(0, 0, WIDTH, HEIGHT), util.GetPalette())
	pixelIndex := 0

	for _, b := range data {
		img.Pix[pixelIndex] = b & 0x0F
		pixelIndex++
		img.Pix[pixelIndex] = (b >> 4) & 0x0F
		pixelIndex++
		if pixelIndex >= SIZE {
			break
		}
	}

	scaledWidth := float32(WIDTH) / float32(100) * float32(scaleFactor)

	scaledImage := resize.Resize(uint(scaledWidth), 0, img, resize.Bicubic)
	millisStr := strconv.FormatInt(time.Now().UnixMilli(), 10)
	if showAsSixel {
		dc := gg.NewContextForImage(scaledImage)
		sixel.NewEncoder(os.Stdout).Encode(dc.Image())
	} else {
		file, err := os.Create(fmt.Sprintf("%sultimate_screenshot_%s.png", config.GetConfig().ScreenshotFolder, millisStr))
		if err != nil {
			panic(err)
		}
		defer file.Close()

		png.Encode(file, scaledImage)
		fmt.Printf("Screenshot successfully written to %s%s%s\n", util.Green, config.GetConfig().ScreenshotFolder, util.Reset)
	}
	return true
}

func GetOutboundIP() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP
}

func StartStream(command uint16, source string, target string) {
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

func StopStream(command uint16, target string) {
	payload := make([]byte, 4)
	copy(payload[:], util.GetWordArray(command))
	network.SendTcpData(payload, target)
}
