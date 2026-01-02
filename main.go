package main

import (
	"fmt"
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

		mode := 0
		runInGuiMode, _ := cmd.Flags().GetBool("gui")
		runInTerminalMode, _ := cmd.Flags().GetBool("terminal")
		runInDatabaseMode, _ := cmd.Flags().GetBool("database")
		if len(args) == 0 && !runInGuiMode && !runInTerminalMode && !runInDatabaseMode {
			cmd.Help()
			return
		}

		if runInGuiMode {
			mode |= 1
		}
		if runInTerminalMode {
			mode |= 2
		}
		if runInDatabaseMode {
			mode |= 4
		}

		if mode != 1 && mode != 2 && mode != 4 {
			fmt.Println("Too much arugemnts. you have to decide select one mode")
			os.Exit(0)
		}

		if runInGuiMode {
			gui.Run()
		} else if runInTerminalMode {
			terminal.Run()
		} else if runInDatabaseMode {
			database.Run()
		}
	},
}

func main() {
	config.ReadConfig()
	database.Cache()
	setup.Setup(rootCmd, false)
	rootCmd.Flags().Bool("gui", false, "run the application in GUI(Graphics User Interface) mode")
	rootCmd.Flags().Bool("terminal", false, "run the application in terminal mode")
	rootCmd.Flags().Bool("database", false, "run the application in database mode")
	rootCmd.PersistentFlags().StringVarP(&config.GetConfig().SelectedDevice, "device", "d", config.GetConfig().SelectedDevice, "set device. needed in non terminal mode")
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	database.Cache()
}
