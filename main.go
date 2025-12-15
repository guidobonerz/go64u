package main

import (
	"de/drazil/go64u/config"
	"de/drazil/go64u/setup"
	"de/drazil/go64u/terminal"
	"de/drazil/go64u/util"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "go64u",
	Short: "Ultimate64 Remote CLI",
	Long:  util.WhiteText("go64u is a tool for remote interaction with the Ultimate64 computer"),
}

func main() {
	config.ReadConfig()
	setup.Setup(rootCmd, false)

	rootCmd.AddGroup(&cobra.Group{ID: "terminal", Title: util.YellowText("Terminal Commands")})

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(terminal.TerminalCommand())
}
