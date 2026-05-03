//go:build windows

package gui

import "golang.org/x/sys/windows"

var procGetKeyboardLayout = windows.NewLazySystemDLL("user32.dll").NewProc("GetKeyboardLayout")

// detectHostLayout asks Windows for the foreground thread's active keyboard
// layout (HKL). The low word is the LANGID; its primary language identifier
// (low 10 bits) is 0x07 for German and 0x09 for English. We currently
// recognise only those two and fall back to layoutUS otherwise.
func detectHostLayout() kbLayout {
	hkl, _, _ := procGetKeyboardLayout.Call(0)
	primary := uint16(hkl&0xFFFF) & 0x3FF
	if primary == 0x07 {
		return layoutDE
	}
	return layoutUS
}
