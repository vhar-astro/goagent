//go:build darwin

package cli

const (
	ioctlReadTermios  = syscallTIOCGETA
	ioctlWriteTermios = syscallTIOCSETA
)

const (
	syscallTIOCGETA = 0x40487413
	syscallTIOCSETA = 0x80487414
)
