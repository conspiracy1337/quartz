package procx

import "golang.org/x/sys/windows"

// Ident is a running process's creation time: it tells a process apart from
// a later one given the same pid.
func Ident(pid int) (uint64, bool) {
	if pid <= 0 {
		return 0, false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid)) //nolint:gosec // Windows process ids are 32-bit
	if err != nil {
		return 0, false
	}
	defer func() { _ = windows.CloseHandle(h) }()
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return 0, false
	}
	v := uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime)
	return v, v > 0
}
