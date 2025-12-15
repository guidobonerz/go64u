package commands

import (
	"de/drazil/go64u/config"
	"de/drazil/go64u/network"
	"de/drazil/go64u/util"
	"fmt"
	"image"
	"image/png"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/nfnt/resize"
	"github.com/spf13/cobra"
)

/*
protected final static int VIC_STREAM_START_COMMAND = 0xff20;

	protected final static int VIC_STREAM_STOP_COMMAND = 0xff30;
*/
var scaleFactor = 100

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
			stream("video", "start")
		},
	}
	cmd.Flags().IntVarP(&scaleFactor, "scale", "s", 100, "scale factor in percent(%)")
	return cmd
}

func stream(name string, command string) {
	port := 11000
	switch name {
	case "video":
		port = config.GetConfig().Stream.Video.Port
	case "audio":
		port = config.GetConfig().Stream.Audio.Port
	case "debug":
		port = config.GetConfig().Stream.Debug.Port
	}
	var url = fmt.Sprintf("streams/%s:%s?ip=%s:%d", name, command, getOutboundIP().String(), port)
	network.Execute(url, http.MethodPut, nil)
	//startVideoStream(getOutboundIP().String())
	readVideoStream(port)
	//stopVideoStream()
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
			fmt.Println("Read timeout occurred. Maybe vic stream not started")
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
	imageData := make([]byte, 384*272/2)
	for socket != nil && running {

		_, _, err := socket.ReadFromUDP(dataBuffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				fmt.Println("Read timeout occurred. Maybe vic stream not started")
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
	img := image.NewPaletted(image.Rect(0, 0, 384, 272), util.GetPalette())
	pixelIndex := 0
	for _, b := range data {
		img.Pix[pixelIndex] = b & 0x0F
		pixelIndex++
		img.Pix[pixelIndex] = (b >> 4) & 0x0F
		pixelIndex++
		if pixelIndex >= 384*272 {
			break
		}
	}
	millisStr := strconv.FormatInt(time.Now().UnixMilli(), 10)
	file, err := os.Create(fmt.Sprintf("%sultimate_screenshot_%s.png", config.GetConfig().ScreenshotFolder, millisStr))
	if err != nil {
		panic(err)
	}
	defer file.Close()

	scaledWidth := float32(384) / float32(100) * float32(scaleFactor)

	scaledImage := resize.Resize(uint(scaledWidth), 0, img, resize.Bicubic)
	png.Encode(file, scaledImage)
	fmt.Printf("Screenshot successfully written to %s%s%s\n", util.Green, config.GetConfig().ScreenshotFolder, util.Reset)
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
