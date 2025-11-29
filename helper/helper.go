package helper

import (
	"fmt"
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
