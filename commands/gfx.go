package commands

import (
	"log"

	"github.com/spf13/cobra"
)

func ScreenControlCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "screencontrol",
		Short:   "Reads one byte from memory",
		Long:    "Peek reads one byte from memory",
		GroupID: "platform",
		Args:    cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			d11 := Peek(0xd011) & 0xff
			log.Printf("d011 mask:%08b\n", d11)
			if d11&8 == 8 {
				log.Printf("Screen height: 25")
			} else {
				log.Printf("Screen height: 24")
			}
			if d11&16 == 16 {
				log.Printf("Screen: On")
			} else {
				log.Printf("Screen: Off")
			}

			if d11&32 == 32 {
				log.Printf("Screenmode: Bitmap")
			} else {
				log.Printf("Screenmode: Text")
			}
			if d11&64 == 64 {
				log.Printf("ExtendedBackground: On")
			} else {
				log.Printf("ExtendedBackground: Off")
			}
			log.Printf("peeked result:0x%02X", d11)

			d16 := Peek(0xd016) & 0xff
			log.Printf("d016 mask:%08b\n", d16)
			if d16&8 == 8 {
				log.Printf("Columns: 40")
			} else {
				log.Printf("Columns: 38")
			}
			if d16&16 == 16 {
				log.Printf("Multicolor: On")
			} else {
				log.Printf("Multicolor: Off")
			}
			vicbank := int(Peek(0xdd00)) & 3
			log.Printf("dd00 mask:%08b\n", vicbank)
			log.Printf("VIC Bank: %02x", vicbank)
			d18 := int(Peek(0xd018))
			log.Printf("d018 mask:%08b\n", d18)
			charmemindex := (d18 >> 1) & 7
			log.Printf("CharMem: %04x:%04x", (3-vicbank)*0x4000+charmemindex*0x800, (3-vicbank)*0x4000+charmemindex*0x800+0x7ff)
			screenmemindex := (d18 >> 4) & 15
			log.Printf("ScreenMem: %04x:%04x", (3-vicbank)*0x4000+screenmemindex*0x400, (3-vicbank)*0x4000+screenmemindex*0x400+0x3ff)
		},
	}
}
