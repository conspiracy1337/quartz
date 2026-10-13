//go:build linux

// Package seccomp builds the system-call filter a sandboxed game runs under,
// as a classic BPF program bubblewrap loads (--seccomp FD). It is generated
// in Go (golang.org/x/net/bpf), with no libseccomp and no cgo. The rules are
// Flatpak's proven blocklist: a game needs almost every ordinary call, and
// what it never needs (namespaces, mounts, tracing, kernel keyrings and
// modules, terminal injection) is refused.
package seccomp

import (
	"encoding/binary"
	"errors"
	"os"

	"golang.org/x/net/bpf"
	"golang.org/x/sys/unix"
)

// Return values.
const (
	retAllow = 0x7fff0000 // SECCOMP_RET_ALLOW
	retErrno = 0x00050000 // SECCOMP_RET_ERRNO | errno
	retKill  = 0x80000000 // SECCOMP_RET_KILL_PROCESS
)

// seccomp_data offsets: nr, arch, then six 64-bit arguments (each low word
// first on these little-endian architectures).
const (
	offNr   = 0
	offArch = 4
	offArgs = 16
)

func errno(e unix.Errno) uint32 { return retErrno | uint32(e) } //nolint:gosec // errno values are small

func arg(i int) uint32 { return uint32(offArgs + 8*i) } //nolint:gosec // i < 6

// cloneNew is every namespace flag clone(2) can take.
const cloneNew = unix.CLONE_NEWNS | unix.CLONE_NEWCGROUP | unix.CLONE_NEWUTS | unix.CLONE_NEWIPC |
	unix.CLONE_NEWUSER | unix.CLONE_NEWPID | unix.CLONE_NEWNET | unix.CLONE_NEWTIME

// socketFamilies may be created; others get EAFNOSUPPORT.
var socketFamilies = []uint32{unix.AF_UNSPEC, unix.AF_UNIX, unix.AF_INET, unix.AF_INET6, unix.AF_NETLINK}

// personalities may be set: the default, its query and the harmless flags
// some runtimes set (as Pandora allows).
var personalities = []uint32{0, 0x8, 0x20000, 0x20008, 0xffffffff}

// ErrArch is returned on an architecture without a syscall table here.
var ErrArch = errors.New("seccomp: no filter for this architecture")

// Instructions is the filter program.
func Instructions() ([]bpf.Instruction, error) {
	if auditArch == 0 {
		return nil, ErrArch
	}
	p := []bpf.Instruction{
		bpf.LoadAbsolute{Off: offArch, Size: 4},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: auditArch, SkipTrue: 1},
		bpf.RetConstant{Val: retKill},
		bpf.LoadAbsolute{Off: offNr, Size: 4},
	}
	if x32Bit != 0 {
		// x32 calls carry this bit; a 64-bit game never makes them.
		p = append(p, bpf.JumpIf{Cond: bpf.JumpBitsSet, Val: x32Bit, SkipFalse: 1}, bpf.RetConstant{Val: errno(unix.EPERM)})
	}
	refuse := func(nrs []uint32, ret uint32) {
		for _, nr := range nrs {
			p = append(p, bpf.JumpIf{Cond: bpf.JumpNotEqual, Val: nr, SkipTrue: 1}, bpf.RetConstant{Val: ret})
		}
	}
	refuse(denied, errno(unix.EPERM))
	// ENOSYS makes glibc fall back: clone3 to clone (whose flags are checked
	// below), the new mount API to nothing.
	refuse(nosys, errno(unix.ENOSYS))

	// One argument checked against a list; every block ends in a return, so
	// the next block starts by reloading nr. A match jumps over the rest of
	// the list and the first return to the second: allow for an allowlist,
	// deny for a denylist.
	oneOf := func(nr uint32, a int, vals []uint32, allowList bool, deny uint32) {
		block := []bpf.Instruction{bpf.LoadAbsolute{Off: arg(a), Size: 4}}
		for i, v := range vals {
			block = append(block, bpf.JumpIf{Cond: bpf.JumpEqual, Val: v, SkipTrue: uint8(len(vals) - i)}) //nolint:gosec // short lists
		}
		if allowList {
			block = append(block, bpf.RetConstant{Val: deny}, bpf.RetConstant{Val: retAllow})
		} else {
			block = append(block, bpf.RetConstant{Val: retAllow}, bpf.RetConstant{Val: deny})
		}
		p = append(p, bpf.LoadAbsolute{Off: offNr, Size: 4}, bpf.JumpIf{Cond: bpf.JumpNotEqual, Val: nr, SkipTrue: uint8(len(block))}) //nolint:gosec // short blocks
		p = append(p, block...)
	}
	// clone with any namespace flag.
	p = append(p, bpf.LoadAbsolute{Off: offNr, Size: 4}, bpf.JumpIf{Cond: bpf.JumpNotEqual, Val: sysClone, SkipTrue: 4},
		bpf.LoadAbsolute{Off: arg(cloneFlagsArg), Size: 4},
		bpf.JumpIf{Cond: bpf.JumpBitsSet, Val: cloneNew, SkipFalse: 1},
		bpf.RetConstant{Val: errno(unix.EPERM)},
		bpf.RetConstant{Val: retAllow})
	// Typing into the terminal the launcher was started from.
	oneOf(sysIoctl, 1, []uint32{unix.TIOCSTI, unix.TIOCLINUX}, false, errno(unix.EPERM))
	oneOf(sysSocket, 0, socketFamilies, true, errno(unix.EAFNOSUPPORT))
	oneOf(sysPersonality, 0, personalities, true, errno(unix.EPERM))
	p = append(p, bpf.RetConstant{Val: retAllow})
	return p, nil
}

// Program is the filter as struct sock_filter entries in host order, which
// is what bubblewrap reads.
func Program() ([]byte, error) {
	ins, err := Instructions()
	if err != nil {
		return nil, err
	}
	raw, err := bpf.Assemble(ins)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, 8*len(raw))
	for _, r := range raw {
		out = binary.LittleEndian.AppendUint16(out, r.Op)
		out = append(out, r.Jt, r.Jf)
		out = binary.LittleEndian.AppendUint32(out, r.K)
	}
	return out, nil
}

// File is the program in a sealed in-memory file, ready to pass as an
// extra file: nothing, the sandboxed child included, can change it.
func File() (*os.File, error) {
	prog, err := Program()
	if err != nil {
		return nil, err
	}
	fd, err := unix.MemfdCreate("quartz-seccomp", unix.MFD_CLOEXEC|unix.MFD_ALLOW_SEALING)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), "quartz-seccomp")
	if _, err := f.Write(prog); err != nil {
		_ = f.Close()
		return nil, err
	}
	if _, err := f.Seek(0, 0); err != nil {
		_ = f.Close()
		return nil, err
	}
	if _, err := unix.FcntlInt(f.Fd(), unix.F_ADD_SEALS, unix.F_SEAL_SEAL|unix.F_SEAL_SHRINK|unix.F_SEAL_GROW|unix.F_SEAL_WRITE); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}
