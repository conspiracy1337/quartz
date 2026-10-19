# Sandbox red-team findings

**Date:** 2026-10-18
**Target:** Quartz Linux sandbox (bubblewrap + seccomp), `internal/sandbox/`
**Method:** a probe jar (`testdata/redteam/Probe.java`) standing in for a malicious
mod — ordinary code inside the game JVM — run inside a real jail by the harness
`redteam_test.go`, once per preset. Every result below is runtime-observed, not
inferred from reading code.
**Host at test time:** Wayland session with XWayland live (`DISPLAY=:0`), bubblewrap
0.11.1, Temurin 21. The X11 findings apply to any session where `DISPLAY` is set
(the launcher keys off that), which includes XWayland and plain X11.

## How to reproduce

```
QUARTZ_REDTEAM=1 go test ./internal/sandbox -run TestRedteamEscapes -v
```

Needs a desktop session (`DISPLAY`) and a JDK >= 16 (java+javac+jar); point
`QUARTZ_JDK_BIN` at its `bin/` if auto-detection misses it. The test fails only on
a broken security invariant; the escapes are reported in its log.

## Runtime matrix

| vector | Standard (default) | Strict | meaning |
|---|---|---|---|
| fs-launcher-secret | BLOCKED | BLOCKED | launcher account store unreadable |
| fs-home-secret | BLOCKED | BLOCKED | real `~/.ssh` unreadable |
| no-new-privs | NoNewPrivs=1 | NoNewPrivs=1 | suid binaries neutralised |
| proc-pids | isolated | isolated | only jail PIDs visible |
| seccomp-unshare | BLOCKED | BLOCKED | `unshare(2)` refused (EPERM) |
| seccomp-mount | BLOCKED | BLOCKED | `mount(2)` refused |
| seccomp-ptrace | BLOCKED | BLOCKED | `ptrace(2)` refused |
| net-loopback | **REACHABLE** | BLOCKED | reach host `127.0.0.1` services |
| net-netlink | **REACHABLE** | BLOCKED | enumerate host interfaces/routes |
| x11-handshake | **REACHABLE** | **REACHABLE** | authenticated to the real X server |
| x11-visibility | **REACHABLE** | **REACHABLE** | 21 other windows visible in X tree |
| x11-capture | **REACHABLE** | **REACHABLE** | `GetImage` returns pixels (full screen) |
| x11-inject | **REACHABLE** | **REACHABLE** | `XTEST` FakeInput accepted |
| x11-keylog-ext | PRESENT | PRESENT | RECORD + XInput2 available |
| leak-machine-id | LEAK | LEAK | stable host id readable |
| leak-dmi | LEAK | LEAK | host hardware model readable |
| devices | OPEN | OPEN | `/dev/dri`,`/dev/snd` (by design) |

## Status (2026-10-18 hardening round)

- **#1 X11 takeover — FIXED.** The launcher no longer hands the game the host X
  server. On a Wayland session it is Wayland-only (the X socket/cookie are not
  bound and `DISPLAY`/`XAUTHORITY` are dropped); on an X session it mints an
  untrusted cookie via the X SECURITY extension; only when neither is possible
  does it fall back to trusted X (logged). `QUARTZ_X11=1` forces the X server
  back for a game whose GLFW is X11-only. See `displayMode`/`applyDisplayEnv` in
  `internal/sandbox/display_linux.go`.
- **#3 fingerprint leak — FIXED.** `/etc/machine-id` is replaced by a fresh
  random per-run id; the `/sys` DMI subtree is masked. See `machineID` and the
  DMI `Tmpfs` in `desktop()`.
- **#2 shared network — ACCEPTED RISK (not fixed).** Per the maintainer: the
  Minecraft malware threat model is account/credential theft, not outbound
  network abuse, and a real fix needs a userspace NAT that isn't available. The
  game keeps host network access under Standard. (Update 2026-10-19: Strict no
  longer removes the network either — a no-network server is useless and
  outbound abuse isn't the threat; only the Custom `Blocked` internet level
  unshares the net namespace now. Windows, which can filter by capability, keeps
  a LAN-only Strict.)

## Status (round 2)

- **#1 follow-up: host abstract unix sockets — FIXED (Linux 6.12+).** Abstract
  sockets belong to the network namespace, not the filesystem, so under Standard
  (shared host network) the jail could still reach every host service listening
  on one, the X server's `@/tmp/.X11-unix/X0` included, without any bind mount.
  On an X session that allows local-user access, this would have bypassed the
  untrusted cookie. Each run now goes through the launcher as a shim
  (`internal/core/landlock`) that applies a Landlock scope
  (`ABSTRACT_UNIX_SOCKET | SIGNAL`) and then execs bubblewrap in place. The
  jail's own abstract sockets still work; host ones get `EPERM`. Older
  kernels, or a setuid bwrap, log a warning and run unscoped; Strict is still
  covered there by its own net namespace.
- **#3 follow-up: hostname, NIC MACs, disk serials — FIXED.** The jail is named
  `quartz` (`--hostname`) and gets a synthesized `/etc/hosts` (the host's
  entries minus any line naming the host, so LAN names still resolve). The
  sysfs dirs of network cards and block devices are masked next to DMI
  (`maskSys` in `scrub_linux.go`); GPU PCI/DRM nodes stay.
- The matrix's `leak-machine-id` row still prints LEAK because some id is
  readable; `checkInvariants` asserts it is not the host's.

The jail tests (`TestJailStandard`, run by `go test`) assert the abstract-socket
scope, the hostname and that no MAC address is readable. Without the shim, the
abstract check fails.

The regression tests for all of the above live in `redteam_test.go`
(`checkInvariants`): run `QUARTZ_REDTEAM=1 go test ./internal/sandbox -run
TestRedteamEscapes -v`.

## Round 3 — adversarial re-test (2026-10-19)

A pen-test pass that attacks the seams round 2 created or left open, fired the way
a real malicious mod would: raw libc `socket`/`connect`/`syscall` through the
JDK's Foreign Function API (a mod can equally bundle a JNI `.so`), not Java's
stdlib. Run it with a JDK >= 22 so the FFI vectors execute (they `SKIP` on 21):

```
QUARTZ_JDK_BIN=~/.jdks/openjdk-26.0.2/bin QUARTZ_REDTEAM=1 \
  go test ./internal/sandbox -run TestRedteamEscapes -v
```

Runtime matrix (this KDE/Wayland + XWayland box, kernel 7.0, Landlock active):

| vector | Standard | Strict | meaning |
|---|---|---|---|
| abstract-x11 | BLOCKED | BLOCKED | host X server's **abstract** socket `@/tmp/.X11-unix/X0` unreachable — the round-2 fix holds even with no `DISPLAY` |
| abstract-canary | BLOCKED | BLOCKED | a known-present host abstract socket unreachable (Landlock scope, proven against a live target) |
| x11-tcp | BLOCKED | BLOCKED | no host X server on TCP 6000+ |
| sock-family-alg | BLOCKED | BLOCKED | `AF_ALG` → `EAFNOSUPPORT` (seccomp family allowlist runs) |
| sock-family-packet | BLOCKED | BLOCKED | `AF_PACKET` raw socket refused (no host packet sniffing) |
| seccomp-raw | BLOCKED | BLOCKED | raw `unshare/mount/ptrace/bpf/io_uring_setup/keyctl/setns/perf_event_open/process_vm_readv/add_key` all `EPERM`/`ENOSYS` |
| hw-ids | BLOCKED | BLOCKED | no NIC MAC, disk serial or DMI serial readable |
| env-leak | BLOCKED | BLOCKED | `FilterEnv` strips a planted `AWS_SECRET_ACCESS_KEY`; no credential-shaped vars reach the game |
| proc-net | **REACHABLE** | BLOCKED | `/proc/net/tcp*` lists the host connection table (no tools needed) under the shared netns |
| audio-capture | **OPEN** | **OPEN** | PipeWire/Pulse sockets reachable → microphone capture surface |

Negative control: rerunning one probe with the Landlock shim disabled
(`Shim: ""`) flips `abstract-x11` and `abstract-canary` to **REACHABLE** under
Standard — i.e. the mod connects straight to the host X server again — proving the
scope is what closes it (and that the test has teeth). Under Strict both stay
BLOCKED even without the shim, because Strict's net-namespace unshare isolates
abstract sockets independently (defense in depth).

### No new escapes. Everything round 2 fixed still holds, with runtime proof.

Two findings are **accepted risk** (owner decision), documented, not changed:

- **Shared host network under Standard** (unchanged from #2) now also covers
  `proc-net`: a mod reads `/proc/net/tcp`+`tcp6` directly to enumerate the host's
  listening and established sockets — passive recon with no `ip`/netlink needed.
  Only the Custom `Blocked` internet level removes it with the rest of the
  network (Strict no longer does; see the 2026-10-19 note above).
- **Microphone / audio capture.** `Graphics` is always-on (even under Strict) and
  binds the PipeWire/Pulse sockets, so a mod that bundles the client libraries can
  record the microphone. Same class as the GPU/audio device nodes: a by-design
  opening for the game's own audio. Left as accepted risk this round; closing it
  would need a separate Microphone rule (UI) or dropping the audio socket under
  Strict only.

The round-3 vectors and invariants live in `redteam_test.go` and
`testdata/redteam/Probe.java`.

## Findings, ranked (original proof)

### 1. HIGH — X11/XWayland full takeover; not closed by any preset

A mod speaks the X11 wire protocol over the bound path socket from pure Java
(no native code, no AWT), authenticates with the bound `.Xauthority` cookie, and
then can: screen-capture the whole root window (observed root `6400x1440`),
enumerate and read every other client's windows (21 seen), inject synthetic
keystrokes/clicks into any focused window via `XTEST`, and passively keylog every
X client via the `RECORD`/`XInput2` extensions. The probe proved each capability
against a drawable/input it created itself (canary-only); the same calls target
real windows unchanged.

- **Why:** when `DISPLAY` is set the launcher binds the real X socket and the
  cookie read-only and intentionally leaves the IPC namespace shared —
  [linux.go:69-74](../../internal/sandbox/linux.go#L69-L74),
  [linux.go:293-308](../../internal/sandbox/linux.go#L293-L308). Graphics is an
  always-allowed rule ([rules.go:75](../../internal/sandbox/rules/rules.go#L75)),
  so Strict still binds it — the X11 row stays REACHABLE under Strict.
- **Blast radius:** full session compromise. Capture passwords typed into a
  browser, inject commands into a terminal, screenshot anything on screen. X11
  has no inter-client isolation; the sandbox hands the mod the session's X server.
- **Hardening (follow-up):** do not expose the host X server. Prefer the Wayland
  socket only when a Wayland session is present (it *is* here); for X sessions,
  either run a nested server (Xpra/Xephyr) or issue an untrusted cookie
  (`xauth generate ... untrusted`, GE `SECURITY` extension) so the client is
  confined; and make display access a user-visible rule that Strict blocks.

### 2. MEDIUM — Shared host network namespace under the default preset

Under Standard the game shares the host network namespace, so it reaches host
loopback services (observed: TCP connect to a `127.0.0.1` listener) and
enumerates host interfaces, routes and addresses via netlink (observed:
`docker0`, bridges, `veth*`, `enp4s0`, `wlp3s0`). Strict closes both.

- **Why:** `--unshare-net` is added only when Internet is Blocked —
  [linux.go:75-77](../../internal/sandbox/linux.go#L75-L77). "Internet allowed"
  currently means "the host's whole network stack", not "the internet".
- **Blast radius:** reach any service bound to host loopback/LAN (dev servers,
  databases, other apps' debug ports, DBus/IPC on TCP), plus network-topology
  recon and a host fingerprint from interface names/MACs.
- **Hardening (follow-up):** give the game its own net namespace even when
  internet is allowed, bridged to the outside with a userspace NAT (pasta/slirp)
  as Flatpak does, so loopback and LAN are not exposed; or at minimum document
  that Standard shares the host network.

### 3. LOW — Host fingerprint leaks (`/etc/machine-id`, `/sys` DMI)

`/etc/machine-id` (a stable, cross-boot host identifier) and DMI fields
(`product_name`, `board_vendor`, `sys_vendor`) are readable inside the jail.

- **Why:** `machine-id` is in the bound `/etc` set
  ([linux.go:51](../../internal/sandbox/linux.go#L51)); DMI comes in with the
  broad sysfs binds `/sys/class` and `/sys/devices`
  ([linux.go:55-56](../../internal/sandbox/linux.go#L55-L56),
  [linux.go:288-292](../../internal/sandbox/linux.go#L288-L292)).
- **Blast radius:** stable cross-instance/cross-server fingerprinting of the user
  and machine; `machine-id` may also key unrelated services.
- **Hardening (follow-up):** bind a synthesized random `machine-id` instead of the
  host's; narrow the sysfs binds to the GPU nodes the drivers actually need, or
  mask `/sys/class/dmi`.

### 4. INFO / by-design — Direct GPU and audio device nodes

`/dev/dri`, `/dev/snd` (and `/dev/nvidia*` where present) are opened directly by
the game — [linux.go:278-287](../../internal/sandbox/linux.go#L278-L287). Required
for graphics/audio, but it is a direct host-kernel (GPU ioctl) attack surface.
Noted, not a defect.

## Defenses that held (confirmed at runtime)

- **Filesystem wall** — the launcher's data root and the real home folder are both
  unreadable (data root and home are tmpfs'd, only allowed paths rebound):
  [linux.go:100-116](../../internal/sandbox/linux.go#L100-L116).
- **no_new_privs = 1** — confirmed in `/proc/self/status`; bwrap's implicit setting
  holds, so bound suid binaries cannot escalate.
- **PID namespace** — the jail sees only its own processes.
- **seccomp denylist** — `unshare`, `mount` and `ptrace` are all refused
  ([seccomp/nr_amd64.go](../../internal/sandbox/seccomp/nr_amd64.go)).
- **The `Blocked` internet level removes the network** — loopback and netlink both
  blocked once the net namespace is unshared. (Strict kept this originally; as of
  2026-10-19 Strict keeps the network and only Custom `Blocked` unshares it.)
