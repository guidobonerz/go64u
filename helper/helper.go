package helper

import (
	"fmt"
	"image/color"
	"strconv"
)

func GetWordAsString(address string) string {
	return fmt.Sprintf("%04X", GetWord(address))
}

func GetByteAsString(address string) string {
	return fmt.Sprintf("%02X", GetByte(address))
}

func GetWord(address string) int64 {
	value, _ := strconv.ParseInt(address, 16, 64)
	if value < 0 && value > 0xffff {
		panic("value MUST between 0000 and ffff")
	}
	return value
}

func GetByte(byte string) int64 {
	value, _ := strconv.ParseInt(byte, 16, 64)
	if value < 0 && value > 0xffff {
		panic("value MUST between 0000 and ff")
	}
	return value
}

func GetWordFromArray(offset int, data []byte) int {
	return int(data[offset+1])<<8 | int(data[offset])
}

func GetByteFromArray(offset int, data []byte) int {
	return int(data[offset])
}

func GetPalette() color.Palette {
	return color.Palette{
		color.RGBA{0x00, 0x00, 0x00, 255}, // 0: Black
		color.RGBA{0xff, 0xff, 0xff, 255}, // 1: White
		color.RGBA{0xaf, 0x2a, 0x29, 255}, // 2: Red
		color.RGBA{0x62, 0xd8, 0xcc, 255}, // 3: Cyan
		color.RGBA{0xb0, 0x3f, 0xb6, 255}, // 4: Purple
		color.RGBA{0x4a, 0xc6, 0x4a, 255}, // 5: Green
		color.RGBA{0x37, 0x39, 0xc4, 255}, // 6: Blue
		color.RGBA{0xe4, 0xed, 0x4e, 255}, // 7: Yellow
		color.RGBA{0xb6, 0x59, 0x1c, 255}, // 8: Orange
		color.RGBA{0x68, 0x38, 0x08, 255}, // 9: Brown
		color.RGBA{0xea, 0x74, 0x6c, 255}, // A: Pink
		color.RGBA{0x4d, 0x4d, 0x4d, 255}, // B: Dark Grey
		color.RGBA{0x84, 0x84, 0x84, 255}, // C: Grey
		color.RGBA{0xa6, 0xfa, 0x9e, 255}, // D: Light Green
		color.RGBA{0x70, 0x7c, 0xe6, 255}, // E: Light Blue
		color.RGBA{0xb6, 0xb6, 0xb6, 255}, // F: Light Grey
	}
}
