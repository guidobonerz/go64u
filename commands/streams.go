package commands

import (
	"bufio"
	"fmt"
	"os"
	"strconv"

	"drazil.de/go64u/config"
	"drazil.de/go64u/imaging"
	"drazil.de/go64u/streams"

	"github.com/ebitengine/oto/v3"
	"github.com/spf13/cobra"
)

var lastStreamId = -1

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

func TwitchControllerCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "twitch",
		Short:   "controller for audio streams",
		Long:    "controller for audio streams",
		GroupID: "stream",
		Args:    cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			TwitchController()
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
		Use:     "screenshot",
		Short:   "Takes a screenshot of the current screen",
		Long:    "Takes a screenshot of the current screen",
		GroupID: "vic",
		Args:    cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {

			scaleFactor, _ := cmd.Flags().GetInt("scale")
			deviceName := config.GetConfig().SelectedDevice
			device := config.GetConfig().Devices[deviceName]
			stream("video", "start")
			rendererConfig := &streams.ImageRendererConfig{
				ScaleFactor: scaleFactor,
				ImageFormat: imaging.JPG,
				Quality:     90,
			}
			videoReader := &streams.VideoReader{
				Device:         device,
				RendererConfig: *rendererConfig,
			}
			videoReader.Read()

			//stream("video", "stop")
			fmt.Printf("screenshot taken from device\n: %s", deviceName)
		},
	}
	cmd.Flags().Int("scale", 100, "scale factor in percent(%)")
	return cmd
}

func TwitchController() {
	device := config.GetConfig().Devices["U64II"]
	stream("video", "start")
	/*
		rendererConfig := &streams.TwitchRendererConfig{
			ScaleFactor: 100,
			Fps:         30,
		}
	*/
	videoReader := &streams.VideoReader{
		Device: device,
		//RendererConfig: *rendererConfig,
	}
	videoReader.Read()
	//renderer.Run()

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
				if lastStreamId != i-1 || len(devices) == 1 {
					lastStreamId = i - 1
					StopStreamChannel()
					stopChan = make(chan struct{})
					streams.AudioStart(device)
					audioReader := streams.AudioReader{
						Device:       device,
						AudioContext: otoCtx,
						StopChan:     device.AudioChannel,
						Renderer:     nil,
					}
					go audioReader.Read()
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
				streams.VideoStart(device)
			case "audio":
				streams.AudioStart(device)
			case "debug":
				streams.DebugStart(device)
			}
		}
	case "stop":
		{
			switch name {
			case "video":
				streams.VideoStop(device)
			case "audio":
				streams.AudioStop(device)
			case "debug":
				streams.DebugStop(device)
			}
		}
	}
	//var url = fmt.Sprintf("streams/%s:%s?ip=%s:%d", name, command, getOutboundIP().String(), port)
	//network.Execute(url, http.MethodPut, nil)
}
