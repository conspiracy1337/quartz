# Sandbox red-team findings — Windows

**Date:** 2026-10-19
**Target:** Quartz Windows sandbox (AppContainer / LowBox token + capability SIDs +
per-path NTFS DACLs), `internal/sandbox/windows.go` and `internal/core/procx/contain_windows.go`
**Method:** a probe jar (`testdata/redteam/WinProbe.java`) standing in for a malicious
mod — ordinary code inside the game JVM — run inside a real AppContainer by the harness
`redteam_windows_test.go`, once per preset. Every result below is runtime-observed on a
live AppContainer, not inferred from reading code; findings were each confirmed by a
direct experiment (e.g. the store blob was read back after the write to prove mutation).
**Host at test time:** Windows 11 Pro 26200, Go 1.26, Temurin JDK 21. AppContainer
backend reported Available.

This is the Windows counterpart to `sandbox-redteam-findings.md` (Linux). The threat
model is the same: a hostile Minecraft mod whose goal is credential/filesystem theft and
tampering with the shared content-addressed store, not outbound network abuse.

## How to reproduce

```
QUARTZ_REDTEAM=1 go test ./internal/sandbox -run TestRedteamEscapes -v
QUARTZ_REDTEAM=1 go test ./internal/sandbox -run TestJunctionEscape -v
```

Needs a JDK (java+javac+jar); point `QUARTZ_JDK_BIN` at its `bin/` if auto-detection
misses it. The suite fails only on a broken security invariant; surfaces and accepted
risks are reported in its log. The canonical jail tests (`TestJailStandard`, run without
`QUARTZ_REDTEAM`) exercise the same confinement with a Go re-exec probe and assert the
same store-write protection, as a second, JDK-free check.

## Runtime matrix

Measured before and after the hardening in this round. "before" is the code as audited;
"after" is with the three fixes below.

| vector | Standard before | Standard after | Strict after | meaning |
|---|---|---|---|---|
| fs-launcher-secret | BLOCKED | BLOCKED | BLOCKED | launcher account store unreadable |
| fs-home-secret | BLOCKED | BLOCKED | BLOCKED | a home-like `~/.ssh` key unreadable |
| fs-other-instance | BLOCKED | BLOCKED | BLOCKED | another instance's folder unreadable |
| write-store-mod | **REACHABLE** | BLOCKED | BLOCKED | modify a hard-linked store blob via `mods/` |
| add-mod | **REACHABLE** | BLOCKED | BLOCKED | add a file to the protected `mods/` folder |
| write-protected-file | **REACHABLE** | BLOCKED | BLOCKED | overwrite a read-only store pack |
| list-ancestors | **LEAK** `[a,b]` | BLOCKED | BLOCKED | enumerate sibling instances up the tree |
| list-home | BLOCKED | BLOCKED | BLOCKED | real `%USERPROFILE%` not listable |
| leftover-alive | **REACHABLE** | BLOCKED | BLOCKED | detached process outlives game + jail |
| junction-plant | BLOCKED | BLOCKED | BLOCKED | mod cannot create a reparse point |
| net-internet | REACHABLE | REACHABLE | **BLOCKED** | outbound internet (Strict removes it) |
| net-loopback | BLOCKED | BLOCKED | BLOCKED | host `127.0.0.1` unreachable (AppContainer default) |
| env-leak | BLOCKED | BLOCKED | BLOCKED | planted secret env var filtered out |
| token-info | Low / AppContainer | same | same | confirmed LowBox at Low integrity |
| read-store | OK | OK | OK | granted store library readable (control) |
| write-gamedir | OK | OK | OK | game folder writable (control) |
| add-pack-file | OK | OK | OK | game's own file beside a pack (control) |

## Findings, ranked

### 1. CRITICAL — store-protection bypass: a mod could write shared store blobs

Under the audited code a mod could modify a file in `mods/`, add new files to `mods/`,
and overwrite read-only store files in content folders (`resourcepacks/`). Confirmed by
writing through the `mods/a.jar` hard link and reading the store blob back: its bytes
changed to the attacker's. Because store blobs are hard links shared across every
instance (including unsandboxed ones), this let one sandboxed mod infect the store for
all of them — exactly what the protection exists to prevent.

**Why the old approach failed.** The code added an inheritable *deny* ACE for the
container (package) SID on `mods/` and on protected store files. A Windows AppContainer
access check has two passes: a normal pass over the token's user/groups, and an
app-container pass that needs a positive grant to the package SID, a capability, or ALL
APPLICATION PACKAGES. The game-folder grant hands the package SID an inheritable MODIFY
that reaches `mods/` and the content folders; the app-container pass honours that allow
and does **not** treat a deny for the same package SID as a veto. The deny ACE was
therefore inert (and the existing unit test only checked ACE *ordering*, never an actual
write). Proof that the model, not the ordering, was the problem: `accounts.json`, which
has `Authenticated Users:(M)` but *no* package ACE, was correctly unreadable — the
app-container pass fails with no positive grant.

**Fix (`sealReadOnly`, `windows.go`).** Stop relying on deny. For each protected folder
and store file, drop the container's entries, grant it read-only, and seal the DACL
(`PROTECTED_DACL_SECURITY_INFORMATION`) so the game folder's inheritable MODIFY can't
flow back in. With no positive write grant, the app-container pass denies writes while
reads still succeed. Verified: `write-store-mod`/`add-mod`/`write-protected-file` →
BLOCKED, store blob unchanged; `read-store`/`add-pack-file` still OK.

### 2. MEDIUM — leftover detached process outlives the game and the jail

A mod could start a detached process that kept running — still holding the container's
granted folders (and network, under Standard) — after the game exited and the jail was
released. Confirmed: the spawned process was alive with growing memory and still writing
into the granted game folder after `Jail.Close()`. The AppContainer has no equivalent of
the Linux PID namespace + `--as-pid-1`; `contain_windows.go` created the process with only
the security-capabilities and handle-list attributes, and `Jail.Close()` is a no-op on
Windows, so nothing reaped children.

**Fix (`newKillOnCloseJob`, `contain_windows.go`).** The contained game is now created
suspended, joined to a job object with `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`
(+`DIE_ON_UNHANDLED_EXCEPTION`), then resumed — so no child can escape before it is in
the job. Closing the job's last handle when the game is reaped terminates any leftover.
Verified: `leftover-alive` → BLOCKED (the same survivor that grew the file before now
never runs past the game).

### 3. MEDIUM — ancestor-listing leak: cross-instance directory enumeration

A mod could enumerate the game folder's ancestors and see sibling instances (observed:
listing the instances folder returned `[a, b]`). The parent-traverse grant used
`accessTraverse = FILE_GENERIC_READ`, which includes `FILE_LIST_DIRECTORY`, and was
applied to the shared `quartz.store` capability on every ancestor up to the drive root —
so every Quartz instance could list every other instance's enclosing directories, and in
the production layout (`%LOCALAPPDATA%\quartz\…`) the chain reaches the user profile.

**Fix (`windows.go`).** `accessTraverse` is now `FILE_TRAVERSE | FILE_READ_ATTRIBUTES`
(`0x000000A0`) — enough to pass through, without `FILE_LIST_DIRECTORY`. Verified:
`list-ancestors` → BLOCKED while the game still reaches its granted folders.

**Partly reverted, by decision (2026-10-23).** `Path.toRealPath()` on Windows looks every
path element up in its parent (FindFirstFile), so mod loaders that resolve the game folder
(Fabric's `mods`) died with `AccessDeniedException` when the folder's parents were
traverse-only. The parents of the game folder (and of Java, already since Java 24+ support)
now carry a non-inherited, name-only list ACE for `quartz.store` (`allowParentListing`,
`listing_windows.go`). `list-ancestors` is therefore LEAK again, by design: a mod sees
sibling instance folder *names*, never their contents (`fs-other-instance` stays BLOCKED).
The harness now asserts `realpath-mods` → OK instead. With the default data root under
`%LOCALAPPDATA%`, the profile folder's own entry names are listable as well. Java's parents
already caused that.

## Defenses that held (confirmed at runtime)

- **Filesystem wall.** The launcher account store, a home-like secret, and another
  instance's folder are all unreadable; the real `%USERPROFILE%` is not listable.
- **Network under Strict.** Strict sets the Internet level to LAN-only (game and server
  alike): the private-network capability is granted but no internet client, so outbound
  internet is BLOCKED while LAN/VPN still works. Loopback is unreachable under *all* cases
  (AppContainers are denied loopback by default), a stronger position than the Linux
  Standard case. See finding 4 for the full three-level model.
- **Env allowlist.** A secret planted in the launcher's environment is filtered out
  (`FilterEnv`) and never reaches the game.
- **LowBox token.** The game runs as a Low-integrity AppContainer; children inherit the
  LowBox and cannot break out upward.
- **Junction creation blocked.** A mod cannot create a directory junction inside its
  writable game folder (reparse-point creation is refused for the LowBox token), so the
  "plant a junction, let the launcher's inheritable ACL stamp the target" escape is not
  reachable from inside the sandbox. `TestJunctionEscape` therefore skips; keep it as a
  guard in case a future Windows build grants reparse creation to AppContainers, at which
  point the launcher's ACL application should be made junction-safe (skip reparse points).

## Accepted / by-design risks

- **LAN/loopback and outbound internet under Standard.** Standard grants the internet
  and private-network capabilities, so a mod can reach the network (Strict removes it).
  This mirrors the Linux "shared host network" accepted risk and the same threat-model
  rationale (credential theft, not network abuse).
- **`ALL APPLICATION PACKAGES` resources.** Any host resource already ACL'd to all app
  packages (much of `System32`, used to run `cmd`/`whoami`/`powershell`) is reachable by
  any AppContainer by construction; it is not something the per-instance ACLs can revoke.
- **`%USERPROFILE%` path disclosure.** The variable is passed through unredirected (only
  `TEMP`/`APPDATA`/`LOCALAPPDATA` are redirected); it discloses the path, not access.

## Follow-up hardening (round 2, 2026-10-19)

### 4. Hosted servers can now be sandboxed on Windows (per the instance's profile)

Previously the server JVM ran with the full user token on Windows: `host/server.go` and
`host.go`'s `canSandbox` hard-excluded Windows, and `windows.go` refused `r.Headless`. A
malicious server plugin/mod was unconfined — the largest Windows gap.

Now a server is sandboxed exactly when its instance's sandbox is on, the same model as the
game and as Linux. The OS exclusions are gone (`server.go` `spawn`, `host.go` `canSandbox`),
and `windows.go` `wrap()` handles headless by reusing the whole game path (container SID,
`quartz.store` read, folder grants, `sealReadOnly`, TEMP/APPDATA redirect, the kill-on-close
job object) with two differences: it skips the window-station capability (no cursor/window)
and sets network per `networkCaps()`.

**The loopback catch, measured (no admin).** A spike (`TestServerConnectivitySpike`) launched
a headless listener inside a real AppContainer: it **binds fine**, but no same-machine client
can reach it — `127.0.0.1` is blocked (expected) *and* the machine's own LAN IP times out.
AppContainer network isolation blocks all same-host inbound; the only lift is a loopback
exemption, which needs admin. Admin is deliberately not used (running the launcher elevated
would make any AppContainer escape far worse), so a sandboxed Windows server is reachable
from **other devices**, not this PC. `Deps.Sandbox` documents this; it is the user's choice
via the sandbox toggle.

**Three-level network — `networkCaps()` + the `rules.Internet` levels.** The Internet rule now
has three levels, applied identically to a game and a server: **Allowed** = internet + LAN
(Standard); **LAN** = the private-network capability only, so LAN and VPN adapters work but the
internet doesn't (Strict — you can still join or host a LAN/Radmin game); **Blocked** = no
network (a Custom option). Windows enforces all three via capabilities. Linux and macOS can't
allow the LAN while blocking the internet (namespace/Seatbelt are all-or-nothing with no
destination filtering), and a no-network server is useless, so there they treat **LAN as full
network** — the network is removed only at the Blocked level (`linux.go`, `darwin.go`). Net
effect: Standard behaves the same everywhere; under **Strict**, Windows additionally blocks
outbound internet (LAN-only, a free exfil-block it can enforce), while Linux/macOS keep the full
network (they can't express LAN-only, and blocking entirely would make servers unjoinable).
Both stay usable for joining/hosting LAN and VPN games; the only asymmetry is the internet
block, which isn't the primary threat (that's the filesystem wall). Covered by `TestNetworkCaps`
and the presets test.

### 5. Command-settings origin gate

`PreLaunch`/`Wrapper`/`PostExit` run programs (pre/post outside the sandbox as the user;
the wrapper inside it) — so a command introduced by a **foreign or hand-edited `instance.json`**
(a shared instance folder, another app, direct tampering) would run. No importer sets these
today (see below), but as defence in depth they are now authenticated: `Store.SetSettings`
(the only UI write path) stamps an HMAC (`Sig`) over the commands with a per-install key kept
next to the `trash` folder — outside any instance, so a game's sandbox can't read it. `Store.Get`
and `List` (the launch read path) blank the commands when the signature doesn't verify, so a
command this launcher didn't set never runs. The gate binds the instance id (a signed blob
can't be reused under another id) and degrades to prior behaviour if the key is unavailable.
Covered by `TestCommandOriginGate`; see `signCommands`/`verified` in `internal/instance/instance.go`.

## Adjacent surfaces (not the AppContainer, noted for completeness)

- **Imported modpacks/launchers — already mitigated, no action.** Contrary to the round-1
  note, every importer deliberately refuses to read `Wrapper`/`JVMArgs`/`PreLaunch`/`PostExit`
  (package docs in `internal/launchers/launchers.go`, `internal/modpack/pack.go`/`read.go`;
  regression test `launchers_test.go`). The only writer of `CommandSettings` is the UI via
  `api.UpdateSettings → Store.SetSettings`. Finding 5 adds a defensive gate on top of that.
- **Loader installer-jar execution** (`loader/processing.go`) runs third-party installer
  jars, but checksum-pinned, argv-only, with a cleaned environment — sound; its trust
  chains back to the pinned version manifest.
