//go:build !linux

package sysinfo

// DedicatedGPU is only needed for Linux's PRIME offload switch.
func DedicatedGPU() string { return "" }
