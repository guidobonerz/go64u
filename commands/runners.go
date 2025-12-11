package commands

import (
	"de/drazil/go64u/helper"
	"de/drazil/go64u/network"
	"net/http"

	"github.com/spf13/cobra"
)

func LoadCommand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "load [file] [address]",
		Short:   "Loads a program into the U64",
		Long:    "With this command a progam can be loaded into memory. The machine resets,\nand loads the attached program into memory using DMA. It does not automatically run the program.",
		GroupID: "runner",
		Args:    cobra.MaximumNArgs(2),
		Run: func(cmd *cobra.Command, args []string) {

			data, _ := helper.ReadFile(args[0])
			network.Execute("runners:load_prg", http.MethodPost, data)
		},
	}
	cmd.Flags().BoolP("detectstart", "d", false, "detect start address if address is not given")
	return cmd
}

func RunCommand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "run [file] [address]",
		Short:   "Loads a program into the U64 and automatically starts it",
		Long:    "With this command a progam can be loaded into memory. The machine resets,\nand loads the attached program into memory using DMA. Then it automatically runs the program.",
		GroupID: "runner",
		Args:    cobra.MaximumNArgs(2),
		Run: func(cmd *cobra.Command, args []string) {

			data, _ := helper.ReadFile(args[0])
			network.Execute("runners:run_prg", http.MethodPost, data)
		},
	}
	cmd.Flags().BoolP("detectstart", "d", false, "detect start address if address is not given")
	return cmd
}

func CrtCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "crt [file]",
		Short:   "Loads a cartridge file into the U64 and automatically starts it",
		Long:    "This command starts a supplied cartridge file. The machine resets,\nwith the attached cartridge active. It does not alter the configuration of the Ultimate.",
		GroupID: "runner",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {

			data, _ := helper.ReadFile(args[0])
			network.Execute("runners:run_crt", http.MethodPost, data)
		},
	}
}
