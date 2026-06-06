//go:build linux

package cli

const (
	ioctlReadTermios  = syscallTCGETS
	ioctlWriteTermios = syscallTCSETS
)

const (
	syscallTCGETS = 0x5401
	syscallTCSETS = 0x5402
)
