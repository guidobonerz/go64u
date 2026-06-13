//go:build windows

package gui

import "golang.org/x/sys/windows"

var procGetKeyboardLayout = windows.NewLazySystemDLL("user32.dll").NewProc("GetKeyboardLayout")

func detectHostLayout() kbLayout {
	hkl, _, _ := procGetKeyboardLayout.Call(0)
	primary := uint16(hkl&0xFFFF) & 0x3FF
	if primary == 0x07 {
		return layoutDE
	}
	return layoutUS
}
