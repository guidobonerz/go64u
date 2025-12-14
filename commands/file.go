package commands

import (
	"de/drazil/go64u/network"
	"de/drazil/go64u/util"
	"fmt"
	"io"
	"log"
	"regexp"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/jlaffaye/ftp"
	"github.com/spf13/cobra"
)

var CurrentPath string = ""

var extensionIcon = map[string]string{

	"dir":     "\U0001F4C1",
	"prg":     "\U0001F4FA",
	"d64":     "\U0001F4BE",
	"d71":     "\U0001F4BE",
	"d81":     "\U0001F4BE",
	"crt":     "\U0001F9E9",
	"tap":     "\U0001F4FC",
	"default": "\U0001F4C4"}
var re = regexp.MustCompile(`\.(\w+)$`)

func RemoteLsCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "ls [path/diskimage] [filter]",
		Short:   "List files of the internal file storage like USB Stick etc. via ftp",
		Long:    "List files of the internal file storage like USB Stick etc. via ftp",
		GroupID: "file",
		Args:    cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			var c = network.GetFtpConnection()
			path := CurrentPath
			if len(args) > 0 {
				path = args[0]
			}
			entries, err := c.List(path)
			if err != nil {
				log.Fatal(err)
			}

			for _, entry := range entries {
				if entry.Type == ftp.EntryTypeFolder {
					fmt.Printf("%s %s%s\n", extensionIcon["dir"], util.Green, entry.Name)
				} else {

					suffix := re.FindStringSubmatch(strings.ToLower(entry.Name))[1]
					var start = "----"
					var err error
					var r *ftp.Response
					if suffix == "prg" {
						r, err = c.Retr(fmt.Sprintf("%s/%s", path, entry.Name))
						if err != nil {
							log.Printf("Error: %v\n", err)
							continue
						}
						var content [2]byte
						io.ReadFull(r, content[:])
						r.Close()
						start = fmt.Sprintf("%04x", util.GetWordFromArray(0, content[:]))

					}

					icon := extensionIcon[suffix]
					if icon == "" {
						icon = extensionIcon["default"]
					}
					fmt.Printf("%s %s%-6s|%s|%s%s\n", icon, util.Gray, humanize.Bytes(entry.Size), start, util.Blue, entry.Name)
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
