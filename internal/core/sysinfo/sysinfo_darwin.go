package sysinfo

import "golang.org/x/sys/unix"

func readMemory() (Memory, error) {
	total, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return Memory{}, err
	}
	// kern.memorystatus_vm_pressure_level: 1 normal, 2 warning, 4 critical.
	level, _ := unix.SysctlUint32("kern.memorystatus_vm_pressure_level")
	return Memory{Total: total, Pressure: level >= 2}, nil
}

func osVersion() string {
	v, err := unix.Sysctl("kern.osproductversion")
	if err != nil {
		return ""
	}
	return v
}
