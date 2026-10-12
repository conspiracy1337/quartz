package sysinfo

import "testing"

func TestParseMeminfo(t *testing.T) {
	m := parseMeminfo([]byte("MemTotal:       32768000 kB\nMemFree:  100 kB\nMemAvailable:   16384000 kB\nbogus\n"))
	if m.Total != 32768000*1024 || m.Available != 16384000*1024 {
		t.Fatalf("got %+v", m)
	}
}

func TestReadMemory(t *testing.T) {
	m, err := ReadMemory()
	if err != nil || m.Total == 0 {
		t.Fatalf("ReadMemory = %+v, %v", m, err)
	}
}
