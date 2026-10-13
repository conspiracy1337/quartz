//go:build linux && (amd64 || arm64)

package seccomp

import (
	"encoding/binary"
	"testing"

	"golang.org/x/net/bpf"
	"golang.org/x/sys/unix"
)

// call runs the filter in x/net/bpf's VM on one seccomp_data. The VM loads
// words big-endian where the kernel loads them in host order, so each
// 32-bit field is written big-endian here: the filter sees the same values.
func call(t *testing.T, vm *bpf.VM, arch, nr uint32, args ...uint32) uint32 {
	t.Helper()
	pkt := make([]byte, 64)
	binary.BigEndian.PutUint32(pkt[offNr:], nr)
	binary.BigEndian.PutUint32(pkt[offArch:], arch)
	for i, a := range args {
		binary.BigEndian.PutUint32(pkt[arg(i):], a)
	}
	out, err := vm.Run(pkt)
	if err != nil {
		t.Fatal(err)
	}
	return uint32(out) //nolint:gosec // filter return values are 32-bit
}

func TestFilter(t *testing.T) {
	ins, err := Instructions()
	if err != nil {
		t.Fatal(err)
	}
	vm, err := bpf.NewVM(ins)
	if err != nil {
		t.Fatal(err)
	}
	eperm, enosys := errno(unix.EPERM), errno(unix.ENOSYS)
	cases := []struct {
		name string
		arch uint32
		nr   uint32
		args []uint32
		want uint32
	}{
		{"read", auditArch, unix.SYS_READ, nil, retAllow},
		{"mmap", auditArch, unix.SYS_MMAP, nil, retAllow},
		{"other arch", 0x40000003, unix.SYS_READ, nil, retKill},
		{"ptrace", auditArch, unix.SYS_PTRACE, nil, eperm},
		{"unshare", auditArch, unix.SYS_UNSHARE, []uint32{unix.CLONE_NEWUSER}, eperm},
		{"mount", auditArch, unix.SYS_MOUNT, nil, eperm},
		{"bpf", auditArch, unix.SYS_BPF, nil, eperm},
		{"clone3", auditArch, unix.SYS_CLONE3, nil, enosys},
		{"thread", auditArch, unix.SYS_CLONE, []uint32{unix.CLONE_VM | unix.CLONE_THREAD | unix.CLONE_SIGHAND}, retAllow},
		{"clone userns", auditArch, unix.SYS_CLONE, []uint32{unix.CLONE_NEWUSER}, eperm},
		{"clone netns", auditArch, unix.SYS_CLONE, []uint32{unix.CLONE_NEWNET | 0x11}, eperm},
		{"ioctl tcgets", auditArch, unix.SYS_IOCTL, []uint32{0, unix.TCGETS}, retAllow},
		{"ioctl tiocsti", auditArch, unix.SYS_IOCTL, []uint32{0, unix.TIOCSTI}, eperm},
		{"ioctl tioclinux", auditArch, unix.SYS_IOCTL, []uint32{0, unix.TIOCLINUX}, eperm},
		{"socket inet6", auditArch, unix.SYS_SOCKET, []uint32{unix.AF_INET6}, retAllow},
		{"socket unix", auditArch, unix.SYS_SOCKET, []uint32{unix.AF_UNIX}, retAllow},
		{"socket vsock", auditArch, unix.SYS_SOCKET, []uint32{unix.AF_VSOCK}, errno(unix.EAFNOSUPPORT)},
		{"socket packet", auditArch, unix.SYS_SOCKET, []uint32{unix.AF_PACKET}, errno(unix.EAFNOSUPPORT)},
		{"personality query", auditArch, unix.SYS_PERSONALITY, []uint32{0xffffffff}, retAllow},
		{"personality exec", auditArch, unix.SYS_PERSONALITY, []uint32{0x0400000}, eperm},
	}
	if x32Bit != 0 {
		cases = append(cases, struct {
			name string
			arch uint32
			nr   uint32
			args []uint32
			want uint32
		}{"x32", auditArch, x32Bit | unix.SYS_READ, nil, eperm})
	}
	for _, c := range cases {
		if got := call(t, vm, c.arch, c.nr, c.args...); got != c.want {
			t.Errorf("%s: %#x, want %#x", c.name, got, c.want)
		}
	}
}

func TestProgramAndFile(t *testing.T) {
	ins, _ := Instructions()
	prog, err := Program()
	if err != nil || len(prog) != 8*len(ins) || len(ins) > 4096 {
		t.Fatalf("%d bytes for %d instructions: %v", len(prog), len(ins), err)
	}
	f, err := File()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write([]byte{0}); err == nil {
		t.Fatal("the sealed filter could be written")
	}
}
