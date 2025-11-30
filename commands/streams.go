package commands

import (
	"de/drazil/go64u/helper"
	"de/drazil/go64u/network"
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

var scaleFactor = 100

func VideoStream() *cobra.Command {
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

func AudioStream() *cobra.Command {
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

func DebugStream() *cobra.Command {
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

func Screenshot() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "screenshot [format]",
		Short:   "Makes a screenshot of the current screen",
		Long:    "Makes a screenshot of the current screen",
		GroupID: "stream",
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
		port = helper.GetConfig().Stream.Video.Port
	case "audio":
		port = helper.GetConfig().Stream.Audio.Port
	case "debug":
		port = helper.GetConfig().Stream.Debug.Port
	}
	var url = fmt.Sprintf("streams/%s:%s?ip=%s:%d", name, command, getOutboundIP().String(), port)
	network.Execute(url, http.MethodPut, nil)
	readVideoStream(port)
}

func readVideoStream(port int) {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", port))
	if err != nil {
		fmt.Println("Error resolving address:", err)
		return
	}

	socket, err := net.ListenUDP("udp", addr)
	if err != nil {
		fmt.Println("Error creating socket:", err)
		return
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
			fmt.Println("Error receiving:", err)
			continue
		}
		var linenumber = helper.GetWordFromArray(4, dataBuffer)
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
	img := image.NewPaletted(image.Rect(0, 0, 384, 272), helper.GetPalette())
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
	file, err := os.Create(fmt.Sprintf("ultimate_screenshot_%s.png", millisStr))
	if err != nil {
		panic(err)
	}
	defer file.Close()

	scaledWidth := float32(384) / float32(100) * float32(scaleFactor)
	fmt.Printf("scale:%f", scaledWidth)
	scaledImage := resize.Resize(uint(scaledWidth), 0, img, resize.Bicubic)
	png.Encode(file, scaledImage)
	log.Println("Screenshot sucessfully written")
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
