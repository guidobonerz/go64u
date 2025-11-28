package main

import (
	"de/drazil/go64u/commands"
	"de/drazil/go64u/helper"
	"os"

	"github.com/spf13/cobra"
)

func main() {

	helper.ReadConfig()
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	if err := rootCmd.Execute(); err != nil {

		os.Exit(1)
	}

}

var rootCmd = &cobra.Command{
	Use:   "go64u",
	Short: "Ultimate64 Remote CLI",
	Long:  `go64U is a tool for remote interaction with the Ultimate64 computer`,
}

func init() {
	rootCmd.AddCommand(commands.GetPokeCommand())
	rootCmd.AddCommand(commands.GetPauseCommand())
	rootCmd.AddCommand(commands.GetPowerOffCommand())
	rootCmd.AddCommand(commands.GetRebootCommand())
	rootCmd.AddCommand(commands.GetResetCommand())
	rootCmd.AddCommand(commands.GetResumeCommand())
	rootCmd.AddCommand(commands.GetToggleMenuCommand())
}
