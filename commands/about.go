package commands

import (
	"de/drazil/go64u/network"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
)

func Version() *cobra.Command {
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
func DeviceInfo() *cobra.Command {
	return &cobra.Command{
		Use:     "info",
		Short:   "Show Device Info",
		Long:    "Show Device Info",
		GroupID: "platform",
		Args:    cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			info := network.Execute("info", http.MethodGet, nil)
			fmt.Println(string(info))
		},
	}
}
