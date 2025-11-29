package commands

import (
	"de/drazil/go64u/network"
	"net/http"

	"github.com/spf13/cobra"
)

func About() *cobra.Command {
	return &cobra.Command{
		Use:   "togglemenu",
		Short: "Toggles the on-screen menu",
		Long:  `This command does the same thing as pressing the Menu button on an 1541 Ultimate cartridge, or briefly pressing the Multi Button on the Ultimate 64. The system will either enter or exit the Ultimate menu system depending on it's current state.`,
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			network.Execute("version", http.MethodGet, nil)
		},
	}
}
