package commands

import (
	"de/drazil/go64u/network"
	"log"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

func Load() *cobra.Command {
	return &cobra.Command{
		Use:   "load [file]",
		Short: "Loads a program into the U64",
		Long:  `With this command a progam can be loaded into memory. The machine resets, and loads the attached program into memory using DMA. It does not automatically run the program.`,
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			data, _ := readFile(args[0])
			network.Execute("runners:load_prg", http.MethodPost, data)
		},
	}
}

func Run() *cobra.Command {
	return &cobra.Command{
		Use:   "run [file]",
		Short: "Loads a program into the U64 and automatically starts it",
		Long:  `With this command a progam can be loaded into memory. The machine resets, and loads the attached program into memory using DMA. Then it automatically runs the program.`,
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			data, _ := readFile(args[0])
			network.Execute("runners:run_prg", http.MethodPost, data)
		},
	}
}

func Crt() *cobra.Command {
	return &cobra.Command{
		Use:   "crt [file]",
		Short: "Loads a cartridge file into the U64 and automatically starts it",
		Long:  `This command starts a supplied cartridge file. The machine resets, with the attached cartridge active. It does not alter the configuration of the Ultimate.`,
		Args:  cobra.ExactArgs(1),
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
