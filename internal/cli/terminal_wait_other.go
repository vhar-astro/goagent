//go:build !linux && !darwin

package cli

import (
	"context"
	"os"
)

func waitForTerminalInput(ctx context.Context, _ *os.File) error {
	return ctx.Err()
}
