//go:build windows

package gui

import (
	"os"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32  = syscall.NewLazyDLL("user32.dll")
	shell32 = syscall.NewLazyDLL("shell32.dll")

	procFindWindowW       = user32.NewProc("FindWindowW")
	procSetWindowLongPtrW = user32.NewProc("SetWindowLongPtrW")
	procCallWindowProcW   = user32.NewProc("CallWindowProcW")
	procDragAcceptFiles   = shell32.NewProc("DragAcceptFiles")
	procDragQueryFileW    = shell32.NewProc("DragQueryFileW")
	procDragQueryPoint    = shell32.NewProc("DragQueryPoint")
	procDragFinish        = shell32.NewProc("DragFinish")
)

const (
	wmDropFiles = 0x0233
)

var gwlpWndProcIndex = -4

func gwlpWndProc() uintptr {
	return uintptr(gwlpWndProcIndex)
}

type pointT struct {
	X, Y int32
}

var (
	origWndProc uintptr
	dropHandler func(clientX, clientY int, data []byte)
)

func dropWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	if msg == wmDropFiles {
		hDrop := wParam
		var pt pointT
		procDragQueryPoint.Call(hDrop, uintptr(unsafe.Pointer(&pt)))
		count, _, _ := procDragQueryFileW.Call(hDrop, 0xFFFFFFFF, 0, 0)
		if count > 0 {
			buf := make([]uint16, 1024)
			procDragQueryFileW.Call(hDrop, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
			path := syscall.UTF16ToString(buf)
			x, y := int(pt.X), int(pt.Y)

			go func(p string, cx, cy int) {
				data, err := os.ReadFile(p)
				if err != nil {
					return
				}
				if dropHandler != nil {
					dropHandler(cx, cy, data)
				}
			}(path, x, y)
		}
		procDragFinish.Call(hDrop)
		return 0
	}
	ret, _, _ := procCallWindowProcW.Call(origWndProc, hwnd, msg, wParam, lParam)
	return ret
}

func enableFileDrop(windowTitle string, onDrop func(clientX, clientY int, data []byte)) {
	dropHandler = onDrop
	go func() {
		titlePtr, err := syscall.UTF16PtrFromString(windowTitle)
		if err != nil {
			return
		}
		var hwnd uintptr
		for range 50 {
			hwnd, _, _ = procFindWindowW.Call(0, uintptr(unsafe.Pointer(titlePtr)))
			if hwnd != 0 {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if hwnd == 0 {
			return
		}
		procDragAcceptFiles.Call(hwnd, 1)
		cb := syscall.NewCallback(dropWndProc)
		orig, _, _ := procSetWindowLongPtrW.Call(hwnd, gwlpWndProc(), cb)
		origWndProc = orig
	}()
}
