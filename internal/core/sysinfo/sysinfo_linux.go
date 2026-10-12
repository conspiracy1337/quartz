package sysinfo

import (
	"bufio"
	"bytes"
	"os"
	"strconv"
	"strings"
)

func readMemory() (Memory, error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return Memory{}, err
	}
	return parseMeminfo(data), nil
}

// parseMeminfo reads MemTotal and MemAvailable (both in kB).
func parseMeminfo(data []byte) Memory {
	var m Memory
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		key, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		kb, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}
		switch key {
		case "MemTotal":
			m.Total = kb * 1024
		case "MemAvailable":
			m.Available = kb * 1024
		}
	}
	return m
}
