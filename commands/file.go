package commands

import (
	"log"

	"github.com/spf13/cobra"
)

func Files() *cobra.Command {
	return &cobra.Command{
		Use:     "ls [path]",
		Short:   "Resets the U64",
		Long:    "This command returns basic information about a file, like size and extension.",
		GroupID: "file",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			log.Println("Not yet implemented!!")
			/*var url = fmt.Sprintf("files%s", args[0])
			fmt.Println(url)
			network.Execute("files:info", http.MethodGet, nil)
			*/
		},
	}
}
