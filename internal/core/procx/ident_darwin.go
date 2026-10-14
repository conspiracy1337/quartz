package procx

import "golang.org/x/sys/unix"

// Ident is a running process's start time, in microseconds: it tells a
// process apart from a later one given the same pid.
func Ident(pid int) (uint64, bool) {
	if pid <= 0 {
		return 0, false
	}
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || kp.Proc.P_pid != int32(pid) { //nolint:gosec // pids fit in 32 bits
		return 0, false
	}
	t := kp.Proc.P_starttime
	v := uint64(t.Sec)*1_000_000 + uint64(t.Usec) //nolint:gosec // a time after 1970
	return v, v > 0
}
