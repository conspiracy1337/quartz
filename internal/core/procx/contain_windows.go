package procx

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// procThreadAttributeSecurityCapabilities is PROC_THREAD_ATTRIBUTE_SECURITY_CAPABILITIES.
const procThreadAttributeSecurityCapabilities = 0x00020009

// securityCapabilities is SECURITY_CAPABILITIES.
type securityCapabilities struct {
	AppContainerSid *windows.SID
	Capabilities    *windows.SIDAndAttributes
	CapabilityCount uint32
	Reserved        uint32
}

// startContained starts the child inside an AppContainer. exec.Cmd can't
// pass the security-capabilities attribute, so this calls CreateProcess
// itself, inheriting only the three pipe ends the child needs.
func startContained(s Spec) (*Child, error) {
	if len(s.ExtraFiles) > 0 {
		return nil, errors.New("procx: extra files can't be passed on Windows")
	}
	app, err := filepath.Abs(s.Argv[0])
	if err != nil {
		return nil, err
	}
	sid, err := windows.StringToSid(s.Contain.SID)
	if err != nil {
		return nil, err
	}
	caps := make([]windows.SIDAndAttributes, len(s.Contain.Capabilities))
	for i, c := range s.Contain.Capabilities {
		if caps[i].Sid, err = windows.StringToSid(c); err != nil {
			return nil, err
		}
		caps[i].Attributes = windows.SE_GROUP_ENABLED
	}
	sc := securityCapabilities{AppContainerSid: sid, CapabilityCount: uint32(len(caps))} //nolint:gosec // a handful of capabilities
	if len(caps) > 0 {
		sc.Capabilities = &caps[0]
	}

	var parent, child []*os.File
	closeAll := func(fs []*os.File) {
		for _, f := range fs {
			_ = f.Close()
		}
	}
	for range 3 {
		r, w, err := os.Pipe()
		if err != nil {
			closeAll(parent)
			closeAll(child)
			return nil, err
		}
		if len(child) == 0 { // stdin: the child reads
			parent, child = append(parent, w), append(child, r)
		} else {
			parent, child = append(parent, r), append(child, w)
		}
	}
	defer closeAll(child)
	fail := func(err error) (*Child, error) {
		closeAll(parent)
		return nil, err
	}
	handles := make([]windows.Handle, len(child))
	for i, f := range child {
		handles[i] = windows.Handle(f.Fd())
		if err := windows.SetHandleInformation(handles[i], windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT); err != nil {
			return fail(err)
		}
	}

	attrs, err := windows.NewProcThreadAttributeList(2)
	if err != nil {
		return fail(err)
	}
	defer attrs.Delete()
	if err := attrs.Update(procThreadAttributeSecurityCapabilities, unsafe.Pointer(&sc), unsafe.Sizeof(sc)); err != nil {
		return fail(err)
	}
	if err := attrs.Update(windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&handles[0]), uintptr(len(handles))*unsafe.Sizeof(handles[0])); err != nil {
		return fail(err)
	}
	si := windows.StartupInfoEx{
		StartupInfo: windows.StartupInfo{
			Flags:     windows.STARTF_USESTDHANDLES,
			StdInput:  handles[0],
			StdOutput: handles[1],
			StdErr:    handles[2],
		},
		ProcThreadAttributeList: attrs.List(),
	}
	si.Cb = uint32(unsafe.Sizeof(si))

	appName, err := windows.UTF16PtrFromString(app)
	if err != nil {
		return fail(err)
	}
	cmdLine, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(s.Argv))
	if err != nil {
		return fail(err)
	}
	var dir *uint16
	if s.Dir != "" {
		if dir, err = windows.UTF16PtrFromString(s.Dir); err != nil {
			return fail(err)
		}
	}
	env, err := envBlock(s.Env)
	if err != nil {
		return fail(err)
	}
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_UNICODE_ENVIRONMENT | createNewProcessGroup)
	if s.NoConsole {
		flags |= createNoWindow
	}
	var pi windows.ProcessInformation
	err = windows.CreateProcess(appName, cmdLine, nil, nil, true, flags, &env[0], dir, &si.StartupInfo, &pi)
	runtime.KeepAlive(sc)
	runtime.KeepAlive(caps)
	runtime.KeepAlive(handles)
	if err != nil {
		return fail(err)
	}
	_ = windows.CloseHandle(pi.Thread)
	// Our handle keeps the process object, so its id can't be reused before
	// os.FindProcess opens its own.
	p, err := os.FindProcess(int(pi.ProcessId))
	_ = windows.CloseHandle(pi.Process)
	if err != nil {
		return fail(err)
	}
	return &Child{Process: p, Stdin: parent[0], Stdout: parent[1], Stderr: parent[2], wait: func() (*os.ProcessState, error) {
		st, err := p.Wait()
		closeAll(parent[1:])
		return st, err
	}}, nil
}

// envBlock is a CreateProcess environment: NUL-separated, double-NUL ended.
func envBlock(env []string) ([]uint16, error) {
	if len(env) == 0 {
		return []uint16{0, 0}, nil
	}
	for _, e := range env {
		if strings.ContainsRune(e, 0) {
			return nil, errors.New("procx: environment entry holds a NUL")
		}
	}
	return utf16.Encode([]rune(strings.Join(env, "\x00") + "\x00\x00")), nil
}
