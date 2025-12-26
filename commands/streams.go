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
	"drazil.de/go64u/util"

	"github.com/ebitengine/oto/v3"
	"github.com/fogleman/gg"
	"github.com/mattn/go-sixel"
	"github.com/nfnt/resize"
	"github.com/spf13/cobra"
)

/*
protected final static int VIC_STREAM_START_COMMAND = 0xff20;
protected final static int VIC_STREAM_STOP_COMMAND = 0xff30;
*/
var scaleFactor = 100
var showAsSixel = false
var lastStreamId = -1

const WIDTH = 384
const HEIGHT = 272
const SIZE = WIDTH * HEIGHT

type Device struct {
	Name  string
	Index int
}

var stopChan chan struct{}

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
			stream("video", "start")
			//stopVideoStream()
		},
	}
	cmd.Flags().IntVarP(&scaleFactor, "scale", "s", 100, "scale factor in percent(%)")
	cmd.Flags().Bool("sixel", false, "show screenshot as sixel graphic in terminal if supported")
	return cmd
}

func AudioController() {
	op := &oto.NewContextOptions{
		SampleRate:   48000,
		ChannelCount: 2,
		Format:       oto.FormatSignedInt16LE,
	}

	otoCtx, readyChan, err := oto.NewContext(op)
	if err != nil {
		panic(err)
	}
	<-readyChan

	devices := []Device{}

	i := 1
	fmt.Println("select stream number to play")
	for deviceName := range config.GetConfig().Devices {
		device := config.GetConfig().Devices[deviceName]
		fmt.Printf("[% 2d] - %s <%s:%s>\n", i, device.Description, device.IpAddress, device.AudioPort)
		devices = append(devices, Device{Name: deviceName, Index: i})
		i++
	}
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print(": ")
	for {
		if !scanner.Scan() {
			break
		}
		command := scanner.Text()
		fmt.Print("\033[1A\033[0G\033[2K: ")
		if isNumber(command) {
			i, _ := strconv.Atoi(command)
			if i > 0 && i <= len(devices) {
				port := config.GetConfig().Devices[devices[i-1].Name].AudioPort
				if lastStreamId != i-1 {
					lastStreamId = i - 1
					stopChan = make(chan struct{})
					stopStream()
					go ReadAudioStream(otoCtx, port, stopChan)
				}
			}
		} else if command == "q" {
			stopStream()
			break
		}

	}
}

func stopStream() {
	if stopChan != nil {
		close(stopChan)
	}

}

func isNumber(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

func stream(name string, command string) {
	port := 11000
	deviceName := "U64I"
	switch name {
	case "video":
		port = config.GetConfig().Devices[deviceName].VideoPort
		readVideoStream(port)
	case "audio":
		port = config.GetConfig().Devices[deviceName].AudioPort
		//ReadAudioStream(port)
	case "debug":
		port = config.GetConfig().Devices[deviceName].DebugPort
	}

	//var url = fmt.Sprintf("streams/%s:%s?ip=%s:%d", name, command, getOutboundIP().String(), port)
	//network.Execute(url, http.MethodPut, nil)

	//startVideoStream(getOutboundIP().String())
	//time.Sleep(time.Second)

}

func ReadAudioStream(otoCtx *oto.Context, port int, stopChan <-chan struct{}) {
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
			select {
			case <-stopChan:
				return
			default:
			}
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

func readVideoStream(port int) {

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

func getOutboundIP() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP
}

func startVideoStream(target string) {
	command := []byte{0xff, 0x20}
	length := []byte{0x00, 0x00}
	duration := []byte{0x00, 0x00}
	t := []byte(target)
	length[0] = byte(len(t) + 2)
	payload := make([]byte, len(t)+6)
	copy(payload[:], command[:])
	copy(payload[len(command):], length[:])
	copy(payload[len(command)+len(length):], duration[:])
	copy(payload[len(command)+len(length)+len(duration):], t[:])

	network.SendTcpData(payload)
}

func stopVideoStream() {
	network.SendTcpData([]byte{0xff, 0x30, 0x00, 0x00})
}
