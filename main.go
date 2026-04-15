package main

import (
	"os"

	"drazil.de/go64u/config"
	"drazil.de/go64u/database"
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

		runInTerminalMode, _ := cmd.Flags().GetBool("terminal")

		if len(args) == 0 && !runInTerminalMode {
			gui.Run()
		} else if runInTerminalMode {
			terminal.Run()
		}
	},
}

func main() {
	rootCmd.Flags().Bool("terminal", false, "run the application in terminal mode")
	rootCmd.PersistentFlags().StringVarP(&config.GetConfig().SelectedDevice, "device", "d", config.GetConfig().SelectedDevice, "set device. needed in non terminal mode")
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
	setup.Setup(rootCmd, false)

}

func init() {
	config.ReadConfig()
	database.Cache()
}
