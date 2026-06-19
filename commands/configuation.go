package commands

import (
	"encoding/json"
	"fmt"
	"net/http"

	"drazil.de/go64u/network"

	"github.com/spf13/cobra"
)

func SIDSocketsConfigurationCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "sscc",
		Short:   "Show SIDSocketsConfiguration",
		Long:    "Show SIDSocketsConfiguration",
		GroupID: "configs",
		Args:    cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(string(SIDSocketsConfiguration()))

		},
	}
}

func SIDSocketsConfiguration() []byte {
	return network.SendHttpRequest(&network.HttpConfig{
		URL:    network.GetUrl("configs/SID%20Sockets%20Configuration"),
		Method: http.MethodGet,
	})
}

// SIDSocketsConfigurationForDevice fetches the SID Sockets Configuration of a
// specific device by IP (the monitor can show several devices, so it cannot
// rely on the globally selected device used by GetUrl).
func SIDSocketsConfigurationForDevice(ip string) []byte {
	return network.SendHttpRequest(&network.HttpConfig{
		URL:    fmt.Sprintf("http://%s/v1/configs/SID%%20Sockets%%20Configuration", ip),
		Method: http.MethodGet,
	})
}

// EnabledSIDSocketCount parses a SID Sockets Configuration response and returns
// how many of the two sockets report "Enabled" (0 on any parse error).
func EnabledSIDSocketCount(jsonData []byte) int {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(jsonData, &root); err != nil {
		return 0
	}
	inner, ok := root["SID Sockets Configuration"]
	if !ok {
		return 0
	}
	var cfg map[string]string
	if err := json.Unmarshal(inner, &cfg); err != nil {
		return 0
	}
	count := 0
	if cfg["SID Socket 1"] == "Enabled" {
		count++
	}
	if cfg["SID Socket 2"] == "Enabled" {
		count++
	}
	return count
}
