package commands

import (
	"de/drazil/go64u/network"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
)

func Files() *cobra.Command {
	return &cobra.Command{
		Use:   "ls [path]",
		Short: "Resets the U64",
		Long:  `This command returns basic information about a file, like size and extension.`,
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			var url = fmt.Sprintf("files%s", args[0])
			fmt.Println(url)
			network.Execute("files:info", http.MethodGet, nil)
		},
	}
}
