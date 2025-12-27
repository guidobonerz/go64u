package main

import (
	"os"

	"drazil.de/go64u/config"
	"drazil.de/go64u/gui"
	"drazil.de/go64u/setup"
	"drazil.de/go64u/terminal"
	"drazil.de/go64u/util"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "go64u",
	Short: "Ultimate64 Remote CLI",
	Long:  util.WhiteText("go64u is a tool for remote interaction with the Ultimate64 computer"),
	Run: func(cmd *cobra.Command, args []string) {

		runInGuiMode, _ := cmd.Flags().GetBool("gui")

		if len(args) == 0 && !runInGuiMode {
			cmd.Help()
			return
		}

		if runInGuiMode {
			gui.Run()

		}
	},
}

func main() {
	config.ReadConfig()

	setup.Setup(rootCmd, false)

	rootCmd.Flags().Bool("gui", false, "run the application in GUI(Graphics User Interface) mode")
	rootCmd.AddGroup(&cobra.Group{ID: "terminal", Title: util.YellowText("Terminal Commands")})

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}

}

func init() {
	rootCmd.AddCommand(terminal.TerminalCommand())
}
