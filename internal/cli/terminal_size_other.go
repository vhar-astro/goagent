//go:build !linux && !darwin

package cli

import "os"

func currentTerminalSize(_ *os.File) (terminalSize, bool) {
	return terminalSize{}, false
}
