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
	rootCmd.AddGroup(&cobra.Group{ID: "platform", Title: "Platform Commands"})
	rootCmd.AddGroup(&cobra.Group{ID: "file", Title: "File Commands"})
	rootCmd.AddGroup(&cobra.Group{ID: "machine", Title: "Machine Commands"})
	rootCmd.AddGroup(&cobra.Group{ID: "runner", Title: "Runner Commands"})
	rootCmd.AddGroup(&cobra.Group{ID: "stream", Title: "Stream Commands"})
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "go64u",
	Short: "Ultimate64 Remote CLI",
	Long:  `go64u is a tool for remote interaction with the Ultimate64 computer`,
}

func init() {
	rootCmd.AddCommand(commands.Version())
	rootCmd.AddCommand(commands.Poke())
	rootCmd.AddCommand(commands.Peek())
	rootCmd.AddCommand(commands.DumpPage())
	rootCmd.AddCommand(commands.PrintAt())
	rootCmd.AddCommand(commands.Pause())
	rootCmd.AddCommand(commands.PowerOff())
	rootCmd.AddCommand(commands.Reboot())
	rootCmd.AddCommand(commands.Reset())
	rootCmd.AddCommand(commands.Resume())
	rootCmd.AddCommand(commands.ToggleMenu())
	rootCmd.AddCommand(commands.Files())
	rootCmd.AddCommand(commands.Load())
	rootCmd.AddCommand(commands.Run())
	rootCmd.AddCommand(commands.Crt())
	rootCmd.AddCommand(commands.VideoStream())
	rootCmd.AddCommand(commands.AudioStream())
	rootCmd.AddCommand(commands.DebugStream())
	rootCmd.AddCommand(commands.Screenshot())
}
