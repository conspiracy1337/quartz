// Package sysinfo reports machine facts the launcher needs for memory
// suggestions and warnings, without cgo.
package sysinfo

// Memory is the machine's RAM in bytes. Available is 0 when the OS does not
// expose a trustworthy figure (macOS compresses memory, so "free" understates it).
type Memory struct {
	Total     uint64 `json:"total"`
	Available uint64 `json:"available"`
	// Pressure is set where the system reports memory pressure instead of a
	// meaningful free figure (macOS, which keeps "free" memory near zero).
	Pressure bool `json:"-"`
}

// ReadMemory returns the current memory figures.
func ReadMemory() (Memory, error) { return readMemory() }

// OSVersion is what Java reports as os.version: the kernel release on Linux,
// the product version on macOS, major.minor.build on Windows. Mojang's rules
// match against it. Empty when unknown.
func OSVersion() string { return osVersion() }
