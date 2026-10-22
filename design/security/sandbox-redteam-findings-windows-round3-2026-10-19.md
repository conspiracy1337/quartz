# Sandbox red-team findings — Windows, Round 3

**Date:** 2026-10-19
**Target:** Quartz Windows sandbox (AppContainer / LowBox token + capability SIDs +
per-path NTFS DACLs + kill-on-close job object), `internal/sandbox/windows.go` and
`internal/core/procx/contain_windows.go`.
**Method:** a second probe jar (`testdata/redteam/WinProbe2.java`) and a native helper
(`testdata/redteam/breakaway.go`) standing in for a malicious mod — ordinary code inside
the game JVM — run inside a **real AppContainer** by the harness `redteam_windows_round3_test.go`,
once per preset. This round deliberately attacks the *next layer* beyond rounds 1–2
(`sandbox-redteam-findings-windows.md`): escaping the kill-on-close job via a service/broker,
stripping the sealed `mods/` DACL by deletion, job breakaway, registry persistence, clipboard
theft, and a *pre-existing* reparse point. Every result is runtime-observed on a live
AppContainer and each escape attempt is backed by a direct second experiment (survivor still
writing after the jail closed; the real HKCU hive read back; the out-of-grant target's DACL
re-inspected).
**Host at test time:** Windows 11 Pro 26200, Go 1.26.9, Temurin JDK 21.0.12.1. AppContainer
backend reported Available.
**Scope note:** this was an **audit-only** engagement — no product sandbox code was changed.
The probe/harness files are diagnostic tooling, gated behind `QUARTZ_REDTEAM`, not shipped
(same class as `redteam_windows_test.go`).

## Headline

**No new escape was found.** Every next-layer vector held. The job-based containment is now
proven comprehensive (direct child, native `CREATE_BREAKAWAY_FROM_JOB`, and both common
service brokers — Task Scheduler and WMI — are all refused), the sealed-folder protection
resists deletion/rename as well as writes, the clipboard and registry-persistence surfaces are
denied to the LowBox, and the launcher's inheritable ACL pass does not follow a pre-planted
junction. The only reachable surface is the by-design read of `HKLM` that every AppContainer
has by construction. No code change is required by this round.

## How to reproduce

```
$env:QUARTZ_REDTEAM="1"; $env:QUARTZ_JDK_BIN="C:\quartz-rt\jdk\jdk-21.0.12.1+1\bin"
C:\quartz-rt\go\bin\go.exe test -count=1 -v -timeout 600s ./internal/sandbox/ \
  -run 'TestRound3Escapes|TestRound3PreplantedJunction'
```

Needs a JDK (java+javac+jar) and the Go toolchain (to build the native breakaway helper;
`QUARTZ_GO_BIN` overrides the path). The suite records a matrix and logs; being an audit pass
it does not assert new invariants. Test roots land under `C:\quartz-rt\run-*` (auto-cleaned).

## Runtime matrix (new vectors)

| vector | Standard | Strict | meaning |
|---|---|---|---|
| broker-schtasks | BLOCKED | BLOCKED | Task Scheduler `/create` → `Access is denied` from the LowBox |
| broker-schtasks-alive | BLOCKED | BLOCKED | no task payload survived the jail close |
| broker-wmi | BLOCKED | BLOCKED | `Win32_Process.Create` → `Access to a CIM resource was not available` |
| broker-wmi-alive | BLOCKED | BLOCKED | no WMI-spawned payload survived |
| breakaway | DENIED | DENIED | `CREATE_BREAKAWAY_FROM_JOB` → `Access is denied`; baseline child spawns but stays *in-job* |
| breakaway-alive | BLOCKED | BLOCKED | the in-job baseline child died with the game |
| seal-delete-mod | BLOCKED | BLOCKED | the sealed store blob in `mods/` cannot be deleted |
| seal-rename-mods | BLOCKED | BLOCKED | `mods/` cannot be renamed out of the way |
| seal-rmdir-mods | BLOCKED | BLOCKED | `mods/` cannot be removed |
| seal-reseal-bypass | BLOCKED | BLOCKED | so a fresh, unsealed `mods/` cannot be recreated |
| clipboard-read | BLOCKED | BLOCKED | `cannot open system clipboard` (persists across 6 retries → denial, not contention) |
| reg-hkcu-run-write | BLOCKED | BLOCKED | HKCU `...\Run` write → `Access is denied` |
| reg-hkcu-persist | BLOCKED | BLOCKED | nothing landed in the *real* user hive |
| reg-hklm-run-write | BLOCKED | BLOCKED | HKLM `...\Run` write → `Access is denied` |
| reg-hklm-read | **OPEN** | **OPEN** | HKLM is readable (by design; see accepted risks) |
| preplanted-junction (own test) | HELD | — | Wrap's inheritable ACL grant did not stamp the out-of-grant junction target |

## Findings, ranked

### 1. INFO / accepted — the game can read `HKLM` (information disclosure only)

A contained mod can `reg query` machine keys (confirmed: it read
`HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProductName`). This is inherent to the
AppContainer model — most of `HKLM` is ACL'd to `ALL APPLICATION PACKAGES`, so any AppContainer
reads it by construction, exactly like much of `System32`. It is a read-only information surface
(OS version, installed-software inventory), not a write or an escape: **every** write attempt
(HKCU and HKLM `...\Run`) was denied, and nothing reached the real user hive. Same class and
rationale as the "`ALL APPLICATION PACKAGES` resources" item already accepted in rounds 1–2.
**Severity: informational. Recommendation: accept** — no per-instance ACL can revoke an
`ALL APPLICATION PACKAGES` grant on the system hive, and the data disclosed is not sensitive.
If ever deemed worth it, the only lever is a restricted/lessprivileged AppContainer with an
explicit capability set, which is disproportionate to a version-string read.

### No other findings

Every other probed vector was blocked. The sections below record *why each defense held*, with
the runtime evidence, because that is the useful product of an audit that finds nothing new.

## Defenses that held (confirmed at runtime)

- **Job escape via a service broker — refused at the broker.** The round-2 kill-on-close job
  catches direct children, so a real escape would have to spawn through a *service* (whose child
  is not in the game's job). Both standard brokers are unreachable from the LowBox:
  `schtasks /create` returns `ERROR: Access is denied` and WMI `Win32_Process.Create` returns
  `Access to a CIM resource was not available to the client`. A normal (unsandboxed) user can do
  both on this host, so the denial is the sandbox, not a syntax or privilege artifact. No payload
  appeared or grew after the jail closed (`broker-*-alive = BLOCKED`).
- **`CREATE_BREAKAWAY_FROM_JOB` denied.** A native helper tried to spawn a child that leaves the
  job: the OS refused with `Access is denied` because `newKillOnCloseJob` omits
  `JOB_OBJECT_LIMIT_BREAKAWAY_OK`. The control proves the test is real: the *same* helper's plain
  child (no breakaway flag) **did** spawn (`baseline-child=OK pid=…-in-job`) and then died with
  the game (`breakaway-alive=BLOCKED`). Direct, breakaway, and brokered escapes are therefore all
  covered — the leftover-process protection is comprehensive, not just the direct-child case
  round 2 tested.
- **Sealed `mods/` resists deletion, not just writes.** The `FILE_DELETE_CHILD` bit the game dir
  grants raised the question of whether a mod could delete the sealed store blobs, empty `mods/`,
  or rename/remove the folder to shed its `PROTECTED` DACL and recreate a fresh, inheriting one.
  All four steps are denied (`AccessDeniedException` on the blob delete, the folder rename, and
  the rmdir), so `seal-reseal-bypass` never gets off the ground. `sealReadOnly`'s read-only grant
  carries no `DELETE`, and sealing the folder `PROTECTED` leaves the container no delete-child on
  it either — so the store-protection from round 1 is robust against the deletion angle too.
- **Clipboard not readable.** The contained JVM cannot even open the system clipboard
  (`IllegalStateException: cannot open system clipboard`), tried 6× over ~1.5 s to rule out a
  transient owner. A mod cannot lift credentials a user copied, despite the window-station
  capability the game holds for cursor control (that capability grants
  `WINSTA_WRITEATTRIBUTES` only, not clipboard access).
- **No registry persistence.** HKCU and HKLM `...\Run` writes are both denied, and a read-back of
  the *real* user hive after the run found nothing — the AppContainer's HKCU is virtualised away
  from the user's actual hive, so even a "successful" write would not persist for logon. The mod
  has no autostart foothold.
- **Pre-planted junction not followed by the ACL pass.** Rounds 1–2 showed a mod cannot *create*
  a junction inside the sandbox. This round planted one *from outside* (as the user/launcher,
  modelling a modpack import or a prior unsandboxed run) — `game\preplanted -> outside` — then ran
  `Wrap`. The inheritable `SetNamedSecurityInfo` grant on the game dir propagated to the real
  child folders (as intended) but did **not** traverse the reparse point: the out-of-grant target
  and its `secret.txt` carried no container ACE afterward. So an attacker cannot pre-seed a
  junction and have the launcher stamp its own grant onto an arbitrary outside path on the next
  launch.

## Accepted / by-design risks (unchanged from rounds 1–2, re-confirmed)

- **`HKLM` read / `ALL APPLICATION PACKAGES` resources** — see finding 1.
- **Outbound network under Standard** — Standard grants the internet capability; Strict removes it
  (LAN-only). Network abuse is not the threat model (the filesystem wall is).
- **`%USERPROFILE%` path disclosure** — the variable is passed through unredirected; it discloses
  the path string, not access (every read/write under it remains ACL-gated and was blocked in
  rounds 1–2).

## Residual recommendations

None are required. Two optional, low-priority hardening ideas, recorded for completeness:

1. **Keep the pre-planted-junction guard as a regression test.** `TestRound3PreplantedJunction`
   (and the round-1 `TestJunctionEscape`) assert that the ACL pass stays reparse-safe. If a future
   Windows build or a refactor of `setACE`/`grantPath` ever changed the propagation to follow
   reparse points, this would catch it. Worth folding into the always-on jail tests rather than the
   `QUARTZ_REDTEAM`-gated suite, so CI guards it without a JDK.
2. **Belt-and-braces job hardening (not needed today).** Breakaway is already denied because the
   job omits `BREAKAWAY_OK`. If ever desired, `JOB_OBJECT_LIMIT_SILENT_BREAKAWAY_OK` is explicitly
   *not* set (correct — setting it would silently allow breakaway), and the job could additionally
   carry UI restrictions; neither changes the current (secure) outcome.

## Methodology notes (for the next auditor)

- The job-escape vectors are only meaningful once the broker command itself is well-formed.
  `schtasks /tr` mis-splits any value containing embedded quotes; the probe avoids quotes entirely
  (the run root has no spaces) and drives WMI through a `.cmd` wrapper for the same reason. An
  early run produced a *false* BLOCKED from a schtasks parse error — always confirm the broker
  actually reached its service (here: a clean `Access is denied` from the service, plus the
  unsandboxed positive control) before recording a block.
- Survival is measured by canary-file growth **after** `Jail.Close()`, mirroring `checkLeftover`
  in the round-1 harness, and every survivor is reaped by matching command line (schtasks task
  deleted, `powershell`/`cmd`/`ping` payloads stopped) so a diagnostic run leaves nothing behind.
