//go:build !linux && !darwin

package cli

import "os"

func newResizeNotifier() (<-chan os.Signal, func()) {
	return nil, func() {}
}
