package commands

import (
	"de/drazil/go64u/network"
	"net/http"

	"github.com/spf13/cobra"
)

func Reset() *cobra.Command {
	return &cobra.Command{
		Use:   "reset",
		Short: "Resets the U64",
		Long:  `This command sends a reset to the machine. The current configuration is not changed.`,
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			network.Execute("machine:reset", http.MethodPut, nil)
		},
	}
}

func Reboot() *cobra.Command {
	return &cobra.Command{
		Use:   "reboot",
		Short: "Reboots the U64",
		Long:  `This command restarts the machine. It re-initializes the cartridge configuration and sends a reset to the machine.`,
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			network.Execute("machine:reboot", http.MethodPut, nil)
		},
	}
}

func Pause() *cobra.Command {
	return &cobra.Command{
		Use:   "pause",
		Short: "Pauses the U64",
		Long:  `When issuing this command, the machine is paused by pulling the DMA line low at a safe moment. This stops the CPU. Note that this does not stop any timers.`,
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			network.Execute("machine:pause", http.MethodPut, nil)
		},
	}
}

func Resume() *cobra.Command {
	return &cobra.Command{
		Use:   "resume",
		Short: "Resume the U64 after pause",
		Long:  `With this command, the machine is resumed from the paused state. The DMA line is released and the CPU will continue where it left off.`,
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			network.Execute("machine:resume", http.MethodPut, nil)
		},
	}
}

func PowerOff() *cobra.Command {
	return &cobra.Command{
		Use:   "poweroff",
		Short: "Shuts down the U64",
		Long:  `This U64-only command causes the machine to power off. Note that it is likely that you won't receive a valid response.`,
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			network.Execute("machine:poweroff", http.MethodPut, nil)
		},
	}
}

func ToggleMenu() *cobra.Command {
	return &cobra.Command{
		Use:   "togglemenu",
		Short: "Toggles the on-screen menu",
		Long:  `This command does the same thing as pressing the Menu button on an 1541 Ultimate cartridge, or briefly pressing the Multi Button on the Ultimate 64. The system will either enter or exit the Ultimate menu system depending on it's current state.`,
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			network.Execute("machine:menu_button", http.MethodPut, nil)
		},
	}
}

func Poke() *cobra.Command {
	return &cobra.Command{
		Use:   "poke [address] [value]",
		Short: "Sets one byte in memory",
		Long:  `POKE writes a byte value (00-ff) in a memory address (0-ffff).`,
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			/*
				address, value, err := helper.ParseAddressAndValue(args[0], args[1])
				if err != nil {
					fmt.Println(err)
					os.Exit(1)
				}

				network.Put("machine","writemem")
			*/
		},
	}
}
