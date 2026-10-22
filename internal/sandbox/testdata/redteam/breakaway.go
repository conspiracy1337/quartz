//go:build windows

// breakaway.exe - red-team helper (round 3). Attempts CreateProcess with
// CREATE_BREAKAWAY_FROM_JOB to test whether a mod can spawn a child that leaves
// the launcher's kill-on-close job object. If the job forbids breakaway (it
// omits JOB_OBJECT_LIMIT_BREAKAWAY_OK) the OS denies it; we then retry without
// the flag to show the baseline child still starts (and stays in the job). The
// child is a timer loop appending to a canary in the granted game folder so the
// harness can confirm whether it outlived the jail. Built on demand by the
// harness and launched by WinProbe2 inside the AppContainer; not shipped.
package main

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

func main() {
	file := `C:\quartz-rt\unused`
	if len(os.Args) > 1 {
		file = os.Args[1]
	}
	ps := os.Getenv("SYSTEMROOT") + `\System32\WindowsPowerShell\v1.0\powershell.exe`
	script := "1..60 | ForEach-Object { Add-Content -LiteralPath '" + file + "' -Value 'alive'; Start-Sleep -Milliseconds 400 }"
	cmd := `"` + ps + `" -NoProfile -WindowStyle Hidden -Command "` + script + `"`

	const createBreakawayFromJob = 0x01000000
	const createNoWindow = 0x08000000

	spawn := func(flags uint32) (uint32, error) {
		argv, err := syscall.UTF16PtrFromString(cmd)
		if err != nil {
			return 0, err
		}
		var si syscall.StartupInfo
		si.Cb = uint32(unsafe.Sizeof(si))
		var pi syscall.ProcessInformation
		if err := syscall.CreateProcess(nil, argv, nil, nil, false, flags, nil, nil, &si, &pi); err != nil {
			return 0, err
		}
		_ = syscall.CloseHandle(pi.Thread)
		_ = syscall.CloseHandle(pi.Process)
		return pi.ProcessId, nil
	}

	if pid, err := spawn(createBreakawayFromJob | createNoWindow); err != nil {
		fmt.Printf("breakaway=DENIED err=%v ", err)
		if pid2, err2 := spawn(createNoWindow); err2 != nil {
			fmt.Printf("baseline-child=DENIED err=%v\n", err2)
		} else {
			fmt.Printf("baseline-child=OK pid=%d-in-job\n", pid2)
		}
		return
	} else {
		fmt.Printf("breakaway=OK pid=%d-left-job\n", pid)
	}
}
