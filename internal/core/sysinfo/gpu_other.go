//go:build !linux && !windows

package sysinfo

// DedicatedGPU is only named where "Use the dedicated GPU" picks one.
func DedicatedGPU() string { return "" }

// OffloadEnv is Linux's PRIME offload.
func OffloadEnv() []string { return nil }

// PreferGPU is Windows' per-program GPU preference.
func PreferGPU(string) error { return nil }
