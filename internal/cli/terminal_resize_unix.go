//go:build linux || darwin

package cli

import (
	"os"
	"os/signal"
	"syscall"
)

func newResizeNotifier() (<-chan os.Signal, func()) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGWINCH)
	return signals, func() { signal.Stop(signals) }
}
