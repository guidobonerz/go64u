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
	Long:  "go64u is a tool for remote interaction with the Ultimate64 computer",
}

func init() {
	rootCmd.AddCommand(commands.VersionCommand())
	rootCmd.AddCommand(commands.PokeCommand())
	rootCmd.AddCommand(commands.PeekCommand())
	rootCmd.AddCommand(commands.DumpPageCommand())
	rootCmd.AddCommand(commands.MessageCommand())
	rootCmd.AddCommand(commands.PauseCommand())
	rootCmd.AddCommand(commands.PowerOffCommand())
	rootCmd.AddCommand(commands.RebootCommand())
	rootCmd.AddCommand(commands.ResetCommand())
	rootCmd.AddCommand(commands.ResumeCommand())
	rootCmd.AddCommand(commands.ToggleMenuCommand())
	rootCmd.AddCommand(commands.FilesCommand())
	rootCmd.AddCommand(commands.LoadCommand())
	rootCmd.AddCommand(commands.RunCommand())
	rootCmd.AddCommand(commands.CrtCommand())
	rootCmd.AddCommand(commands.VideoStreamCommand())
	rootCmd.AddCommand(commands.AudioStreamCommand())
	rootCmd.AddCommand(commands.DebugStreamCommand())
	rootCmd.AddCommand(commands.ScreenshotCommand())
	rootCmd.AddCommand(commands.MountCommand())
	rootCmd.AddCommand(commands.UnmountCommand())
	rootCmd.AddCommand(commands.DeviceInfoCommand())
	rootCmd.AddCommand(commands.ScreenControlCommand())

}
