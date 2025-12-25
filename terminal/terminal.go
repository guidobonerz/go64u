package terminal

import (
	"bufio"

	"drazil.de/go64u/commands"
	"drazil.de/go64u/setup"

	"drazil.de/go64u/util"

	"fmt"
	"log"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func quitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "quit",
		Short: "quit terminal",
		Run: func(cmd *cobra.Command, args []string) {
			log.Println("leave terminal mode")
			os.Exit(0)
		},
	}
}

func TerminalCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "terminal",
		Short:   "Enter terminal (REPL) mode",
		Long:    "Enter terminal (REPL) mode",
		GroupID: "terminal",
		Args:    cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			run()
		},
	}
}

func run() {
	fmt.Println("Welcome to the go64u terminal mode! Type 'quit' to exit.")
	replCmd := &cobra.Command{}
	setup.Setup(replCmd, true)
	replCmd.AddCommand(quitCommand())
	replCmd.AddCommand(commands.RemoteCdCommand())
	replCmd.AddCommand(commands.StreamControllerCommand())
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Printf("%s%s: ", util.White, commands.CurrentPath)
		if !scanner.Scan() {
			break
		}
		commandLine := scanner.Text()
		args := strings.Fields(commandLine)
		replCmd.SetArgs(args)

		if err := replCmd.Execute(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		util.ResetAllFlags(replCmd)
	}
}
