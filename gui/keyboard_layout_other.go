//go:build !windows

package gui

// detectHostLayout is a stub on non-Windows builds. macOS and Linux paths
// could be added later (TISCopyCurrentKeyboardLayoutInputSource on macOS,
// XkbGetState on X11, the wl_keyboard.keymap on Wayland).
func detectHostLayout() kbLayout { return layoutUS }
