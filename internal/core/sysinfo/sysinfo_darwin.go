package sysinfo

import "golang.org/x/sys/unix"

func readMemory() (Memory, error) {
	total, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return Memory{}, err
	}
	return Memory{Total: total}, nil
}
