package sysinfo

// DiskFree is how many bytes the current user can still write on the drive
// that holds path.
func DiskFree(path string) (uint64, error) { return diskFree(path) }
