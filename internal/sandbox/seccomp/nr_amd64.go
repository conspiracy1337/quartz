//go:build linux

package seccomp

import "golang.org/x/sys/unix"

const (
	auditArch      = unix.AUDIT_ARCH_X86_64
	x32Bit         = 0x40000000 // __X32_SYSCALL_BIT
	cloneFlagsArg  = 0
	sysClone       = unix.SYS_CLONE
	sysIoctl       = unix.SYS_IOCTL
	sysSocket      = unix.SYS_SOCKET
	sysPersonality = unix.SYS_PERSONALITY
)

var denied = []uint32{
	unix.SYS_PTRACE, unix.SYS_UNSHARE, unix.SYS_SETNS, unix.SYS_MOUNT, unix.SYS_UMOUNT2, unix.SYS_PIVOT_ROOT, unix.SYS_CHROOT,
	unix.SYS_BPF, unix.SYS_PERF_EVENT_OPEN, unix.SYS_USERFAULTFD, unix.SYS_KEYCTL, unix.SYS_ADD_KEY, unix.SYS_REQUEST_KEY,
	unix.SYS_KEXEC_LOAD, unix.SYS_KEXEC_FILE_LOAD, unix.SYS_INIT_MODULE, unix.SYS_FINIT_MODULE, unix.SYS_DELETE_MODULE,
	unix.SYS_CREATE_MODULE, unix.SYS_ACCT, unix.SYS_QUOTACTL, unix.SYS_SYSLOG, unix.SYS_USELIB, unix.SYS_OPEN_BY_HANDLE_AT,
	unix.SYS_NAME_TO_HANDLE_AT, unix.SYS_IO_URING_SETUP, unix.SYS_IO_URING_ENTER, unix.SYS_IO_URING_REGISTER,
	unix.SYS_MOVE_PAGES, unix.SYS_MBIND, unix.SYS_SET_MEMPOLICY, unix.SYS_GET_MEMPOLICY, unix.SYS_MIGRATE_PAGES,
	unix.SYS_VHANGUP, unix.SYS_PROCESS_VM_READV, unix.SYS_PROCESS_VM_WRITEV, unix.SYS_NFSSERVCTL, unix.SYS_LOOKUP_DCOOKIE,
}

var nosys = []uint32{
	unix.SYS_CLONE3, unix.SYS_FSOPEN, unix.SYS_FSCONFIG, unix.SYS_FSMOUNT, unix.SYS_FSPICK, unix.SYS_MOVE_MOUNT, unix.SYS_OPEN_TREE,
	unix.SYS_MOUNT_SETATTR,
}
