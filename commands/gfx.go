package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

const SCREEN_HEIGHT = 0x08
const SCREEN_WIDTH = 0x08
const SCREEN_ENABLED = 0x10
const MULTICOLOR = 0x10
const SCREEN_MODE = 0x20
const EXTENDED_BACKGROUND = 0x40

var height = [...]string{"24", "25"}
var width = [...]string{"38", "40"}
var onoff = [...]string{"off", "on"}
var screenmode = [...]string{"text", "bitmap"}
var shift = [...]int{1, 3}
var mask = [...]int{7, 1}
var offset = [...]int{0x800, 0x2000}

func ScreenControlCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "screencontrol",
		Short:   "Reads one byte from memory",
		Long:    "Peek reads one byte from memory",
		GroupID: "platform",
		Args:    cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			d11 := ReadFromMemory(0xd011, 1) & 0xff
			d16 := ReadFromMemory(0xd016, 1) & 0xff
			d18 := int(ReadFromMemory(0xd018, 1) & 0xff)
			vicbank := int(ReadFromMemory(0xdd00, 1)&0xff) & 3
			screenModeFlag := (d11 >> 5) & 1
			fmt.Print("\n** Screen Mode and Memory Information **\n\n")
			fmt.Printf("d011 bitmask:%08b/%02x\n", d11, d11)
			fmt.Printf("d016 bitmask:%08b/%02x\n", d16, d16)
			fmt.Printf("d018 bitmask:%08b/%02x\n", d18, d18)
			fmt.Printf("dd00 bitmask:%08b/%02x\n", vicbank, vicbank)

			fmt.Printf("Screen            : %s\n", onoff[(d11>>4)&1])
			fmt.Printf("Screen height     : %s\n", height[(d11>>3)&1])
			fmt.Printf("Screen width      : %s\n", width[(d11>>3)&1])
			fmt.Printf("Screenmode        : %s\n", screenmode[screenModeFlag])
			fmt.Printf("ExtendedBackground: %s\n", onoff[(d11>>6)&1])
			fmt.Printf("Multicolor        : %s\n", onoff[(d16>>4)&1])

			gfxMemIndex := (d18 >> shift[screenModeFlag]) & mask[screenModeFlag]
			charMemFrom := (3-vicbank)*0x4000 + gfxMemIndex*offset[screenModeFlag]
			fmt.Printf("CharMem           : %04x:%04x\n", charMemFrom, charMemFrom+offset[screenModeFlag]-1)
			screenmemindex := (d18 >> 4) & 15
			screenMemFrom := (3-vicbank)*0x4000 + screenmemindex*0x400
			fmt.Printf("ScreenMem         : %04x:%04x\n", screenMemFrom, screenMemFrom+0x3ff)
			fmt.Printf("Spritepointer     : %04x:%04x\n", screenMemFrom+0x3f8, screenMemFrom+0x3ff)
			s := 0
			for i := 0x3f8; i < 0x400; i++ {
				sp := int((ReadFromMemory(screenMemFrom+i, 1) & 0xff) * 0x40)
				fmt.Printf("Sprite %d          : %04x\n", s, screenMemFrom+sp)
				s++
			}

		},
	}
}
