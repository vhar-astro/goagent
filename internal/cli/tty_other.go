//go:build !linux && !darwin

package cli

import (
	"os"
)

type ttyState struct{}

func makeRawTTY(_ *os.File) (*ttyState, error) {
	return nil, nil
}

func restoreTTY(_ *os.File, _ *ttyState) error {
	return nil
}
