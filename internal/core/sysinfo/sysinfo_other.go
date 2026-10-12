//go:build !linux && !darwin && !windows

package sysinfo

func readMemory() (Memory, error) { return Memory{}, nil }

func osVersion() string { return "" }
