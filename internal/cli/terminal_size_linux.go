//go:build linux

package cli

import (
	"os"
	"syscall"
	"unsafe"
)

const ioctlGetWindowSize = 0x5413 // TIOCGWINSZ

func currentTerminalSize(file *os.File) (terminalSize, bool) {
	if file == nil {
		return terminalSize{}, false
	}

	var size struct {
		Rows uint16
		Cols uint16
		X    uint16
		Y    uint16
	}
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), ioctlGetWindowSize, uintptr(unsafe.Pointer(&size))); errno != 0 {
		return terminalSize{}, false
	}
	if size.Cols == 0 || size.Rows == 0 {
		return terminalSize{}, false
	}

	return terminalSize{Width: int(size.Cols), Height: int(size.Rows)}, true
}
