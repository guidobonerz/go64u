package commands

import (
	"de/drazil/go64u/network"
	"de/drazil/go64u/util"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/spf13/cobra"
)

var peekLength uint16
var output string
var outputtype string

func ResetCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "reset",
		Short:   "Resets the U64",
		Long:    "This command sends a reset to the machine. The current configuration is not changed.",
		GroupID: "machine",
		Args:    cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			network.Execute("machine:reset", http.MethodPut, nil)
		},
	}
}

func RebootCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "reboot",
		Short:   "Reboots the U64",
		Long:    "This command restarts the machine./nIt re-initializes the cartridge configuration and sends a reset to the machine.",
		GroupID: "machine",
		Args:    cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			network.Execute("machine:reboot", http.MethodPut, nil)
		},
	}
}

func PauseCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "pause",
		Short:   "Pauses the U64",
		Long:    "When issuing this command, the machine is paused by pulling the DMA line low at a safe moment.\nThis stops the CPU. Note that this does not stop any timers.",
		GroupID: "machine",
		Args:    cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			network.Execute("machine:pause", http.MethodPut, nil)
		},
	}
}

func ResumeCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "resume",
		Short:   "Resume the U64 after pause",
		Long:    "With this command, the machine is resumed from the paused state.\nThe DMA line is released and the CPU will continue where it left off.",
		GroupID: "machine",
		Args:    cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			network.Execute("machine:resume", http.MethodPut, nil)
		},
	}
}

func PowerOffCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "poweroff",
		Short:   "Shuts down the U64",
		Long:    "This U64-only command causes the machine to power off.\nNote that it is likely that you won't receive a valid response.",
		GroupID: "machine",
		Args:    cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			network.Execute("machine:poweroff", http.MethodPut, nil)
		},
	}
}

func ToggleMenuCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "togglemenu",
		Short:   "Toggles the on-screen menu",
		Long:    "This command does the same thing as pressing the Menu button on an 1541 Ultimate cartridge,\nnor briefly pressing the Multi Button on the Ultimate 64. The system will either enter or exit the Ultimate menu system depending on it's current state.",
		GroupID: "machine",
		Args:    cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			network.Execute("machine:menu_button", http.MethodPut, nil)
		},
	}
}

func WriteMemoryCommand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "writemem [address] [value]",
		Short:   "Sets one byte in memory",
		Long:    "POKE writes a byte value (00-ff) in a memory address (0-ffff).",
		GroupID: "machine",
		Args:    cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			if len(args[1]) > 2 {
				panic("only one byte allowed")
			}
			network.Execute(fmt.Sprintf("machine:writemem?address=%s", util.GetWordAsString(args[0])), http.MethodPost, []byte{byte(util.GetByte(args[1]))})
		},
	}
	//cmd.Flags().Uint16VarP(&peekLength, "length", "l", 1, "set the length of data to peek, defaults to 1")
	return cmd
}

func ReadMemoryCommand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "readmem [address] [length] --type [file, data,] --output",
		Short:   "Reads serveral bytes fro memory by a given length",
		Long:    "Reads serveral bytes fro memory by a given length",
		GroupID: "machine",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			//var response []byte = network.Execute(fmt.Sprintf("machine:readmem?address=%s&length=1", helper.GetWordAsString(args[0])), http.MethodGet, nil)
			b := ReadFromMemory(int(util.GetWord(args[0])), peekLength)
			log.Printf("peeked result:0x%02X", b)
		},
	}
	cmd.Flags().Uint16VarP(&peekLength, "length", "l", 1, "set the length of data to peek, defaults to 1")
	cmd.Flags().StringVarP(&output, "output", "o", "output.bin", "set the output file name")
	cmd.Flags().StringVarP(&outputtype, "type", "t", "file", "set the output type [file, bin]")
	return cmd
}

func DumpPageCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "page [number] [file]",
		Short:   "dump a page to file",
		Long:    "dump a page to file",
		GroupID: "machine",
		Args:    cobra.MaximumNArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			//var response []byte = network.Execute(fmt.Sprintf("machine:readmem?address=%s&length=1", helper.GetWordAsString(args[0])), http.MethodGet, nil)
			log.Println("Not yet implemented!!")
		},
	}
}

func MessageCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "message [message] [x] [y]",
		Short:   "Writes a message on screen",
		Long:    "Writes a message on screen at given position.",
		GroupID: "machine",
		Args:    cobra.ExactArgs(3),
		Run: func(cmd *cobra.Command, args []string) {
			y, _ := strconv.Atoi(args[2])
			x, _ := strconv.Atoi(args[1])
			location := 0x400 + y*40 + x
			message := []byte(args[0])
			buffer := make([]byte, len(message))
			for i, v := range message {
				buffer[i] = util.ASCIIToScreenCodeLowercase[v]
			}
			network.Execute(fmt.Sprintf("machine:writemem?address=%04x", location), http.MethodPost, buffer)
		},
	}
}

func ReadFromMemory(address int, length uint16) byte {
	return network.Execute(fmt.Sprintf("machine:readmem?address=%04x&length=%d", address, length), http.MethodGet, nil)[0]
}
