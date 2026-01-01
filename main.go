package main

import (
	"fmt"
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
		runInTerminalMode, _ := cmd.Flags().GetBool("terminal")
		if len(args) == 0 && !runInGuiMode && !runInTerminalMode {
			cmd.Help()
			return
		}
		if runInGuiMode && runInTerminalMode {
			fmt.Println("you have to decide between gui and terminal mode")
			os.Exit(0)
		}

		if runInGuiMode {
			gui.Run()
		} else if runInTerminalMode {
			terminal.Run()
		}
	},
}

func main() {
	config.ReadConfig()

	setup.Setup(rootCmd, false)

	rootCmd.Flags().Bool("gui", false, "run the application in GUI(Graphics User Interface) mode")
	rootCmd.Flags().Bool("terminal", false, "run the application in terminal mode")
	rootCmd.PersistentFlags().StringVarP(&config.GetConfig().SelectedDevice, "device", "d", config.GetConfig().SelectedDevice, "set device. needed in non terminal mode")
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
}
