//go:build linux && !amd64 && !arm64

package seccomp

const (
	auditArch      = 0
	x32Bit         = 0
	cloneFlagsArg  = 0
	sysClone       = 0
	sysIoctl       = 0
	sysSocket      = 0
	sysPersonality = 0
)

var denied, nosys []uint32
