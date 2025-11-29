package commands

import (
	"de/drazil/go64u/network"
	"log"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

func Load() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "load [file] [address]",
		Short:   "Loads a program into the U64",
		Long:    "With this command a progam can be loaded into memory. The machine resets,\nand loads the attached program into memory using DMA. It does not automatically run the program.",
		GroupID: "runner",
		Args:    cobra.MaximumNArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Flags().BoolP("detectstart", "d", false, "detect start address if address is not given")
			data, _ := readFile(args[0])
			network.Execute("runners:load_prg", http.MethodPost, data)
		},
	}
	cmd.Flags().BoolP("detectstart", "d", false, "detect start address if address is not given")
	return cmd
}

func Run() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "run [file] [address]",
		Short:   "Loads a program into the U64 and automatically starts it",
		Long:    "With this command a progam can be loaded into memory. The machine resets,\nand loads the attached program into memory using DMA. Then it automatically runs the program.",
		GroupID: "runner",
		Args:    cobra.MaximumNArgs(2),
		Run: func(cmd *cobra.Command, args []string) {

			data, _ := readFile(args[0])
			network.Execute("runners:run_prg", http.MethodPost, data)
		},
	}
	cmd.Flags().BoolP("detectstart", "d", false, "detect start address if address is not given")
	return cmd
}

func Crt() *cobra.Command {
	return &cobra.Command{
		Use:     "crt [file]",
		Short:   "Loads a cartridge file into the U64 and automatically starts it",
		Long:    "This command starts a supplied cartridge file. The machine resets,\nwith the attached cartridge active. It does not alter the configuration of the Ultimate.",
		GroupID: "runner",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {

			data, _ := readFile(args[0])
			network.Execute("runners:run_crt", http.MethodPost, data)
		},
	}
}

func readFile(fileName string) ([]byte, error) {
	bytes, err := os.ReadFile(fileName)
	if err != nil {
		log.Fatal(err)
	}
	return bytes, err
}
