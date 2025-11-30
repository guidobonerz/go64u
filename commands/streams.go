package commands

import (
	"de/drazil/go64u/helper"
	"fmt"
	"image"
	"image/png"
	"log"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"
)

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
	return &cobra.Command{
		Use:     "screenshot [format]",
		Short:   "Makes a screen of the current screen",
		Long:    "Makes a screen of the current screen",
		GroupID: "stream",
		Args:    cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			readStream(11000)
		},
	}
}

func stream(name string, command string) {
	//var url = fmt.Sprintf("streams/%s:%s?ip=%s:%s", name, command, getOutboundIP().String(), "11000")
	//network.Execute(url, http.MethodPut, nil)
	//channel := make(chan int)
	readStream(11000)
}

func readStream(port int) {
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

	dataBuffer := make([]byte, 780) // adjust size as needed
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
				if writeImage(imageData) {
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

func writeImage(data []byte) bool {
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

	png.Encode(file, img)
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
