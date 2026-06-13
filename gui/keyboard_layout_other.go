//go:build !windows

package gui

func detectHostLayout() kbLayout { return layoutUS }
