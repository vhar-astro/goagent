//go:build linux || darwin

package cli

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

type ttyState struct {
	termios syscall.Termios
}

func makeRawTTY(file *os.File) (*ttyState, error) {
	fd := file.Fd()
	termios, err := ioctlGetTermios(fd)
	if err != nil {
		return nil, err
	}

	original := termios
	termios.Lflag &^= syscall.ICANON | syscall.ECHO
	termios.Cc[syscall.VMIN] = 1
	termios.Cc[syscall.VTIME] = 0

	if err := ioctlSetTermios(fd, termios); err != nil {
		return nil, err
	}

	return &ttyState{termios: original}, nil
}

func restoreTTY(file *os.File, state *ttyState) error {
	if file == nil || state == nil {
		return nil
	}

	return ioctlSetTermios(file.Fd(), state.termios)
}

func ioctlGetTermios(fd uintptr) (syscall.Termios, error) {
	termios := syscall.Termios{}
	if _, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, fd, ioctlReadTermios, uintptr(unsafe.Pointer(&termios)), 0, 0, 0); errno != 0 {
		return syscall.Termios{}, fmt.Errorf("read termios: %w", errno)
	}

	return termios, nil
}

func ioctlSetTermios(fd uintptr, termios syscall.Termios) error {
	if _, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, fd, ioctlWriteTermios, uintptr(unsafe.Pointer(&termios)), 0, 0, 0); errno != 0 {
		return fmt.Errorf("write termios: %w", errno)
	}

	return nil
}
