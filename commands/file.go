package commands

import (
	"de/drazil/go64u/helper"
	"fmt"
	"log"
	"time"

	"github.com/jlaffaye/ftp"
	"github.com/spf13/cobra"
)

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
func FtpLsCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "fls [path]",
		Short:   "List files of the internal file storage like USB Stick etc.",
		Long:    "This command returns basic information about a file, like size and extension.",
		GroupID: "file",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			c, err := ftp.Dial(fmt.Sprintf("%s:21", helper.GetConfig().IpAddress), ftp.DialWithTimeout(5*time.Second))
			if err != nil {
				log.Fatal(err)
			}
			err = c.Login("anonymous", "anonymous")
			if err != nil {
				log.Fatal(err)
			}
			entries, err := c.List(args[0])
			if err != nil {
				log.Fatal(err)
			}
			for _, entry := range entries {
				if entry.Type == ftp.EntryTypeFolder {
					fmt.Printf("<dir> %s\n", entry.Name)
				} else {
					fmt.Printf("%s\n", entry.Name)
				}

			}
		},
	}
}
func FtpCdCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "fcd [path]",
		Short:   "List files of the internal file storage like USB Stick etc.",
		Long:    "This command returns basic information about a file, like size and extension.",
		GroupID: "file",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			c, err := ftp.Dial(fmt.Sprintf("%s:21", helper.GetConfig().IpAddress), ftp.DialWithTimeout(5*time.Second))
			if err != nil {
				log.Fatal(err)
			}
			err = c.Login("anonymous", "anonymous")
			if err != nil {
				log.Fatal(err)
			}

			err = c.ChangeDir(args[0])
			if err != nil {
				log.Fatal(err)
			}
		},
	}
}
