package commands

import (
	"de/drazil/go64u/network"
	"de/drazil/go64u/util"
	"fmt"
	"log"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/jlaffaye/ftp"
	"github.com/spf13/cobra"
)

var CurrentPath string = ""

func FilesCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "ls [path]",
		Short:   "List files of the internal file storage like USB Stick etc.",
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
func RemoteLsCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "ls [path/diskimage] [filter]",
		Short:   "List files of the internal file storage like USB Stick etc. via ftp",
		Long:    "List files of the internal file storage like USB Stick etc. via ftp",
		GroupID: "file",
		Args:    cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			var c = network.GetFtpConnection()
			path := ""
			if len(args) > 0 {
				path = args[0]
			}

			entries, err := c.List(path)
			if err != nil {
				log.Fatal(err)
			}
			for _, entry := range entries {

				if entry.Type == ftp.EntryTypeFolder {
					fmt.Printf("%s\U0001F4C1 %s%-24s\n", util.Yellow, util.Green, entry.Name)
				} else {
					if strings.HasSuffix(entry.Name, "d64") {
						fmt.Printf("\U0001F4BE %s%-30s - %s%s\n", util.Blue, entry.Name, util.Gray, humanize.Bytes(245234525))
					} else {
						fmt.Printf("\U0001F4C4 %s%-30s - %s%s\n", util.Blue, entry.Name, util.Gray, humanize.Bytes(entry.Size))
					}
				}
			}
		},
	}
}

func RemoteCdCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "cd [path]",
		Short:   "changes the folder on the ultimate64 via ftp",
		Long:    "changes the folder on the ultimate64 via ftp",
		GroupID: "file",
		Args:    cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			var c = network.GetFtpConnection()
			path := ""
			if len(args) > 0 {
				path = args[0]
			}

			err := c.ChangeDir(path)
			if err != nil {
				log.Fatal(err)
			}
			CurrentPath, err = c.CurrentDir()
			if err != nil {
				log.Fatal(err)
			}
		},
	}
}
