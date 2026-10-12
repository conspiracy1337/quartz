package sysinfo

import "golang.org/x/sys/unix"

func readMemory() (Memory, error) {
	total, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return Memory{}, err
	}
	return Memory{Total: total}, nil
}

func osVersion() string {
	v, err := unix.Sysctl("kern.osproductversion")
	if err != nil {
		return ""
	}
	return v
}
