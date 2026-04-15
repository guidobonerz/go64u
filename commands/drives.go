package commands

import (
	"fmt"
	"net/http"
	"strings"

	"drazil.de/go64u/network"
	"drazil.de/go64u/util"

	"github.com/spf13/cobra"
)

func MountCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "mount [drive] [file]",
		Short:   "Mounts a diskimage [d64/g64/d71/g71/d81] on a given drive",
		Long:    "Mounts a diskimage [d64/g64/d71/g71/d81] on a given drive",
		GroupID: "runner",
		Args:    cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			if !isValidDrive(args[0]) {
				panic("not a valid drive name, A or B required")
			}
			payload, _ := util.ReadFile(args[1])
			network.SendHttpRequest(&network.HttpConfig{
				URL:     network.GetUrl(fmt.Sprintf("drives/%s:mount?type=d64&mode=readwrite", args[0])),
				Method:  http.MethodPost,
				Payload: payload,
			})
		},
	}
}

func UnmountCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "unmount [drive]",
		Short:   "Unmounts a diskimage from a given drive",
		Long:    "Unmounts a diskimage from a given drive",
		GroupID: "runner",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if !isValidDrive(args[0]) {
				panic("not a valid drive name, A or B required")
			}
			network.SendHttpRequest(&network.HttpConfig{
				URL:    network.GetUrl(fmt.Sprintf("drives/%s:remove?type=d64&mode=readwrite", args[0])),
				Method: http.MethodPut,
			})
		},
	}
}

func DrivesCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "drives",
		Short:   "Show drive info",
		Long:    "Show drive info",
		GroupID: "drives",
		Args:    cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(string(Drives()))
		},
	}
}

func Drives() []byte {
	return network.SendHttpRequest(&network.HttpConfig{
		URL:    network.GetUrl("drives"),
		Method: http.MethodGet,
	})
}

func isValidDrive(drive string) bool {
	return strings.ToLower(drive) == "a" || strings.ToLower(drive) == "b"
}
