// Package sysinfo reports machine facts the launcher needs for memory
// suggestions and warnings, without cgo.
package sysinfo

// Memory is the machine's RAM in bytes. Available is 0 when the OS does not
// expose a trustworthy figure (macOS compresses memory, so "free" understates it).
type Memory struct {
	Total     uint64 `json:"total"`
	Available uint64 `json:"available"`
}

// ReadMemory returns the current memory figures.
func ReadMemory() (Memory, error) { return readMemory() }
