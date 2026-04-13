package commands

import (
	"fmt"
	"time"

	"drazil.de/go64u/config"
	"drazil.de/go64u/network"
	"drazil.de/go64u/util"

	"github.com/spf13/cobra"
)

func OnlineCheckCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "online",
		Short:   "Check if the selected device is online",
		Long:    "Checks if the currently selected device responds to HTTP requests",
		GroupID: "platform",
		Args:    cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			deviceName := config.GetConfig().SelectedDevice
			device := config.GetConfig().Devices[deviceName]
			if device == nil {
				fmt.Println("No device selected")
				return
			}
			if network.IsDeviceOnline(device, 2*time.Second) {
				fmt.Printf("%s[ONLINE]%s %s (%s)\n", util.Green, util.Reset, deviceName, device.IpAddress)
			} else {
				fmt.Printf("%s[OFFLINE]%s %s (%s)\n", util.Red, util.Reset, deviceName, device.IpAddress)
			}
		},
	}
}
