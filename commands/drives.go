package commands

import (
	"de/drazil/go64u/helper"
	"de/drazil/go64u/network"
	"fmt"
	"net/http"
	"strings"

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
			payload, _ := helper.ReadFile(args[1])
			network.Execute(fmt.Sprintf("drives/%s:mount?type=d64&mode=readwrite", args[0]), http.MethodPost, payload)
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
			network.Execute(fmt.Sprintf("drives/%s:remove?type=d64&mode=readwrite", args[0]), http.MethodPut, nil)
		},
	}
}

func isValidDrive(drive string) bool {
	return strings.ToLower(drive) == "a" || strings.ToLower(drive) == "b"
}
