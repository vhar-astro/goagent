package cli

import "os"

// terminalSize is the usable character geometry reported by a terminal.
type terminalSize struct {
	Width  int
	Height int
}

// terminalSizeProvider is injectable so footer behavior can be tested without
// an attached terminal.
type terminalSizeProvider func(*os.File) (terminalSize, bool)
