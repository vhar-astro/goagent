//go:build linux

package cli

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

func waitForTerminalInput(ctx context.Context, file *os.File) error {
	fd := int(file.Fd())
	if fd < 0 {
		return fmt.Errorf("invalid terminal file descriptor %d", fd)
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var set syscall.FdSet
		bytes := unsafe.Slice((*byte)(unsafe.Pointer(&set)), int(unsafe.Sizeof(set)))
		if fd >= len(bytes)*8 {
			return fmt.Errorf("terminal file descriptor %d exceeds FD_SETSIZE", fd)
		}
		bytes[fd/8] |= byte(1) << uint(fd%8)
		timeout := syscall.Timeval{Usec: 100_000}
		ready, err := syscall.Select(fd+1, &set, nil, nil, &timeout)
		if err == syscall.EINTR {
			continue
		}
		if err != nil {
			return err
		}
		if ready > 0 && bytes[fd/8]&(byte(1)<<uint(fd%8)) != 0 {
			return nil
		}
	}
}
