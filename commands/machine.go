package commands

import (
	"de/drazil/go64u/helper"
	"de/drazil/go64u/network"
	"fmt"
	"log"
	"net/http"

	"github.com/spf13/cobra"
)

func Reset() *cobra.Command {
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

func Reboot() *cobra.Command {
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

func Pause() *cobra.Command {
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

func Resume() *cobra.Command {
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

func PowerOff() *cobra.Command {
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

func ToggleMenu() *cobra.Command {
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

func Poke() *cobra.Command {
	return &cobra.Command{
		Use:     "poke [address] [value]",
		Short:   "Sets one byte in memory",
		Long:    "POKE writes a byte value (00-ff) in a memory address (0-ffff).",
		GroupID: "machine",
		Args:    cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			if len(args[1]) > 2 {
				panic("only one byte allowed")
			}
			network.Execute(fmt.Sprintf("machine:writemem?address=%s", helper.GetWordAsString(args[0])), http.MethodPost, []byte{byte(helper.GetByte(args[1]))})
		},
	}
}

func Peek() *cobra.Command {
	return &cobra.Command{
		Use:     "peek [address]",
		Short:   "Reads one byte from memory",
		Long:    "Peek reads one byte from memory",
		GroupID: "machine",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			var response []byte = network.Execute(fmt.Sprintf("machine:readmem?address=%s&length=1", helper.GetWordAsString(args[0])), http.MethodGet, nil)
			log.Printf("peeked result:0x%02X", response[0])
		},
	}
}

func DumpPage() *cobra.Command {
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

func PrintAt() *cobra.Command {
	return &cobra.Command{
		Use:     "printat [message] [x] [y]",
		Short:   "Writes a message on screen",
		Long:    "Writes a message on screen at given position.",
		GroupID: "machine",
		Args:    cobra.ExactArgs(3),
		Run: func(cmd *cobra.Command, args []string) {
			log.Println("Not yet implemented!!")
		},
	}
}
