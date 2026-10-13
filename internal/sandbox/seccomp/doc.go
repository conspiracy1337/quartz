// Package seccomp builds the system-call filter a sandboxed game runs under,
// as a classic BPF program bubblewrap loads (--seccomp FD). It is generated
// in Go (golang.org/x/net/bpf), with no libseccomp and no cgo. The rules are
// Flatpak's proven blocklist: a game needs almost every ordinary call, and
// what it never needs (namespaces, mounts, tracing, kernel keyrings and
// modules, terminal injection) is refused.
package seccomp
