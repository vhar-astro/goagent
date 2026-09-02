//go:build darwin

package cli

import (
	"context"
	"fmt"
	"os"
	"syscall"
)

func waitForTerminalInput(ctx context.Context, file *os.File) error {
	fd := int(file.Fd())
	if fd < 0 || fd >= len(syscall.FdSet{}.Bits)*32 {
		return fmt.Errorf("terminal file descriptor %d exceeds FD_SETSIZE", fd)
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var set syscall.FdSet
		set.Bits[fd/32] |= int32(1) << uint(fd%32)
		timeout := syscall.Timeval{Usec: 100_000}
		err := syscall.Select(fd+1, &set, nil, nil, &timeout)
		if err == syscall.EINTR {
			continue
		}
		if err != nil {
			return err
		}
		if set.Bits[fd/32]&(int32(1)<<uint(fd%32)) != 0 {
			return nil
		}
	}
}
