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
var lastPath string = ""

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
var insideDiskimage bool
var entry *ftp.Entry
var mountedDiskImage []byte

func RemoteLsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "ls [path/diskimage]",
		Short:   "List files of the internal drive like USB Stick, SD Card, DiskImages, etc.",
		Long:    "List files of the internal drive like USB Stick, SD Card, DiskImages, etc.",
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
				f, _ := cmd.Flags().GetString("filter")
				if filter(entry.Name, f) {
					if entry.Type == ftp.EntryTypeFolder {
						fmt.Printf("%s %s%s\n", extensionIcon["dir"], util.Green, entry.Name)
					} else {
						var address = ""
						suffix := getSuffix(entry)
						showStart, _ := cmd.Flags().GetBool("memaddress")
						if showStart {
							address = "|----"
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
								address = fmt.Sprintf("|%04x", util.GetWordFromArray(0, content[:]))

							}

						}
						icon := extensionIcon[suffix]
						if icon == "" {
							icon = extensionIcon["default"]
						}
						valueParts := strings.Fields(humanize.Bytes(entry.Size))
						fmt.Printf("%s %s%-6s%s|%s%s\n", icon, util.Gray, fmt.Sprintf("%6s %-3s", valueParts[0], valueParts[1]), address, util.Blue, entry.Name)
					}
				}
			}

		},
	}
	cmd.Flags().BoolP("memaddress", "m", false, "displays the start address of a program if possible")
	cmd.Flags().StringP("filter", "f", "", "filters the list by a match pattern like '*.prg'")
	//cmd.Flags().BoolP("sort", "s", false, "displays the start address of a program if possible")

	return cmd
}

func RemoteCdCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "cd [path/diskimage]",
		Short:   "Change the folder on the internal drive. Makes only sense in REPL mode",
		Long:    "Change the folder on the internal drive. Makes only sense in REPL mode",
		GroupID: "file",
		Args:    cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			var c = network.GetFtpConnection()

			path := ""
			if len(args) > 0 {
				path = args[0]
			}

			var err error

			entry, err = c.GetEntry(path)
			if entry.Type == ftp.EntryTypeFile && !insideDiskimage && getSuffix(entry) == "d64" {
				insideDiskimage = true
				lastPath = CurrentPath
				CurrentPath = fmt.Sprintf("%s/%s", CurrentPath, entry.Name)
				var r *ftp.Response
				r, err = c.Retr(CurrentPath)
				mountedDiskImage = make([]byte, entry.Size)
				io.ReadFull(r, mountedDiskImage)
				r.Close()
			} else {
				if path == ".." && insideDiskimage {
					insideDiskimage = false
					CurrentPath = lastPath
				} else {
					err = c.ChangeDir(path)
					if err != nil {
						log.Fatal(err)
					}
					CurrentPath, err = c.CurrentDir()
				}
			}
			if err != nil {
				log.Fatal(err)
			}
		},
	}
}

func getSuffix(entry *ftp.Entry) string {
	return re.FindStringSubmatch(strings.ToLower(entry.Name))[1]
}
func filter(name string, pattern string) bool {
	match := false

	f := 0
	if strings.HasPrefix(pattern, "*") {
		f |= 1
		pattern = pattern[1:]
	}
	if strings.HasSuffix(pattern, "*") {
		f |= 2
		pattern = pattern[:len(pattern)-1]
	}
	switch f {
	case 1:
		match = strings.HasSuffix(name, pattern)
	case 2:
		match = strings.HasPrefix(name, pattern)
	case 3:
		match = strings.Contains(name, pattern)
	default:
		match = true
	}
	return match
}
