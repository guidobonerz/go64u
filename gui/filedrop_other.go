//go:build !windows

package gui

func enableFileDrop(windowTitle string, onDrop func(clientX, clientY int, data []byte)) {}
