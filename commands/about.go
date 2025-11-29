package commands

import (
	"de/drazil/go64u/network"
	"net/http"

	"github.com/spf13/cobra"
)

func Version() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Version of the REST API",
		Long:    `Returns the current version of the ReST API.`,
		GroupID: "platform",
		Args:    cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			network.Execute("version", http.MethodGet, nil)
		},
	}
}
