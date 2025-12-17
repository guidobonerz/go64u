package commands

import (
	"fmt"
	"log"
	"net/http"

	"drazil.de/go64u/network"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type DeviceInfo struct {
	Product         string `yaml:"product"`
	FirmwareVersion string `yaml:"firmware_version"`
	FPGAVersion     string `yaml:"fpga_version"`
	CoreVersion     string `yaml:"core_version"`
	Hostname        string `yaml:"hostname"`
	UniqueID        string `yaml:"unique_id"`
}

var deviceInfo DeviceInfo

func VersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Version of the REST API",
		Long:    "Returns the current version of the ReST API.",
		GroupID: "platform",
		Args:    cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			network.Execute("version", http.MethodGet, nil)
		},
	}
}
func DeviceInfoCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "info",
		Short:   "Show Device Info",
		Long:    "Show Device Info",
		GroupID: "platform",
		Args:    cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			info := network.Execute("info", http.MethodGet, nil)
			err := yaml.Unmarshal(info, &deviceInfo)
			if err != nil {
				log.Fatalf("Error parsing config file: %v", err)
			}
			fmt.Print("\n** Device Information **\n\n")
			fmt.Printf("Product         : %s\n", deviceInfo.Product)
			fmt.Printf("Firmware Version: %s\n", deviceInfo.FirmwareVersion)
			fmt.Printf("FPGA Version    : %s\n", deviceInfo.FPGAVersion)
			fmt.Printf("Core Version    : %s\n", deviceInfo.CoreVersion)
			fmt.Printf("Hostname        : %s\n", deviceInfo.Hostname)
			fmt.Printf("Unique ID       : %s\n", deviceInfo.UniqueID)
		},
	}
}
