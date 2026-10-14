# Quartz Client: everything it does

This lists every feature Quartz Client ships, taken from the source code, with links to where each one lives. It describes what the code does today. The original v1 scope is in `../FEATURES.md`.

All paths are relative to this folder (`src/`). Frontend paths start with `frontend/src/`.

---

## Contents
1. [At a glance](#1-at-a-glance)
2. [App shell and navigation](#2-app-shell-and-navigation)
3. [Keyboard and command palette](#3-keyboard-and-command-palette)
4. [First-run setup](#4-first-run-setup)
5. [Home](#5-home)
6. [Library and instances](#6-library-and-instances)
7. [Instance page](#7-instance-page)
8. [Content: mods, resource packs, shaders, data packs](#8-content-mods-resource-packs-shaders-data-packs)
9. [Browse and project pages](#9-browse-and-project-pages)
10. [Modpacks and importing](#10-modpacks-and-importing)
11. [Exporting](#11-exporting)
12. [Launching](#12-launching)
13. [Logs and crashes](#13-logs-and-crashes)
14. [Game sandbox](#14-game-sandbox)
15. [Syncing and shared keybinds](#15-syncing-and-shared-keybinds)
16. [Hosting a local server](#16-hosting-a-local-server)
17. [Worlds, servers and screenshots](#17-worlds-servers-and-screenshots)
18. [Accounts and skins](#18-accounts-and-skins)
19. [Downloads and network](#19-downloads-and-network)
20. [Settings and theming](#20-settings-and-theming)
21. [Platform integration](#21-platform-integration)
22. [Security measures](#22-security-measures)
23. [Performance measures](#23-performance-measures)
24. [Developer tooling](#24-developer-tooling)

---

## 1. At a glance

- **Stack:**
  - The backend is Go and the window is Wails v2. The UI is Svelte 5 + TypeScript.
  - cgo is used only for the native window. Discord, the keychain, the clipboard, SQLite, seccomp and shortcuts are all pure Go or OS calls.
- **Platforms:** Windows, macOS and Linux, on x64 and arm64.
  - Windows and Linux ship as a single binary each.
  - macOS ships as a zipped universal `Quartz.app`.
  - Linux uses the system's WebKitGTK; [build.sh](build.sh) picks the 4.1 tag automatically.
- **Window:** 1200×760 by default, 960×600 minimum, dark by default. [app.go](internal/app/app.go)
- **No telemetry, no launcher account.** About shows "Telemetry: None". [About.svelte](frontend/src/routes/settings/About.svelte)
- **Repo map:**

| Where | What |
|---|---|
| [main.go](main.go), [internal/app/](internal/app/) | Composition root, startup, timers, shutdown |
| [internal/api/](internal/api/) | Facades bound to the UI (one file per area) |
| `internal/<domain>/` | Feature packages (instance, launch, content, modpack, sandbox, …) |
| [internal/core/](internal/core/) | Shared primitives (download, netx, tasks, archive, secrets, …) |
| [frontend/src/routes/](frontend/src/routes/) | Pages |
| [frontend/src/lib/](frontend/src/lib/) | Components, overlays, UI primitives, stores, API client, mocks |
| [tools/](tools/) | Release signing, design parity, wrapper jar build |

---

## 2. App shell and navigation

**Sidebar** ([Sidebar.svelte](frontend/src/lib/components/Sidebar.svelte))
- Brand "Quartz Client", plus a search button that opens the palette and shows its current chord.
- Nav: Home, Library (with instance count), Browse, Skins, Syncing.
- **Recent:**
  - The 4 most recently played instances, with a green dot on running ones.
  - Right-click opens the instance menu. An "All" link goes to the Library.
- **Footer:**
  - Downloads, with a badge counting active jobs, and Settings.
  - Account chip showing the default account's head and type, or "Add account" when there is none.
- Below 1100px the sidebar collapses to 60px of icons. Below 760px the layout stacks. [monolith.css](frontend/src/styles/monolith.css)

**Status bar and tray** ([StatusBar.svelte](frontend/src/lib/components/StatusBar.svelte))
- Shown only while downloads run. It displays the job label, overall % and speed.
- Clicking it opens a tray of jobs with bars. It appears on every page except Downloads.

**Routing** ([router.svelte.ts](frontend/src/lib/router.svelte.ts), [boot.ts](frontend/src/lib/boot.ts))
- Hash routes cover home, library, new instance, import, browse, project, skins, accounts, syncing, downloads, every settings tab, settings search, instance tabs with sub-pages, and setup steps.
- Unknown routes fall back to Home.
- The start page is Home, Library or the last instance, set by the "Open on startup" setting. Unfinished setup always opens `/setup/1`.
- Navigating closes any open dialog.

**Lifecycle** ([app.go](internal/app/app.go), [open.go](internal/app/open.go))
- **Single copy per data folder.** The lock ID is `AppID + sha256(root)[:4]`, so a portable copy can run next to the installed one. A second launch focuses the window and forwards its arguments.
- **When a game starts:** keep the launcher open, hide it (it reappears when every game has exited), or close it (the game keeps running). While downloads run, close hides instead, so they aren't cut off.
- **Start minimized** with `--minimized`, which autostart uses.
- **Self-restart** with `--wait-pid`, used after updates and data-folder moves. [relaunch.go](internal/core/procx/relaunch.go)
- **Background timers:**
  - Launcher update check 30 s after start, then every 12 h.
  - Modpack-update and follow-latest checks every 6 h.
  - A daily purge of the trash and unused store files.
- **Graceful quit:**
  - With downloads unfinished, closing asks first: a toast offers **Quit anyway**, and closing again while it shows quits. The launcher's own quits (restart, update) don't ask. [quit.go](internal/app/quit.go)
  - A hosted server gets up to 35 s to save.
  - Every store is flushed.
  - A downloaded update installs on quit.
- **Recovery at start:**
  - Games the launcher didn't see end (it closed when they started, or crashed) are finished, or adopted while they still run. See [Launching](#12-launching).
  - Orphaned hosted servers are stopped.
- **Launcher crashes:**
  - A fatal crash is written to `logs/launcher-crash.txt`. The next start keeps a redacted copy (the newest 5) and says the launcher closed unexpectedly. [crash.go](internal/core/logging/crash.go)
  - Timers and watchers run each round under `safe.Run`, so a bug in one is logged instead of ending the launcher. [safe.go](internal/core/safe/safe.go)

**Toasts** ([Toast.svelte](frontend/src/lib/overlays/Toast.svelte), [session.svelte.ts](frontend/src/lib/session.svelte.ts))
- One toast at a time. It dismisses itself after 8 s and shows a draining bar. Hover or focus holds it.
- A toast with Undo or an action is never replaced: later toasts wait their turn (up to 3, duplicates dropped). A plain notice gives way at once.
- Tones: trash (with Undo), info, and error (`role=alert`).
- A toast can carry an action button such as Review, Open or Console.

---

## 3. Keyboard and command palette

**Shortcuts** ([shortcuts.ts](frontend/src/lib/shortcuts.ts), [keys.ts](frontend/src/lib/keys.ts), [listkeys.ts](frontend/src/lib/listkeys.ts))

| Action | Default | Scope |
|---|---|---|
| Command palette | Ctrl+K | anywhere |
| Switch instance | Ctrl+P | anywhere |
| Settings | Ctrl+, | anywhere |
| Play | Ctrl+Enter | instance page, not running |
| Stop game | Ctrl+. | instance page, running |
| Open folder | Ctrl+O | instance page |
| Next tab | Ctrl+Tab | cycles the 11 instance tabs |
| Shortcut sheet | ? | anywhere (fixed) |
| Go to Home / Library / Browse / Downloads | G then H / L / B / D (within 800 ms) | anywhere |
| List: move / toggle / search / select all | J K / Space / `/` / Ctrl+A | content lists, update review |

- Cmd counts as Ctrl. Shift counts on letters (Ctrl+Shift+K isn't Ctrl+K).
- Chords follow the physical key when a layout types something else (Cyrillic, Greek, macOS Option), so they work on any layout. G sequences too.
- Ctrl and Alt chords work while you type. While a dialog is open, only the palette and switcher work. Esc closes overlays.
- **Rebinding** lives in Settings → General → Keyboard shortcuts:
  - A chord must use Ctrl or Alt, and a chord that is already taken is rejected.
  - Esc cancels capture, and a reset button appears on changed chords.
  - Up to 16 custom chords are stored and validated in Go.
  - [ShortcutsGroup.svelte](frontend/src/lib/settings/ShortcutsGroup.svelte), [keycapture.ts](frontend/src/lib/keycapture.ts)
- **Shortcut sheet:** four groups (Anywhere, Go to, Instance, Lists) showing the chords in effect. [Shortcuts.svelte](frontend/src/lib/overlays/Shortcuts.svelte)

**Command palette** ([Palette.svelte](frontend/src/lib/overlays/Palette.svelte))
- **Commands mode (Ctrl+K):** fuzzy matching (the query's letters in order), ranked so substrings, word starts and runs come first. Up to 8 results, matched letters highlighted. [fuzzy.ts](frontend/src/lib/fuzzy.ts)
  - Check for updates in all instances. The result toast has a "Review" action.
  - Update N mods in the most recent instance (only when updates are pending).
  - Install launcher update, or Check for launcher updates.
  - On an instance page: Create shortcut…, Export instance….
  - Import from other launchers, Open data folder, Create a new instance, Show keyboard shortcuts.
  - One command for every registry setting, labelled "Settings, &lt;Tab&gt;". Settings are also found by their description and group, below label matches.
- **Instances mode (Ctrl+P):**
  - Up to 5 instances, each with version, loader and "played X ago".
  - Enter plays the selected instance; Ctrl+Enter opens it.
- Tab switches mode. `?` opens the shortcut sheet.

---

## 4. First-run setup

Files: [routes/setup/](frontend/src/routes/setup/), [paths.go](internal/core/paths/paths.go), [api/launchers.go](internal/api/launchers.go)

1. **Welcome:**
   - Language (English only for now).
   - "Where to keep instances": **Change** moves the data folder and restarts. This is disabled when the folder is fixed by a flag, env var or portable mode, or once instances exist.
     - Folders with "!" in the path (it breaks Java) and temporary folders are refused.
     - The move is all or nothing: renames are undone, and copies to another drive are staged until every one succeeds. If it fails, the launcher keeps the old folder and says why. [pointer.go](internal/core/paths/pointer.go)
   - Look: Dark, Light or System.
2. **Sign in:**
   - Microsoft in the browser, a code on another device, or play offline.
   - Setup moves on automatically after an account is added. Skip is allowed.
3. **Bring your instances:**
   - Detected launchers each have a toggle, on by default when the launcher has instances.
   - Choose **Copy them** or **Use them in place**.
   - A card shows which Java runtimes will download in the background, with their size.
   - **Import N and finish**.
- Setup can be run again from Settings → General, and the palette jumps to step 3.
- A fresh install sets the default most memory to two thirds of the RAM, between 1 and 4 GB. [store.go](internal/settings/store.go)

---

## 5. Home

Files: [Home.svelte](frontend/src/routes/Home.svelte), [home.go](internal/home/home.go), [api/home.go](internal/api/home.go)

The page is a single "focus column" that never scrolls. Rows with nothing to show are removed.

- **Alerts** ("Needs attention"), each with one action. [alerts.ts](frontend/src/lib/api/alerts.ts)
  - Each source is read on its own, so one that fails doesn't hide the rest.
  - Several alerts of one kind merge into one ("Survival and 2 more are missing mods"), which acts on the first.
  - Shown most urgent first, at most 3. [HomeAlerts.svelte](frontend/src/lib/components/HomeAlerts.svelte), [api/index.ts](frontend/src/lib/api/index.ts)

  | Alert | Action |
  |---|---|
  | Sign-in expired | **Sign in** |
  | Data folder has "!" in its path, or is in a temporary folder | **Review** (Settings → Storage) |
  | Instance missing a dependency | **Fix** (opens the Mods tab) |
  | Modpack update available | **Review** |
  | Every mod supports the new Minecraft release | **Switch** |
  | Under 5 GB free on the data drive | **Clean up** |

- **Skin card:** the default account's 3D model with its cape, and a "Played this week" bar chart for Monday to Sunday in local time. Sessions that cross midnight count on both days. [SkinCard.svelte](frontend/src/lib/components/SkinCard.svelte)
- **Screenshot rotator:** recent screenshots from every instance.
  - Advances every 5 s and pauses on hover or while the window is hidden. Has dots and arrows.
  - Clicking opens the screenshot in its instance.
  - [ShotRotator.svelte](frontend/src/lib/components/ShotRotator.svelte)
- **News strip:** the latest Java Edition article from Mojang's launcher feed, cached for 1 h.
  - Only https minecraft.net links are accepted, and the image is proxied.
  - Clicking opens the browser. ✕ hides that article for good.
  - [NewsBanner.svelte](frontend/src/lib/components/NewsBanner.svelte)
- **Servers card:** servers from every instance, merged by address.
  - Filters: All or Online. Refresh forces a fresh ping.
  - Each row shows favicon, name, address, version badge, players and ping bars.
  - Split **Join**: the button uses the instance last used on that server; ▾ picks another instance that has it.
  - Online servers sort first, then by recency.
  - [ServerList.svelte](frontend/src/lib/components/ServerList.svelte), [servers/home.go](internal/servers/home.go)

---

## 6. Library and instances

**Library page** ([Library.svelte](frontend/src/routes/Library.svelte))
- Buttons: Import, New instance.
- Search matches names and tags. Filter chips: All, Modded, Modpacks, Vanilla, Servers.
- Sort by Last played, Name or Playtime. Names sort numerically ("Pack 2" before "Pack 10"), and an instance never played counts from when it was made.
- **Cards:** cover, icon, name, version and loader, last played, hours.
  - A hover Play button; if the instance is already running, it opens the menu.
  - Right-click opens the instance menu.

**Instance menu** ([InstanceMenu.svelte](frontend/src/lib/overlays/InstanceMenu.svelte), [instanceActions.ts](frontend/src/lib/instanceActions.ts))
- Actions: Play/Stop, Rename, Change icon, Open folder, Duplicate, Create shortcut…, Export…, Repair game files, Move game folder, Move to trash.
- **Play as:** every account, for a single launch. The default account doesn't change.
- Opens with right-click, Shift+F10 or the Menu key.

**Create an instance** ([NewInstance.svelte](frontend/src/routes/NewInstance.svelte), [api/instances.go](internal/api/instances.go))
- **Name and icon:** the name follows the picked version until you type your own (max 64). The icon opens the icon dialog.
- **Minecraft version:** a two-step picker, family then release, with a Snapshots toggle, the Java version needed and the release date. [VersionPicker.svelte](frontend/src/lib/components/VersionPicker.svelte)
- **Loader:** None, Fabric (default), Quilt, NeoForge or Forge, with the latest stable build shown.
- **Share with other instances:** Keybinds, Game options, Servers, Worlds, Resource packs, Shaders, Screenshots, Hotbars. Keybinds, Servers, Resource packs and Shaders are on by default.
- **More options:** group (or a new group), memory, and sandbox. Sandbox is on by default when the system supports it.
- **Footer:** an estimate of the first download, excluding files already shared. [api/versions.go](internal/api/versions.go)
- Before creating, the launcher checks that the version and loader build exist upstream.
- A name another instance already has gets " (2)", " (3)" … so menus never list two alike.

**Editing and organising** ([instance/edit.go](internal/instance/edit.go), [groups.go](internal/instance/groups.go), [art.go](internal/instance/art.go))
- **Rename:** changes the display name only; the folder stays.
- **Change icon:** 10 colour tints, a 1–2 letter initial, or a picture.
  - Pictures must be PNG or JPEG, at most 8 MB. They are re-encoded to a square PNG of at most 128px and stored by SHA-256.
  - [IconDialog.svelte](frontend/src/lib/overlays/IconDialog.svelte), [icons.go](internal/icons/icons.go)
- **Groups, tags, notes:**
  - An instance's group is set at creation or in its settings, and the list of groups lives in `groups.json`. The Library doesn't group its cards.
  - Up to 12 tags of 24 characters, deduplicated case-insensitively.
  - Notes up to 4000 characters.
- **Playtime and last played** per instance, plus the total across all instances. Seconds below a minute carry over to the next session, so short sessions add up.

**Duplicate** ([instance/duplicate.go](internal/instance/duplicate.go), [api/instances.go](internal/api/instances.go))
- A job makes "&lt;name&gt; (copy)" with the settings, content, worlds, options, modpack record and screenshot details.
- Mods and packs are linked from the store (reflink or hardlink), after a fresh scan so only unchanged files are linked; everything else is copied.
- Left behind: logs, crash reports, caches and other folders the game rebuilds (the same list export uses), launch history, activity and playtime.
- The copy is staged in a hidden folder and appears only once complete; a failure or cancel leaves nothing, and a crash's leftovers are removed at the next start.
- A toast with **Open** says when it's ready. Also in the palette on an instance page.

**Trash, move, repair** ([trash.go](internal/instance/trash.go), [move.go](internal/instance/move.go))
- **Move to trash:**
  - A rename into `trash/`, with a toast offering Undo. Items can be restored for 30 days.
  - Restore re-ids the instance on a name clash and brings its shortcuts back.
  - The Storage page lists trashed instances with Restore and Delete now.
- **Move game folder** to another drive:
  - On the same volume it's a rename.
  - Across volumes it copies through `os.Root`, checks every file's size, and deletes the original only once the instance points at the copy. If saving the new place fails, a rename is undone and a copy is dropped. It runs as a job.
  - Links that pointed into the old folder are repointed into the new one.
  - **Default location** moves the folder back.
  - A moved folder carries a marker. If it's missing (the drive is unplugged), launching, repairing, scanning and modpack updates refuse and say the drive may be disconnected, instead of creating an empty game there.
- **Repair game files** re-hashes every file of the version and re-downloads damaged ones. A toast says so when everything is intact. [launch.go](internal/launch/launch.go)

**Storage model** ([instance.go](internal/instance/instance.go), [effective.go](internal/instance/effective.go))
- Each instance is `instances/<slug>-<4hex>/` containing `instance.json` (schema v2), `game/`, `runs/` and `content.json`. The id never changes.
  - Hand edits are repaired field by field. A field of the wrong type falls back to its default and the rest is kept (a copy goes to `.corrupt`); only broken JSON starts from defaults.
- Instance settings sit on top of the global ones:
  - The Java card overrides as a whole. A new instance starts with it empty and inherits the global values live.
  - Window settings fall back field by field.
  - The minimum heap floor is 256 MB.
  - If auto-pick is off and no Java is chosen, the launch is blocked rather than downloading Java silently.
- A guard refuses file changes (trash, move, duplicate, version change, export) while a game or hosted server is using the instance, or while one of its downloads (install, update, version switch) is still queued, waiting or running. Moving to the trash also releases the folder watch.

---

## 7. Instance page

**Header** ([InstHeader.svelte](frontend/src/lib/components/InstHeader.svelte), [InstanceActions.svelte](frontend/src/lib/components/InstanceActions.svelte))
- **Shows:** breadcrumb, icon, Minecraft version, loader, hours played, and a "Sandboxed" badge.
- **Buttons:** Open folder, a More-actions menu, **Host** (or a live `localhost:<port>` chip that toggles the host panel), Play/Stop.
- **Tabs with counts:** Overview, Mods, Resource packs, Shaders, Data packs, Worlds, Servers, Screenshots, Logs, Version, Settings.
- While the page is open, outside file changes are watched live, including content folders created after the page opened; if the system drops events, everything is rescanned. [content/watch.go](internal/content/watch.go), [gamewatch.go](internal/gamewatch/gamewatch.go)

**Overview** ([Overview.svelte](frontend/src/routes/instance/Overview.svelte), [content/activity.go](internal/content/activity.go))
- **Activity feed:** All, Sessions or Changes.
  - Sessions: played, crashed, failed to start, screenshots taken.
  - Changes: content added, updated or removed.
- **Details:** Minecraft, loader, Java, memory, content counts, created date, folder.
- **Updates:** pending updates with a Review button.
- **Protection and sync:**
  - Sandbox status and the number of sync targets.
  - A per-instance toggle for Discord status.

**Version tab** ([Version.svelte](frontend/src/routes/instance/Version.svelte), [ChangeMinecraft.svelte](frontend/src/routes/instance/ChangeMinecraft.svelte), [ChangeLoader.svelte](frontend/src/routes/instance/ChangeLoader.svelte), [content/switch.go](internal/content/switch.go))
- **Components card:**
  - Minecraft version, with its date and the Java it needs.
  - Loader build, marked Latest stable, Stable or Beta.
  - The Java in use and why. **Manage** opens Settings → Java.
- **Change Minecraft:** uses the version picker and shows "N of M mods have a matching version" before **Switch**.
- **Change loader:** pick a loader and build, with an optional beta toggle, or remove the loader.
- **Switching:**
  - Every identified mod moves to its build for the target.
  - Mods with no build are turned off but stay in the folder. Unknown mods are left alone.
  - The instance itself changes last, so a failed download leaves it untouched.
- **Keep the loader up to date:** before a launch, the loader moves to the newest stable build. Being offline never blocks the launch.
- **Follow the latest Minecraft release:** once every mod supports the newest release, the launcher offers a switch. It only offers; you switch yourself. Checked at start and every 6 h. [follow.go](internal/follow/follow.go)

**Instance settings** ([InstanceSettings.svelte](frontend/src/routes/instance/InstanceSettings.svelte))
- **Java and memory:**
  - An "Override global settings" switch. While it's off, the dimmed rows show the global values the instance inherits; turning it on, or editing a row, starts from them.
  - Java runtime: automatic or any detected runtime.
  - Min and max memory in MB, with "This PC has N GB. X GB suits N mods."
  - JVM arguments.
- **Game window:**
  - Width × height, start fullscreen.
  - When the game starts: keep the launcher open, hide it, or close it.
  - **Open straight into** one of the instance's worlds or servers. [into.go](internal/instance/into.go)
  - **Play as:** the account this instance always launches with (default account or any added one). Right-click Play as still wins for one launch; if the account was removed, the default plays and the launch panel says so.
- **Syncing:** a shared-keybinds toggle and a join/leave toggle for each enabled sync target.
- **Sandbox:** On/Off and the preset, linking to the Sandbox page.
- **Performance:** dedicated GPU (Linux and Windows), Feral GameMode and MangoHud (Linux).
- **About:** group, tags, notes.
- **Advanced:** repair, move the game folder (or move it back to the default), move to trash.

**Commands and variables** ([Commands.svelte](frontend/src/routes/instance/Commands.svelte), [launch/commands.go](internal/launch/commands.go))
- **Commands:**
  - Before launch: must succeed for the game to start.
  - Wrapper: for example `prime-run`.
  - After exit: receives `$EXIT_CODE`. A failure shows an error toast with its last lines.
- **Variables** (Prism-compatible): `$INST_ID $INST_NAME $INST_DIR $INST_MC_DIR $INST_JAVA $INST_MC_VERSION $INST_LOADER`.
- **Environment variables:** an editable table. Variables that inject JVM flags are refused.
- **Native libraries:** use the system GLFW or the system OpenAL, found where the linker looks (`LD_LIBRARY_PATH`, `ld.so.conf` and its includes, NixOS, Flatpak, the usual folders). If one isn't found, the launch panel warns and the game uses its own.

**Sandbox page:** see [§14](#14-game-sandbox).

---

## 8. Content: mods, resource packs, shaders, data packs

**One shared list** ([ContentTab.svelte](frontend/src/lib/components/ContentTab.svelte))
- The Mods, Resource packs, Shaders, Data packs, Worlds and Servers tabs all use the same component: same toolbar, rows, skeleton and empty states.
- **Toolbar:** search (names and file names), filter chips with counts, a sort button, a secondary action and a primary action.

  | Tab | Filter chips |
  |---|---|
  | Mods | All, Updates, Disabled, Client only |
  | Resource packs | All, Enabled, Disabled |
  | Data packs | One chip per world |
  | Worlds | One chip per game mode |
  | Servers | All, Online |

  - Sort: Name (load order for resource packs), Recently added, Size, Enabled first. Each tab keeps its sort while the launcher runs. Dragging resource packs works in load order.
  - Secondary: **Check updates** (or Open folder, or Refresh).
  - Primary: **Add …** opens Browse filtered to this type and instance. Its ▾ (the split button Home's Join and the project page's Install use) offers **From file…** (a native picker, with the same checks as a drop) and, on Mods, **Turn off unused libraries**. Worlds has Import world, Servers has Add server.
- **Rows:**
  - An enable toggle (shaders are exclusive), icon, name, subtitle linking to the project page, version, and a source letter (M, C or F).
  - **Status badges:** Update vX, Locked, Unknown, made for another version, problem (info, warn or danger tone), Active. Problems include a mod for another loader or Minecraft, a dependency missing or of the wrong version, a conflict, a duplicate, an unused library, and "Can't read this file" for a broken or truncated file.
- **Selecting:** the update review's checkbox sits over a row's icon, shown on hover and on every row once something is picked. Shift-click picks a range, Ctrl+A everything shown, Esc clears. A bar at the bottom (the review's footer) offers Turn on, Turn off, Remove (one Undo) and Update, which opens the review with those ticked. [selection.ts](frontend/src/lib/selection.ts)
- **Row menu:** Install &lt;dependency&gt; (on a "Needs X" row: its project is found by mod id and the Install dialog opens), Move up/down (resource packs), Change version…, Lock/Unlock version, Open on Modrinth/CurseForge, Show in folder, Remove (with Undo).
- Turning off or removing mods that other enabled mods need says which ones in the toast.
- **Drag-reorder** for enabled resource packs, with rows shifting live as you drag.
- **Drop files onto the window** to add them to the current tab.
  - Data packs need a world chosen first. Rejected files come back with a reason each.
  - [content/add.go](internal/content/add.go), [api/drops.go](internal/api/drops.go)
- **List keys:** J/K move, Space toggles, `/` searches.

**Enable/disable by kind** ([content/toggle.go](internal/content/toggle.go), [content/config.go](internal/content/config.go))

| Kind | How it's turned on and off |
|---|---|
| Mods, data packs | A `.disabled` suffix. A folder data pack also renames its `pack.mcmeta` (Minecraft loads any folder that has one); one off by name only is fixed at the next scan |
| Resource packs | On/off and order in `options.txt`; `incompatibleResourcePacks` is maintained |
| Shaders | Sets the active pack in the Iris or Oculus properties file, or OptiFine's `optionsshaders.txt` when OptiFine (or OptiFabric) is the shader mod |

- Every edit is key by key; all other lines stay byte for byte.
- **Resource pack list:**
  - Before 1.13 the list holds bare file names; both forms are read, and the right one is written.
  - Built-in packs (vanilla, Programmer Art, mod resources) keep their place just above the file pack they sat on.
  - A list cut off mid-write keeps its complete entries; one that can't be read at all is left alone rather than rewritten without its built-ins.
  - Before the first start, edits go to YOSBR's or Default Options' `options.txt`, so the modpack's defaults still apply.
- Edits made while the game runs are queued and applied when it exits, unless the player changed the same setting in game meanwhile: then the game's choice wins.
- **Version lock** per item is stored in `.pw.toml` (packwiz `pin` and Prism's lock key). Update checks skip locked items.
- **Remove with Undo:** the item goes to a hold bin for 15 minutes, and its store blob is kept a week longer. [holdbin.go](internal/holdbin/holdbin.go), [content/remove.go](internal/content/remove.go)

**Scanning and metadata** ([content/scan.go](internal/content/scan.go), [content/kind.go](internal/content/kind.go), [content/meta/](internal/content/meta/))
- **Incremental scan:**
  - Unchanged files are only stat'ed. New files are hashed after a stability check, so half-written files wait for the next scan.
  - Uses up to min(GOMAXPROCS, 8) workers. Concurrent scans of the same folder are merged.
  - Partial or browser-temp files are skipped. Jars over 1 GB are listed but not parsed.
  - A content folder that is a link elsewhere (Prism's copy with symlinks) is followed: its files are listed and can be toggled, but aren't adopted into the store or removed from here.
  - The `.pw.toml` of a file deleted outside the launcher is removed.
- **Index files:** a per-instance `content.json` and a global `catalog.json` keyed by sha1, which shares parse results across instances. [content/index.go](internal/content/index.go), [content/catalog.go](internal/content/catalog.go)
- **Mod metadata:** `fabric.mod.json`, `quilt.mod.json`, `mods.toml`, `neoforge.mods.toml` and `mcmod.info`.
  - Jars built for several loaders report every loader.
  - Nested jars are read one level deep for the mod ids they provide.
  - A jar with no descriptor (OptiFine, a JarJar wrapper, an old coremod) is described by the first mod nested in it, else its manifest. Installers aren't mods.
  - Malformed TOML is salvaged and JSON is read leniently: a field of the wrong type stays empty and the rest is read.
  - Client/server side, dependencies with their version ranges, and the Minecraft range are read. mcmod.info's `dependencies` only order loading; `requiredMods` (and FML's `required-after:`) are requirements.
  - Mod and pack icons are trimmed of transparent borders and fitted, not cropped.
  - [meta/mod.go](internal/content/meta/mod.go)
- **Pack metadata:** every `pack.mcmeta` generation, including `supported_formats` and 1.21.9's `min_format`/`max_format`, plus `pack.png`.
  - Labels such as "Made for 1.20" come from Prism's format tables.
  - [meta/pack.go](internal/content/meta/pack.go)
- **Live watcher:** fsnotify on the visible instance, debounced to 400 ms.

**Content-addressed store** ([store/store.go](internal/content/store/store.go), [store/gc.go](internal/content/store/gc.go))
- **Layout:** each mod or pack is kept once, at `<sha1[:2]>/<sha1>`, sealed read-only. Files are only ever added or deleted, never edited.
- **Placing files into instances:** reflink, else hardlink, else copy. Never symlinks.
  - Files you drop into a game folder yourself are adopted into the store by hardlink.
  - In-place imports get private clones.
- **Integrity:** a sha1 collision with different content is refused.
- **Cleanup:**
  - Two-phase GC: unused blobs are marked first and deleted after a grace period.
  - References are counted from instance indexes, trashed instances and held removals.
  - Settings → Storage shows the unused size and has a "Clean up" button.

**Identification and packwiz** ([content/identify.go](internal/content/identify.go), [packwiz.go](internal/content/packwiz/packwiz.go))
- **Hash lookup:** manually added files are identified in the background on Modrinth (sha1/sha512) and CurseForge (murmur2). Files nobody knows are asked about again after 24 h.
- **packwiz `.pw.toml`:** identified files get one, with both update sources, both lock keys and Prism's dependency key.
  - Unknown keys survive a rewrite.
  - Writes are atomic and size-capped.

**Dependencies** ([content/deps.go](internal/content/deps.go), [core/version](internal/core/version/version.go))
- Among enabled mods it flags, most serious first:
  - a mod built for another loader ("Made for Forge"; Sinytra Connector and Kilt let Fabric mods count on Forge/NeoForge)
  - missing dependencies
  - conflicts, only when the installed version is in the declared range
  - a dependency of the wrong version ("Needs Lib ≥ 2.0")
  - a mod whose Minecraft range excludes the instance ("Made for 1.20.x")
  - duplicates
  - library mods that nothing uses
- Ranges are Fabric/Quilt predicates or Forge's Maven ranges. A version or range that can't be read with certainty (snapshots, "${version}", Minecraft-prefixed builds) is never held against a mod.
- Well-known dependency ids get friendly names. Home raises an alert for missing dependencies.

**Updates** ([content/updates.go](internal/content/updates.go), [ModUpdates.svelte](frontend/src/routes/instance/ModUpdates.svelte), [ChangelogDialog.svelte](frontend/src/lib/components/ChangelogDialog.svelte))
- **Bulk check** per kind and channel (Release, Beta or Alpha), using Prism's algorithm.
  - Loader chains apply: Quilt also runs Fabric mods, and NeoForge 1.20.1 also runs Forge mods.
  - Required dependencies a new version needs are offered too ("needed by …").
- **Review screen:**
  - A channel switch, which re-checks.
  - A platform that couldn't be reached is named, instead of everything looking up to date.
  - A checkbox for each update, "from → to", and a changelog for every version in between (up to 5).
  - A card of locked mods, each with Unlock.
  - "N selected, about X MB", then **Update N**. Updates CurseForge only serves from its website can be ticked; the job waits for their manual download.
- **Applying** is one job:
  - Downloads are hash-verified. Files over 512 MB are refused, and file names are sanitised.
  - A new file with the old one's name is placed beside it and swapped in with one rename, so a failure keeps the old file.
  - Renames are followed in the resource-pack list and in the active shader and its settings.
- **Check all instances:** one palette command. It checks instances one after another to respect rate limits.

**Installing from Browse** ([content/install.go](internal/content/install.go), [Install.svelte](frontend/src/lib/overlays/Install.svelte))
- **Version choice:** the newest version that fits the instance's Minecraft and loader. Mods can't be installed into an instance without a loader; the dialog says to add one first.
- **Required dependencies** install automatically. The resolver walks breadth-first, up to depth 6 and 64 nodes.
  - A dependency pinned to a version gets that version; one named only by its version is resolved to its project.
  - With no build on the chosen channel, a required dependency falls back to beta, then alpha. With none at all, the dialog lists it as missing.
  - A required dependency that is installed but turned off is turned on by the install.
- **Optional dependencies** are offered as checkboxes.
- **Already installed?** Detected by platform id, by cross-platform slug (Fabric API from CurseForge counts as Fabric API from Modrinth), or by the mod ids jars declare and provide, so a hand-added, unidentified jar or Quilted Fabric API counts too.
- **Dialog:**
  - Target instance; a world picker for data packs.
  - What the install replaces, and the dependencies with their sizes.
  - A memory warning ("X GB suits N mods").

**Server and client split** ([content/server.go](internal/content/server.go))
- Mods are split into server and client-only using the mod's own metadata, Modrinth's `server_side` field, or the packwiz side.
- This feeds Host and the "Client only" chip.

---

## 9. Browse and project pages

**Browse** ([Browse.svelte](frontend/src/routes/Browse.svelte), [browse/search.go](internal/browse/search.go), [browse/categories.go](internal/browse/categories.go))
- **Types:** Mods, Modpacks, Resource packs, Shaders, Data packs. Every type uses the same layout.
- **"Adding to":**
  - Picks the target instance and is remembered for the session.
  - Modpacks have no target: a pack becomes a new instance, so no instance's version, loader or content filters it.
- **Merged search:**
  - Modrinth and CurseForge results become one row per project, matched by normalised title and author.
  - Relevance interleaves both sources by rank. A row never jumps once it's shown.
  - Downloads are summed and the newer update date is shown.
  - When one platform can't be reached, a line says so ("CurseForge couldn't be reached; showing Modrinth only") instead of the results looking complete.
- **Sort:** Relevance, Downloads, Recently updated. Search waits 300 ms after typing stops.
- **Filters pane** (an overlay under 1100px):
  - Version and loader chips taken from the target instance. For modpacks, a Minecraft version picker instead ("Any version" by default).
  - Curated categories mapped to both platforms; a category CurseForge lacks drops CurseForge results. Two or more categories mean all of them on both platforms (CurseForge's "any of" answer is narrowed).
  - **Runs on:** Client side (needed on the client, optional on a server) or Server side (the other way round), Modrinth's side facets as Prism and XMCL use them. CurseForge publishes no sides, so it is left out.
  - Hide AI-generated content (on by default), Hide ads/telemetry, Hide already installed (on by default; not for modpacks).
- **Results:** infinite scroll, loading the next page 400px ahead.
  - **Install** opens the Install dialog. For modpacks it opens Import.
  - A project the target already has says **Installed** instead.

**Project page** ([Project.svelte](frontend/src/routes/Project.svelte), [browse/project.go](internal/browse/project.go), [browse/links.go](internal/browse/links.go))
- **Source switch** (Modrinth or CurseForge): defaults to whichever was updated more recently and says why. Single-platform projects are matched to their twin by title and author.
- **Install:** a split button with an "Install into" instance menu.
- **Description:**
  - Rich text: headings, paragraphs, bulleted and numbered lists, images, code blocks (line breaks kept), links. [docs.go](internal/core/docs/docs.go)
  - Badges (pictures sized 64px or less, or two or more in one paragraph) sit in one row, each still a link.
  - Pictures load from the Modrinth and CurseForge CDNs, imgur and GitHub, always through the launcher's picture proxy. A redirect may only lead to another allowed host. [remoteimg.go](internal/core/remoteimg/remoteimg.go)
  - CurseForge's `linkout?remoteUrl=` wrapper is unwrapped to the real address.
  - Links to a Modrinth or CurseForge project open its page in the launcher; other links open the browser.
  - A side panel showing the installed version in the target instance, with **Update to Y** (only a version the update channel allows).
  - Links, and View on &lt;source&gt;.
- **Versions:**
  - Only-for-this-instance toggle and Release/Beta/Alpha chips. Modpacks list every version, with no instance filter.
  - Table of version, channel, Minecraft, date, size and an action (Install, Update or Installed).
  - Installing an older modpack version imports that version.
  - Clicking a version expands its changelog.
- **Gallery** and **Changelog** tabs.
- **Privacy:** the UI only gets plain data, same-origin pictures and opaque refs. It never gets raw URLs. [browse/browse.go](internal/browse/browse.go), [docs.go](internal/core/docs/docs.go)

---

## 10. Modpacks and importing

**Import page** ([Import.svelte](frontend/src/routes/Import.svelte), [api/imports.go](internal/api/imports.go))
- **Input:** drop a file, choose a file, or paste a link. The link is read 600 ms after typing stops.
- **Preview card:**
  - Name, version, loader, Minecraft, source, mod count and size, overrides.
  - **Already installed:** when an instance came from the same pack, "You have &lt;name&gt;" with **Update it instead**, which opens that instance's update page with this version picked.
  - Warnings:
    - jars in overrides that aren't from the source
    - jars downloaded from outside the source
    - files that need a manual download
    - files CurseForge no longer has, left out
  - **Optional files** (Modrinth `env` optional, CurseForge and FTB optional): one toggle each, off by default as Prism does. The ones picked install turned on.
  - The new instance's name.

**Formats** ([modpack/read.go](internal/modpack/read.go), [modpack/link.go](internal/modpack/link.go), [modpack/folder.go](internal/modpack/folder.go))

| Source | Notes |
|---|---|
| Modrinth `.mrpack` | Downloads only from `cdn.modrinth.com`, `github.com`, `raw.githubusercontent.com` or `gitlab.com` over https; sha1 + sha512 required |
| CurseForge zip | File ids resolved through the API; resource packs and shaders go to their own folders; a file CurseForge removed is left out with a warning; blocked files Modrinth has by sha1 download from Modrinth, the rest go to manual download |
| Prism / MultiMC zip | Game and loader from `mmc-pack.json`; jar mods and LiteLoader are dropped with a warning; the pack Prism installed it from is read; **commands and JVM args are never read** |
| Technic | Zip or Solder; loader found from `bin/` files; md5-checked mod zips |
| FTB | Public API |
| Links | Modrinth or CurseForge page (optionally one version), FTB or Technic page, or a direct https pack (≤1 GB) |

**Install** ([modpack/install.go](internal/modpack/install.go))
- A pack that has been read is staged under a single-use token for 30 minutes.
- Install is one job:
  1. Game and loader.
  2. Pack files: into the store by sha1 when known, otherwise ingested.
  3. Manual downloads.
  4. The instance itself.
- Nothing is left behind on failure or cancel. `modpack.json` records which files the pack placed where.
- The new instance's content is scanned at once, so its counts and lists are right before a tab is opened (also after an update).
- **Safety caps:** 20,000 files, 512 MB per file, 4 GB of overrides, 16 MB manifest.
  - `CleanPath` refuses absolute paths, `..`, drive letters, launcher folders and Windows reserved names.
  - Extraction runs through `os.Root` and never creates links.
  - [modpack/pack.go](internal/modpack/pack.go), [modpack/layers.go](internal/modpack/layers.go)

**Modpack updates** ([modpack/update.go](internal/modpack/update.go), [modpack/swap.go](internal/modpack/swap.go), [ModpackUpdate.svelte](frontend/src/routes/instance/ModpackUpdate.svelte), [backup.go](internal/backup/backup.go))
- **Checks:** Modrinth, CurseForge and FTB packs are checked on the update channel, and feed the Home alert.
  - Only versions for the instance's Minecraft version and loader are offered, so a pack published for several Minecraft versions stays on its line.
- **Any version:** the update page lists every version of the pack (installed, channel, Minecraft). It starts on the update for the instance's own line; when there is none it says so. The player can pick another Minecraft line or go back to an older version. Overview's Details links to it as **Change**.
- **Diff review:**
  - Added, Updated and Removed lists of mods, resource packs and shader packs, paired by project or by file-name stem.
  - Release notes as sentences, including whether the Minecraft version, the loader or the loader's build changes ("Minecraft 1.21.1, Fabric 0.19.3").
- **Three-way merge:**
  - A file you edited is kept ("Your edits — Kept").
  - A mod you removed stays removed.
  - A file the pack dropped is deleted only if you never changed it.
- **All or nothing:** every new file is staged beside its target first, then swapped in. A failure at any point puts every file back and leaves the manifest, version and pack record unchanged.
- **Back up worlds first** is optional and zips every world.
- The update is disabled while the game is running.

**Manual downloads for blocked CurseForge files** ([manual.go](internal/manual/manual.go), [Blocked.svelte](frontend/src/lib/overlays/Blocked.svelte))
- Files Modrinth has by sha1 never get here: they download from Modrinth when the pack is read.
- **Waiting:** the job pauses on a warning step that holds no worker slot. The Downloads page shows a **Download** link for it.
- **Downloads-folder watch:**
  - Finds the folder from XDG user-dirs, the Windows Known Folder, or `~/Downloads`.
  - Takes any file whose size and sha1 match, after a 500 ms settle.
  - Controlled by a setting. Turning it on also checks files already there.
- **Other options:**
  - Drop files onto the window.
  - **Use Modrinth copy:** the same file found on Modrinth by hash.
  - Open all the CurseForge pages.
  - Skip the files.
- Found files are copied into the store, never moved.

**Import from other launchers** ([launchers/](internal/launchers/), [ImportLaunchers.svelte](frontend/src/routes/setup/ImportLaunchers.svelte))
- **Launchers:**
  - Prism (including Flatpak)
  - MultiMC
  - Modrinth App: reads `app.db` with a pure-Go read-only SQLite reader, old and new schemas, custom folder. [sqlitero](internal/core/sqlitero/sqlitero.go)
  - CurseForge app
  - ATLauncher (including `disabledmods/`)
- **Imported:** name, Minecraft, loader, icon, playtime, last played, memory, and the modpack the instance came from: Prism's `ManagedPack*`, the CurseForge app's `projectID`/`fileID`, the Modrinth App's linked project.
- **Never imported:** commands, JVM args or wrappers.
- **Copy or play in place:**
  - Copying clones or copies; it never hardlinks, keeps mtimes, and skips links and devices.
  - In place, nothing in the original folder changes, and the instance is never linked to a pack or updated.
- **Recognising the pack** ([modpack/recognise.go](internal/modpack/recognise.go)):
  - A copied instance with no recorded pack is looked for in the background. Packs named like the instance, or like the name the import read (the player may have renamed it), are searched on Modrinth and CurseForge (top 3).
  - Their versions for the instance's Minecraft version and loader are compared by the sha1 of their mods, walking back while the match rises (at most 5 per pack).
  - At 85% or more, the instance is linked to that pack and version, the activity feed says "Recognised as …", and its update is checked at once so Home can offer it. The outcome is logged.
  - The update baseline is built from that version (also for a recorded link, at the first review), so changes the player made since count as their edits.
- Missing Java runtimes download first.

---

## 11. Exporting

Files: [export.go](internal/exporting/export.go), [list.go](internal/exporting/list.go), [write.go](internal/modpack/write.go), [ExportDialog.svelte](frontend/src/lib/overlays/ExportDialog.svelte)

- **Formats:**
  - **Modrinth (.mrpack):** links files Modrinth serves and bundles the rest.
  - **CurseForge zip:** names every file CurseForge identified by project and file id, with the loader id the app expects (NeoForge 1.20.1 included), the instance's memory as `recommendedRam` and an escaped `modlist.html`. The rest goes in `overrides/`.
  - **Full zip:** everything bundled; Quartz, Prism and the Modrinth App can import it.
  - **Mod list:** Markdown, CSV or HTML.
- **Name, version, summary** (author for CurseForge), remembered per instance. The instance's private notes are never exported.
- **Include toggles:** options and keybinds, server list, mod settings (config), worlds, screenshots. Mods, resource packs and shaders are always included.
- **Linking:**
  - Files nobody identified yet are looked up on both platforms first.
  - A missing sha512 is hashed on the spot.
  - A content folder that is a link (Prism's copy with links) is followed.
- **Sides:** client-only mods get `env` client required, server unsupported. Bundled ones go to `client-overrides/`. Turned-off mods are optional.
- **Live estimate:** "N download from Modrinth (or CurseForge); M bundled", or "N files, X MB".
- **Never exported:**
  - Launcher data: `.quartz`, logs, crash reports, natives, versions, libraries, assets, caches, temp files.
  - Private files: usercache, command history, Realms data.
  - Accounts, settings, `instance.json`, notes and launch history.
- **Hardening:**
  - Reads go through `os.Root`, regular files only.
  - The destination can't be inside the exported folders.
  - Written beside the target, then renamed.
  - CSV formula injection is guarded; Markdown and HTML are escaped.
- Available from the instance menus and the palette.

---

## 12. Launching

**Pipeline and launch panel** ([launch.go](internal/launch/launch.go), [api/launch.go](internal/api/launch.go), [LaunchPanel.svelte](frontend/src/lib/overlays/LaunchPanel.svelte))
- Six ordered steps, each shown with live detail in the panel, with Cancel:
  1. **Sign in:** the Microsoft session is refreshed if needed, with an offline fallback when Microsoft is unreachable.
  2. **Checking mods:** a stat-only scan. It warns about missing dependencies but never blocks, and updates the loader if that's enabled.
  3. **Java:** the instance override, or the right major version picked automatically; a missing runtime is downloaded in the same job. Warns when free RAM is under 70% of the max heap (the game rarely fills it at once), or on macOS when the system reports memory pressure.
  4. **Files:** an install plan with counters such as "2,318 / 3,604". A fully installed version touches no network.
  5. **Sandbox**
  6. **Start**
- **Failures:**
  - No account → Add account (offline tab).
  - Expired sign-in → Add account (browser tab).
  - Anything else → an error toast.

**Version install** ([install.go](internal/install/install.go), [mojang/](internal/mojang/), [meta.go](internal/meta/meta.go))
- **Supported versions:** Minecraft 1.6.1 and newer, with every rule type parsed: OS, arch, version range, features.
- **Install model** (XMCL's): resolve → diagnose → plan → execute. The loader profile is merged over vanilla, so every loader on one version shares the client jar, natives and assets.
- **Checksums:** loader libraries without one get sha1 from Maven sidecar files, so they're verified too.
- **Natives:**
  - Kept in a folder keyed by a hash of the exact native-jar set.
  - Extracted to a new folder and then swapped in.
  - A sandboxed run gets its own private copy.
- **Legacy assets:** the virtual and map_to_resources layouts are rebuilt.
- **Metadata cache:** version JSONs, asset indexes and runtime manifests are pinned by sha1, cached forever and verified. On failure the launcher backs off 1, 3, 9 and 27 s and keeps serving the last good copy.

**Mod loaders** ([loader/](internal/loader/))
- **Supported:** Fabric, Quilt, Forge and NeoForge, straight from upstream with no meta server in between.
  - Fabric and Quilt: [fabric.go](internal/loader/fabric.go). Forge and NeoForge versions: [forge.go](internal/loader/forge.go).
- **Forge:** recommended and latest come from promotions; newer builds are marked beta.
- **NeoForge:** handles both the 1.20.1 artifact and the later one, matching versions numerically.
- **Forge/NeoForge installer:**
  - The installer runs straight from the jar, for modern and legacy profiles.
  - Each processor step is skipped when its pinned outputs already match, and every output is verified.
  - Mojang mappings are downloaded and verified by the launcher itself.
  - Dead legacy Maven hosts are rewritten.
  - [installer.go](internal/loader/installer.go), [processing.go](internal/loader/processing.go)
- **Loader picker:** builds for every loader are fetched in parallel. An unreachable loader site shows no builds instead of failing the picker. [api/versions.go](internal/api/versions.go)

**Java runtimes** ([java/java.go](internal/java/java.go), [detect.go](internal/java/detect.go), [providers.go](internal/java/providers.go), [Java.svelte](frontend/src/routes/settings/Java.svelte))
- **Detection:**
  - Linux: `/usr/lib/jvm`, `/opt`, Snap.
  - macOS: JavaVirtualMachines and Homebrew.
  - Windows: Program Files vendor folders and the registry (64- and 32-bit views).
  - Everywhere: SDKMAN, asdf, `~/.jdks`, Gradle, `JAVA_HOME`, `PATH`, and the official launcher's runtimes.
- **Probing:** through the wrapper jar, cached by size and mtime. 4 probes at once, 15 s timeout.
- **Auto-pick order:** the instance override, then a managed runtime of the exact major, then a detected one for the host's arch, otherwise a download.
- **A chosen runtime is checked:** an instance override, or the default runtime with auto-pick off, that is older than the Minecraft version needs stops the launch with what to change. A newer one on Forge for a Java 8 version only warns.
- **Downloads:** Eclipse Temurin, Azul Zulu or Mojang (Java 8, 16, 17, 21, 25).
  - The setting picks the first vendor; the others are fallbacks.
  - Mojang files prefer the lzma copy and are verified after decoding.
- **Settings → Java:**
  - Each runtime shows its source badge (Managed, System, SDKMAN, Found), a Default badge, and how many instances use it.
  - Scan again, Add from folder, Download.
  - Per-runtime: Show in folder, Use for every instance, Remove (managed and unused only) or Forget.
- **Env cleaning:** `JAVA_TOOL_OPTIONS`, `_JAVA_OPTIONS`, `JDK_JAVA_OPTIONS`, `CLASSPATH` and similar are stripped from the game and from probes. [java/env.go](internal/java/env.go)

**Starting the game** ([run.go](internal/launch/run.go), [wrapperjar.go](internal/wrapperjar/wrapperjar.go), [commands.go](internal/launch/commands.go))
- **Token handling:** game arguments, including the access token, go over stdin to an embedded wrapper jar. They never appear on argv.
  - The jar is pinned by SHA-256 and reinstalled if it is tampered with.
  - The saved `cmd.txt` is redacted.
- **JVM arguments:**
  - `-Xms`/`-Xmx` and the user's JVM args, split shell-style without a shell.
  - `-Dlog4j2.formatMsgNoLookups=true` is always set (Log4Shell), and so is a UTF-8 console (`stdout.encoding`, `stderr.encoding` and their pre-18 `sun.*` names), so text isn't garbled on Windows code pages. Games and hosted servers share this list. [java/env.go](internal/java/env.go)
  - LWJGL 2 versions on Linux get `-DLWJGL_DISABLE_XRANDR=true` when there's no `xrandr` program, which they'd crash without.
  - The log4j config file.
  - Forge's module exports.
  - System GLFW and OpenAL paths, if chosen.
- **Quick Play** (1.20+) opens straight into a world or server. Older versions fall back to `--server`/`--port`. Fullscreen and custom size are supported.
- **Dedicated GPU** ([sysinfo/gpu_linux.go](internal/core/sysinfo/gpu_linux.go), [gpu_windows.go](internal/core/sysinfo/gpu_windows.go)):
  - **Linux:** the card is picked as Pandora does: switcheroo-control's udev tag, else the driver (NVIDIA's and nouveau are dedicated, Intel's integrated GPU sits at `00:02.0`, amdgpu's own flag says whether it's fused into the processor), then which one drives the display.
  - NVIDIA's driver gets its PRIME offload variables; others get `DRI_PRIME=pci-…` naming the exact card. Both get the matching `VK_LOADER_DRIVERS_SELECT`.
  - Nothing switches when there's one GPU or the dedicated one already drives the display. Without sysfs it falls back to `DRI_PRIME=1`.
  - **Windows:** the game's Java is set to High performance in Windows' graphics preferences, unless the player chose already.
- **Linux performance:** `gamemoderun` and `mangohud`.
- **Player commands:** run as argv, never through a shell, with a 10-minute timeout. A failure shows the last lines of output.
- **Stop:** a gentle quit first (SIGTERM or WM_CLOSE), then a kill after 15 s, so the singleplayer world can save, or at once on a second Stop. The game survives the launcher closing. [procx/](internal/core/procx/)

**Multiple copies and run views** ([runview.go](internal/runview/runview.go), [merge.go](internal/runview/merge.go), [recover.go](internal/runview/recover.go))
- **Several at once:** several instances, or the same instance more than once (from Play as), can run together.
- **Run views:** each run sees its own assembled view of the instance.
  - **Linux:** shared folders are bind-mounted, and a concurrent copy gets an overlayfs.
  - **macOS:** APFS clones.
  - **Windows:** junctions, plus a ReFS block clone or a copy for a concurrent copy.
  - Never symlinks.
- **Merge-back:**
  - Worlds merge as a unit. A world both copies changed is kept as "World (copy 2)", with a toast.
  - Other files merge newest-wins. A deletion applies only if nobody changed the file meanwhile.
- **Recovery:** view state is saved, so views left by a crash are merged back on the next start.

**Games that outlive the launcher** ([adopt.go](internal/launch/adopt.go), [procx/ident.go](internal/core/procx/ident.go))
- **Why:** "When a game starts: close" quits the launcher on purpose, and a launcher can crash. Without this, the run's option changes were overwritten by the shared ones on the next launch, its record and playtime were lost, and nothing counted it as running.
- **Pending file:** each running game leaves `<runs>/<id>.pending.json` with its record, its process (pid plus start time, so a reused pid never counts), its view and its sync session. A normal exit removes it.
- **Next start:**
  - **Game gone:** the run is finished at once: merged back, synced files and options copied out, finishers run, record saved, playtime added. The end time is the last write to `logs/latest.log`.
  - **Game still running:** it is adopted. It counts as running, so every guard works and its shared folders stay leased; Stop works; it is finished when it exits (checked every 2 s). Its console isn't available and its after-exit command doesn't run.

**Run history** ([runs.go](internal/launch/runs.go))
- **Each run records:** start, length, exit code, result, Java, memory, account, mod count, sandbox, world, server, screenshots and crash summary.
- **Limits:** 200 records. The newest 50 keep their evidence: log, redacted command and redacted crash report.
- **On exit:** playtime is updated, and the world played and screenshots taken are recorded.

---

## 13. Logs and crashes

**Live console** ([LogsLive.svelte](frontend/src/routes/instance/LogsLive.svelte), [gamelog/](internal/gamelog/))
- **Buffer:** keeps the last 20,000 lines, sent in 100 ms batches. Follows the newest run, or the hosted server.
- **Reading:** a line over 64 KB is cut, and the output is always read to its end, so a mod printing a huge line can't stall the game on a full pipe. The run's log is written per batch, not per line.
- **Parsing:** log4j XML events, plain lines and legacy formats. Malformed input never crashes it.
- **View:**
  - Level chips: Info, Warn (count), Error (count), Debug (off by default).
  - Search with highlighting.
  - Wrap mode shows the last 2,000 lines; unwrapped mode is virtualised.
  - Auto-scroll while you're at the bottom.
- **Actions:** Copy the filtered view, Upload to mclo.gs.
- Secrets are redacted and marked.

**Log files** ([LogFiles.svelte](frontend/src/routes/instance/LogFiles.svelte), [logfiles.go](internal/logfiles/logfiles.go))
- **Lists:** `latest.log` first, then dated logs and crash reports. Gzipped logs are decompressed and viewing is capped at 8 MB.
- **Viewer:** find with highlighting, coloured WARN/ERROR lines, Copy, Upload.
- **Delete old logs:** removes compressed logs older than 30 days, per instance or everywhere, with a size preview.

**Launch history** ([LogHistory.svelte](frontend/src/routes/instance/LogHistory.svelte))
- **Table:** started, length, exit code, and result (Closed, Crashed, Stopped or Failed to start). It opens on the latest crash.
- **Details:** Java, memory, account, sandbox, mods loaded.
- **Saved with each run:** `latest.log`, `crash-report.txt` and `launch-command.txt`, each with Open.
- **Actions:** Copy debug info, Upload logs.

**Uploads and debug info** ([logfiles.go](internal/logfiles/logfiles.go), [redact.go](internal/core/logging/redact.go))
- **mclo.gs upload:**
  - Removes tokens, emails, the home user name and every account's player name first.
  - Over 25,000 lines (or 10 MB), the first 10,000 and the last 14,900 lines are kept with the cut marked, so both the versions and mod list and the crash survive.
  - A run's upload starts with its debug summary.
- **Copy debug info:** a redacted summary of the launcher, system, instance, run and mods.

**Crash dialog** ([Crash.svelte](frontend/src/lib/overlays/Crash.svelte), [crash.go](internal/crash/crash.go), [rules.go](internal/crash/rules.go))
- **When:** opens automatically when a game crashes or fails to start. A crash report written during the run makes it a crash even when the game then exited with 0 (Forge's loading-error screen does), unless the player stopped it.
- **Evidence:** the crash report, or else the JVM's own crash log (`hs_err_pid*.log`) cut before its environment section, both redacted.
- **Likely cause:** found by an ordered rule table over the crash report (≤2 MB) and the end of the log (≤1 MB):
  - mixin failure (3 variants)
  - missing dependency (Forge, NeoForge, Fabric), or one of the wrong version (Fabric: "needs Architectury 13.0.6 or later")
  - a mod for another Minecraft version (Fabric)
  - two mods that say they can't run together (Fabric)
  - duplicate mods
  - a mod named in the crash (Forge MOD section, Fabric "Suspected Mods")
  - Java refusing to start: the heap doesn't fit (a 32-bit Java), or an argument it doesn't know
  - Java too new for an old loader (ASM's "Unsupported class file major version", old ModLauncher's class loader cast)
  - wrong Java (class version)
  - out of memory (heap or native)
  - a native crash: a graphics driver's library (nvoglv64, atio6axx, ig*icd, Mesa's drivers…) is the graphics cause; any other library is named
  - graphics driver
- **One-click fixes:** **Update and relaunch** or **Disable it and relaunch**. The culprit mod is shown with its name, version and icon.
- **Trace:** limited to 40 lines (exception, Caused-by chain, mod lines; for a native crash, its signal and crashing frame). Obfuscated class names show as "a game class".
- **Also:** open the crash report, copy debug info, upload, and launch again.

---

## 14. Game sandbox

Files: [sandbox.go](internal/sandbox/sandbox.go), [rules/rules.go](internal/sandbox/rules/rules.go), [Sandbox.svelte](frontend/src/routes/instance/Sandbox.svelte), [api/sandbox.go](internal/api/sandbox.go)

- **Per-instance isolation,** on by default for new instances when the system supports it.
- **Fails closed:** if the sandbox can't start, the instance doesn't launch, and the launcher says why. The page shows which tool is in use, or why none is available.
- **Presets:** Standard, Strict, Custom.

| Rule | Standard | Strict | Choices |
|---|---|---|---|
| This instance | Read and write | Read and write | fixed |
| Mods | Read only | Read only | fixed |
| Shared libraries and assets | Read only | Read only | fixed |
| Java runtime | Read only | Read only | fixed |
| Synced folders | Read and write | Read only | rw / ro / blocked |
| Internet | Allowed | Blocked | on / off |
| Discord status | Allowed | Blocked | on / off (Windows: blocked only) |
| Graphics and audio | Allowed | Allowed | fixed |
| Your home folder | Blocked | Blocked | blocked / ro (Windows: blocked only) |
| Extra folders | – | – | up to 32, rw / ro / remove |

- **Read only or blocked on folders the game writes to:**
  - "Read only": the game still writes, but into a throwaway layer that is dropped on exit.
  - "Blocked": the game sees an empty throwaway folder.
- **Mods read only:** a mod's file is its store blob, hardlinked into every instance that uses it where the disk can't clone, so a game that could write `mods/` would change it for all of them, unsandboxed ones too. Writes there go to a throwaway layer (Linux with overlays; a mod's own cache still works) or fail. Resource and shader packs hold no code and stay writable, since in-game drag-drop adds packs there.
- **Skins:** before 1.21.4 the game saves a skin under the assets and reads it back; each instance gets its own skin cache there (mounted on Linux; elsewhere `assets/skins` itself is writable).
- **Extra folders** are added through a native dialog only. System roots, the launcher's data, and the home folder (or anything containing it) are refused.

**Linux: bubblewrap + seccomp** ([linux.go](internal/sandbox/linux.go), [seccomp.go](internal/sandbox/seccomp/seccomp.go), [bwrap.go](internal/core/bwrap/bwrap.go))
- **Namespaces:** user, pid, uts, cgroup and net (when internet is off). With `--as-pid-1`, leftover processes die with the game. Uses `--disable-userns` when bubblewrap supports it.
- **Filesystem:**
  - A private `/tmp` and `/dev/shm`.
  - Read-only system folders, including NixOS's and Guix's stores and `/run` links, and selected `/etc` files.
  - A made-up passwd and group that name only the player.
  - The home folder is an empty tmpfs or read-only.
  - The launcher's data folder is hidden, then only the allowed parts are bound back in.
- **Desktop access:** GPU, display, audio, and Discord IPC (native, Flatpak, Snap, Vesktop).
- **Isolation details:** per-run XDG and shader-cache folders. The D-Bus session is pointed nowhere.
- **GameMode** is registered from outside the sandbox by pid. [gamemode_linux.go](internal/sandbox/gamemode_linux.go)
- **Seccomp filter:**
  - Classic BPF generated in pure Go (no libseccomp), based on Flatpak's blocklist.
  - Blocks ptrace, unshare/setns, the mount family, chroot, bpf, perf, userfaultfd, keyrings, kexec, modules, io_uring and `process_vm_*`.
  - Blocks namespace clone flags and the TIOCSTI/TIOCLINUX terminal-injection ioctls.
  - Limits sockets to UNIX, INET, INET6 and NETLINK.
  - Supports amd64 and arm64, and loads from a sealed memfd.

**macOS: Seatbelt** ([darwin.go](internal/sandbox/darwin.go), [seatbelt.go](internal/sandbox/seatbelt.go))
- A deny-by-default `sandbox-exec` profile adapted from Pandora.
- **Allowed:** graphics, fonts, input, audio and the window server. Only the listed paths are reachable.
- **Network:** included only when the rule allows it.
- **Blocked:** CUPS, and LaunchServices opens and Apple events (both in Pandora's profile), so a mod can't have another app run a script it wrote.

**Windows: AppContainer** ([windows.go](internal/sandbox/windows.go), [contain_windows.go](internal/core/procx/contain_windows.go))
- **Container:** one per instance.
- **Folder access:** granted through custom capability SIDs that the token carries only while the rule allows them, so stale ACL entries reach nobody.
- **Internet:** the capability is added only when allowed.
- **Per-run folders:** temp and app-data folders for each run.
- **Mods:** an inherited deny entry for the container on `mods/`, ahead of the game folder's grant.
- **Mouse:** a capability of Quartz's own may set window-station attributes, so the game can confine and move the cursor.
- **Cleanup:** the container is deleted when the instance is purged.

**Env allowlist** ([env.go](internal/sandbox/env.go))
- Only display, audio, locale, system and GPU/overlay variables pass (including the cursor theme and size, `GBM_BACKEND` and the `__EGL_`, `__VK_` and `LIBVA_` families).
- `LD_LIBRARY_PATH` passes, and its folders outside the home and data folders are bound read-only.
- Agent sockets, the session bus and tokens stay out.

**Hosted servers** run headless inside the instance's sandbox on Linux and macOS.

---

## 15. Syncing and shared keybinds

Files: [Syncing.svelte](frontend/src/routes/Syncing.svelte), [syncing/service.go](internal/syncing/service.go), [catalog.go](internal/syncing/catalog.go), [api/sync.go](internal/api/sync.go)

**Model**
- **Shared copy:** one real copy of each shared item lives in `<data>/shared/`.
- **Two switches:** the Syncing page turns a target on, and each instance joins the targets it wants (from its settings or at creation).
- **Folders** are assembled into each run by the run view: bind mount, clone or junction.
- **Files** are copied in before launch and out after exit. Newest wins, and they are never linked. [syncing/files.go](internal/syncing/files.go), [session.go](internal/syncing/session.go)
  - A server list the launcher edits goes to the shared copy straight away.
  - The session is saved with the run, so a launcher that closed while the game ran still copies the run's changes out.

**Targets**
- **Files** ("Newest copy wins"): Game options (`options.txt`), Server list, Command history, Hotbars.
- **Folders** ("Linked into each instance"): Worlds, Configs, Screenshots, Resource packs, Shader packs, Data packs, Server resource packs (`downloads/`).
- **Mod data** (greyed out until some instance has the mod): Xaero's Minimap/World Map, JourneyMap, Distant Horizons, Voxy, Litematica schematics, Flashback, Bobby.
- **Custom paths:**
  - Up to 64 relative paths, added as a file or a folder.
  - Reserved folders (mods, logs, versions, libraries, …) are refused. [settings/syncing.go](internal/settings/syncing.go)
- **Row details:** each row shows how many instances use it. **Open synced folder** opens the shared copy.

**`options.txt` merge** ([syncing/options.go](internal/syncing/options.go), [keycodes.go](internal/core/keycodes/keycodes.go))
- **Merge rule:** key by key. Only keys a run changed are written back, so the newest value of each key wins.
- **Kept per instance:** `version`, `resourcePacks` and `incompatibleResourcePacks`.
- **Format conversion:**
  - Keybinds across 1.13 (LWJGL2 codes ↔ GLFW names).
  - Language across 1.11 (`en_US` ↔ `en_us`).

**Shared keybinds** ([syncing/keys.go](internal/syncing/keys.go))
- **Choosing keys:** **Add key** opens a menu of every key found in your instances, grouped by game or mod category.
  - Names come from the game's and mods' `en_us` lang files (JSON and legacy `.lang`). Mod keys show as "Name (Mod)".
- **Binding:** click the key button, then press any key or mouse button.
- **On launch:** only the chosen keys (up to 128) are written into each opted-in instance's `options.txt`.
  - Keys a version doesn't have are skipped.
  - The rest of the file is left alone.
- **Per instance:** a switch for whether it uses shared keybinds.

**Join, leave and conflicts** ([syncing/items.go](internal/syncing/items.go))
- **Joining** moves the instance's own items into the shared copy, so nothing disappears.
  - On a name clash, both are kept, the newcomer as "Name (2)". World titles are renamed to match.
  - Identical items (by size + SHA-1, or by folder manifest) are merged once.
- **Leaving** gives the instance its own copy back.
- **Turning a target off** makes every instance leave and moves the shared copy to the trash for 30 days.
- **Busy guard:** edits are refused while any instance sharing the files is running. Big moves run as jobs.

---

## 16. Hosting a local server

Files: [host/host.go](internal/host/host.go), [host/server.go](internal/host/server.go), [Host.svelte](frontend/src/lib/overlays/Host.svelte), [HostPanel.svelte](frontend/src/lib/overlays/HostPanel.svelte), [install/server.go](internal/install/server.go)

**Basics**
- **What it does:** the **Host** button runs the instance as a dedicated server on this computer, with the same version and loader. One server at a time.
- **Where it lives:** `instances/<id>/server/`, outside the game folder.

**Host dialog** (settings are remembered per instance)
- **World:** pick an existing one (played in place, including shared worlds) or "New world".
- **Port:** 1024–65535, default 25565. A busy port is refused.
- **Max players:** 1–999, default 10.
- **Memory:** 512 MB up to the machine's RAM.
- **Show on your local network:**
  - Off: the server listens on localhost only.
  - On: it is announced on multicast `224.0.2.60:4445` every 1.5 s, just like "Open to LAN". [lan.go](internal/host/lan.go)
- **Allow offline accounts:** sets `online-mode=false` and `enforce-secure-profile=false`.
- **Mods summary:** "N of M also run on servers; K client-only left out", with **Review**, which opens Mods on the "Client only" chip.

**Preparing the server**
- **Install:** the server jar and loader are installed as a job, with the right Java. Fabric and Quilt use their server launch class. Forge-style loaders use argument files pointed at the shared library store. [loader/server.go](internal/loader/server.go)
- **Mods:** only server-capable mods go in, and the server's `mods/` folder is made to match exactly.
- **Configs:** `config`, `defaultconfigs`, `kubejs` and `scripts` are copied one way; newer files win.
- **`server.properties`:**
  - Your own edits and comments are kept, and values are escaped properly.
  - **The world's own rules:** for an existing world, difficulty, hardcore and the game mode for new players come from its level.dat on every start, as Open to LAN does. Otherwise the server forces its own difficulty onto the world. Servers before 1.14 get numbers. [rules.go](internal/host/rules.go)
  - `spawn-protection=0` unless you set it.
  - RCON and query are forced off. The MOTD is the instance name.
  - [props.go](internal/host/props.go)
- **EULA:** accepted for you on Start, with a comment saying so.
- **Cheats:** a world with cheats on makes the host an operator (`ops.json`, or `ops.txt` before 1.7.6). Entries already there are kept.
- **World backup:** the world is zipped before hosting, keeping the 5 newest. [host/backup.go](internal/host/backup.go)
- **Inventory carry:** your singleplayer player moves into `playerdata/<uuid>.dat` for the server and back afterwards (1.9+). [carry.go](internal/host/carry.go)

**While running**
- **Header:** a `localhost:<port>` chip toggles the panel.
- **Panel:**
  - Player count, and localhost and LAN addresses with Copy.
  - The last console lines and a `/` command box (≤256 characters).
  - Join, Console, Stop.
- **Console:** 20k lines. "Done (…)!" marks the server ready. Join and leave lines trigger a ping for the player count.
- **Stop:** sends `stop`, then kills after 30 s or on a second Stop. Quitting the launcher stops the server gracefully, after it saves.
- **Recover:** a server left running by a dead launcher is asked to stop (then killed) only if its pid still has the start time recorded with it, so a reused pid is never hit. This works on every OS; old pid files fall back to the server folder check (Linux). [procx/ident.go](internal/core/procx/ident.go)

---

## 17. Worlds, servers and screenshots

**Worlds** ([worlds.go](internal/worlds/worlds.go), [api/worlds.go](internal/api/worlds.go), [CopyWorld.svelte](frontend/src/lib/overlays/CopyWorld.svelte), [WorldEdit.svelte](frontend/src/lib/overlays/WorldEdit.svelte))
- **List:** every folder with a `level.dat`, newest played first.
  - Read from NBT, falling back to `level.dat_old`: name, game mode, version, last played.
  - Shows the world icon and the folder size.
- **Play** opens straight into the world.
- **Rename** changes `LevelName` only; the folder stays.
- **Copy seed** returns it as text, since seeds don't fit a JS number. Newer versions keep it in `data/minecraft/world_gen_settings.dat`, which is read when level.dat has none.
- **Import** from a zip or a folder, by picker or drag-and-drop:
  - `level.dat` can be up to two folders deep.
  - Files are staged first, so a failed import leaves nothing behind.
  - Name clashes become "(2)".
- **Export as .zip.**
- **Copy to another instance** (the copy keeps its name).
- **Delete with Undo,** Show in folder, Open the saves folder.
- **Guard:** changes are refused while the world is in use.
  - Rename, delete, copy and export also check `session.lock`, which the game locks while it plays (from 1.16), so a world open in another launcher's game is left alone too. Hosting and quick play into such a world are refused. [fsx/locked_unix.go](internal/core/fsx/locked_unix.go), [locked_windows.go](internal/core/fsx/locked_windows.go)

**Servers** ([servers.go](internal/servers/servers.go), [slp.go](internal/core/slp/slp.go), [ServerDialog.svelte](frontend/src/lib/overlays/ServerDialog.svelte))
- **Editing `servers.dat`:** add, edit, reorder, remove with Undo, copy address, join.
  - Every NBT tag the game wrote is kept.
  - Server resource packs can be set to Ask, Always use or Never use.
- **Add/Edit dialog:** pings the address live 500 ms after you stop typing.
- **Server List Ping:**
  - Follows SRV records.
  - Reports ping, players, a clean version label ("Paper 1.21.4" → "1.21.4"), and "Needs X" when the protocol doesn't match the instance.
  - Favicons are re-encoded and stored by hash.
- **Limits:** results are cached for 60 s, with at most 8 pings at once. [servers/status.go](internal/servers/status.go), [favicons.go](internal/servers/favicons.go)

**Screenshots** ([screenshots.go](internal/screenshots/screenshots.go), [Screenshots.svelte](frontend/src/routes/instance/Screenshots.svelte), [ScreenshotViewer.svelte](frontend/src/routes/instance/ScreenshotViewer.svelte))
- **Grid:**
  - Search by day or world. Chips: All, This week, and one per world.
  - **Select** mode to move several screenshots to the trash at once (with Undo).
  - Open folder.
- **World tagging:** each screenshot is filed under the world played when it was taken. Instances sharing a screenshots folder see each other's tags.
- **Viewer:**
  - Prev/next (also ← →) and a filmstrip.
  - Details: file, date, pixel size, file size, world.
  - Actions:
    - **Copy image** to the clipboard, without cgo: user32 on Windows, osascript on macOS, wl-copy or xclip on Linux. [clipimg/](internal/core/clipimg/)
    - **Use as instance cover**
    - **Show in folder** (on Linux it selects the file over D-Bus)
    - **Move to trash**

---

## 18. Accounts and skins

**Microsoft sign-in** ([msa/browser.go](internal/auth/msa/browser.go), [msa/device.go](internal/auth/msa/device.go), [xbox.go](internal/auth/xbox/xbox.go), [mcapi.go](internal/mcapi/mcapi.go), [AddAccount.svelte](frontend/src/lib/overlays/AddAccount.svelte))
- **Browser:** auth code + PKCE with a loopback redirect on `127.0.0.1` and `[::1]`.
  - Random state, constant-time state check, `prompt=select_account`, 10-minute timeout.
  - The local callback answers only `GET /`, so other local software can't probe or finish the flow.
  - The reply page in the browser is static and script-free with a strict CSP.
- **Code:** device code with a QR code generated in Go. The QR pre-fills the code at microsoft.com/link. Copy code, with an expiry countdown.
- **Token chain:** MSA → Xbox → XSTS → Minecraft → profile, with each step shown. Entitlements are checked only when there's no profile, so Game Pass accounts work.
- **Clear error messages** with a link to the fix: banned, parental consent, no Xbox profile, Xbox terms, region, age verification, child account. [auth.go](internal/auth/auth.go)
- **One sign-in at a time:** starting a new one cancels the previous.

**Accounts** ([Accounts.svelte](frontend/src/routes/Accounts.svelte), [accounts.go](internal/accounts/accounts.go), [manager.go](internal/accounts/manager.go), [refresher.go](internal/accounts/refresher.go))
- **Account types:**
  - Microsoft.
  - Offline: 3–16 characters of `A-Z 0-9 _`, with a Java-compatible offline UUID. Works without owning the game.
- **Default account:** set with Make default. The first account added becomes the default, and removing it promotes the next.
- **Play as:** another account for one launch.
- **Tokens:**
  - Stored only in the OS keychain; `accounts.json` holds no tokens.
  - Every token read is registered with the log redactor.
- **Background refresh:**
  - The default account goes first, then one account every 20 s.
  - Tokens are renewed 12 h before they expire. Accounts a running game uses are never refreshed.
- **Play offline when sign-in fails:** if Microsoft is unreachable, the account plays offline under its own name.
- **"Sign in again"** appears when:
  - a refresh is refused,
  - the keychain lost the token,
  - the client ID changed,
  - or the refreshed profile is a different player.
- **Account rows:** status (Signed in, Needs sign-in or Offline) and "refreshed automatically X ago".
- **Remove:** with an inline confirm.

**Skins and capes** ([Skins.svelte](frontend/src/routes/Skins.svelte), [skins/library.go](internal/skins/library.go), [canon.go](internal/skins/canon.go), [look.go](internal/accounts/look.go), [PlayerModel.svelte](frontend/src/lib/components/PlayerModel.svelte))
- **Local library:** skins are stored by the SHA-256 of a canonical PNG.
- **Adding skins:** upload a file, from an https link, or copy another player's current skin by name.
- **Canonicalising:**
  - At most 256 KB, 64×64 or 64×32.
  - Legacy 64×32 skins are upgraded the way Minecraft does it.
  - Slim arms are detected automatically.
- **Managing:** rename, Classic or Slim, Default skin, remove with Undo.
- **Capes:** "No cape" plus every cape the account owns.
- **Wear this skin and cape:**
  - Microsoft accounts: a real upload, cape change or reset.
  - Offline accounts: cosmetic, in the launcher only.
- **Import of current skins:** the skin each account currently wears is added to the library automatically.
- **3D preview:** CSS 3D, no WebGL, with overlays.
  - Drag to rotate; Rotate left/right, Walk, Front and Back buttons.
  - Scales with the window height.

---

## 19. Downloads and network

**Downloads page** ([Downloads.svelte](frontend/src/routes/Downloads.svelte), [tasks.svelte.ts](frontend/src/lib/tasks.svelte.ts), [api/downloads.go](internal/api/downloads.go))
- **Tabs:** Active, Finished and Failed, keeping 24 h of history.
- **Active tab:**
  - A summary of count, speed, ETA and MB done, with Pause all/Resume all and Cancel all.
  - Each job expands into its steps, each with a bar and "x of y". Pause/Resume and Cancel per job.
  - A warning step links to the manual-download dialog.
- **Finished tab:** Clear.
- **Failed tab:** Retry all, or per job, with the error text.

**Job engine** ([tasks/engine.go](internal/core/tasks/engine.go), [job.go](internal/core/tasks/job.go), [history.go](internal/core/tasks/history.go))
- **Jobs:**
  - A job is a list of steps that run in parallel or wait at barriers.
  - One global worker pool, sized by "Parallel downloads" (1–16, default 8), runs the oldest job first. The size changes live.
  - Work that is already verified is dropped when the job is submitted.
- **Retries:** backoff of 1, 4 and 15 s with ±25% jitter.
- **Pause:** keeps partial files so they resume with HTTP Range.
- **Waiting on you:** work that waits for the user (manual downloads) holds no worker slot.
- **UI updates:** snapshots at most every 100 ms and nothing while idle. Speed is a moving average.
- **History:** persisted for 24 h (up to 500 jobs), so failed jobs can be retried after a restart.

**Downloads** ([download.go](internal/core/download/download.go), [verified.go](internal/core/download/verified.go))
- **Writing:**
  - Streamed to `.part`, hashed on the way, verified, then atomically renamed.
  - Resumable with Range and If-Range.
  - LZMA decoding.
- **Validation:** https only, no userinfo, the destination must stay inside its root, at most 2 GB.
- **Sharing:** concurrent fetches of the same file share one transfer. An identical file found in the store is reused by hardlink.
- **Verified index:** a file that passed verification is trusted after a stat, so thousands of game files check in milliseconds.

**HTTP layer** ([netx.go](internal/core/netx/netx.go), [httpcache.go](internal/core/httpcache/httpcache.go), [network.go](internal/network/network.go))
- **One shared client:**
  - Timeouts: dial 15 s, TLS 15 s, headers 30 s, idle body 60 s. HTTP/2.
  - Up to 10 redirects, never from https down to http.
  - Per-host rate limits that honour Retry-After (capped at 10 minutes) and Modrinth's reset header.
  - Per-host headers: the CurseForge API key goes only to `api.curseforge.com`.
  - Body caps; URLs are logged without query or userinfo.
- **Metadata cache:**
  - ETag, Last-Modified, Cache-Control and Expires.
  - Duplicate in-flight requests are merged.
  - Content pinned by sha1 skips the network.
  - The last good copy is served when offline.
- **Proxy:** system, none, or a custom HTTP, HTTPS or SOCKS5 proxy with authentication.
  - The password is stored only in the keychain.
  - Proxy changes apply live.

---

## 20. Settings and theming

**Settings pages** ([registry.ts](frontend/src/lib/settings/registry.ts), [routes/settings/](frontend/src/routes/settings/), [settings.go](internal/settings/settings.go))
- **Tabs:** General, Appearance, Java, Downloads, Storage, Syncing (opens the Syncing page), Integrations, Updates, About.
- **Search settings:** searches titles, descriptions and groups, with highlighted, grouped results. [Search.svelte](frontend/src/routes/settings/Search.svelte)
- **Validation and live apply:**
  - Every field has a default and is validated in Go. A bad field is fixed on its own, and a corrupt file is kept as `.corrupt`.
  - Changes apply live: proxy, pool size, Downloads-folder watcher, account refresher, custom CSS, OS registration and Discord.

| Tab › Group | Setting | Options (default) |
|---|---|---|
| General › Launcher | Language | English |
| | When a game starts | Keep open / Hide / Close |
| | Open on startup | Home / Library / Last instance |
| | Start with the computer | toggle |
| | First-run setup | Run again |
| General › Keyboard shortcuts | Every editable chord | capture |
| Downloads | Parallel downloads | 1–16 (8) |
| | Watch the Downloads folder | toggle (on) |
| | Proxy | System / None / Custom → type, host + port, user, password (keychain) |
| Downloads › Mod updates | Mod update channel | Release / Beta / Alpha |
| Integrations | Discord status | toggle |
| | Open Modrinth and CurseForge links | toggle |
| Appearance › Layout | Density | Comfortable / Compact |
| | Text size | Small / Default / Large |
| | Reduce motion | System / On / Off |
| Java | Pick Java automatically | toggle (on) |
| | Download missing versions from | Temurin / Zulu / Mojang (Temurin) |
| | Default memory | min/max MB (1024 / 4096) |
| | Default JVM arguments | text (`-XX:+UseG1GC -XX:+ParallelRefProcEnabled`) |
| | Warn when free memory is low | toggle (on) |
| Java › Instances | Suggest memory from mod count | toggle (on) |
| Updates | Automatic launcher updates | toggle |
| Accounts page | Refresh sign-ins automatically; Play offline when sign-in fails | toggles |

- **Per-instance overrides:** instance settings override these field by field, as in Prism.
- **Storage page** ([Storage.svelte](frontend/src/routes/settings/Storage.svelte)):
  - The data folder's location, noting portable mode.
  - Trashed instances, each with days left, Restore and Delete now.
  - Clean up old logs and unused store files, showing "X freed".
- **Updates page** ([Updates.svelte](frontend/src/routes/settings/Updates.svelte)): launcher version, update status, Check now.
- **About page:** version, system, data folder, total playtime, "Telemetry: None".

**Theming** ([Appearance.svelte](frontend/src/routes/settings/Appearance.svelte), [prefs.svelte.ts](frontend/src/lib/prefs.svelte.ts), [themes.css](frontend/src/styles/themes.css), [tokens.css](frontend/src/styles/tokens.css))
- **Themes:** Monolith (the default dark theme), Graphite, OLED and Light, plus System, which follows the OS (Monolith or Light).
  - The other themes derive their surfaces from the base using `color-mix`.
- **Accent colour:**
  - 7 swatches (`#3ecf8e` is the default) or a custom `#rrggbb`.
  - The text colour on accent backgrounds is computed from WCAG luminance.
- **Density:** Compact sets row heights to 32, 40 and 48.
- **Text size:** zooms the whole UI to 0.923×, 1× or 1.077×.
- **Reduce motion:** On turns off every animation. System follows `prefers-reduced-motion`.
- **Custom CSS:** [customcss.go](internal/theme/customcss.go)
  - **Load file:** pick a CSS file. It reloads every time you save it; the folder is watched because editors save by write-then-rename. Parse errors show as a badge.
  - **Export theme:** writes an example stylesheet built from the current tokens.
  - **Override order:** user CSS loads last, unlayered, so it beats the token layer.
  - **Safety:** it is set through `textContent`, so it can only ever be CSS.
- **Fonts:** Geist and Geist Mono, bundled.
- **Accessibility:**
  - Focus-visible outlines and ARIA labels on every icon button.
  - Proper tab, radio, listbox and menu roles. Menus can be opened from the keyboard.
  - A `role=log` console and `role=progressbar` bars.
  - Skeletons are `aria-hidden`.
- **Interface rules:**
  - Every data view has a skeleton loader shaped like the real content and an empty state ([Empty.svelte](frontend/src/lib/ui/Empty.svelte)).
  - Layouts are stable with fixed row heights.
  - Spacing follows one 24/16/16 grid.

---

## 21. Platform integration

**Command line** ([intent.go](internal/intent/intent.go), [cli.go](internal/cli/cli.go), [main.go](main.go))

| Command | What it does |
|---|---|
| `quartz [--dir <folder>]` | Start the launcher, optionally with another data folder |
| `quartz <link\|file>` | Open a link or pack, as described under deep links |
| `--launch/-l <instance> [--server/-s <addr> \| --world/-w <folder>] [--account/-a <name>]` | Launch an instance |
| `--import/-I <link\|file>` | Open the Import preview |
| `list [--json]`, `accounts [--json]`, `--version/-V [--json]`, `--help` | Headless; reads files read-only, works with the launcher open or closed |

- **Matching:** instances and accounts match by exact id, otherwise by the single case-insensitive name match. [find.go](internal/intent/find.go)
- **Output:** a tab-separated table, or JSON `{ok, command, data|error}`.
- **Exit codes:** 0 OK, 1 failure, 2 usage error.
- **Windows:** the GUI exe attaches to the parent console.
- **Strict parsing:**
  - Unknown flags or stray words are errors, values are length-capped, and control characters are rejected.
  - A second launch forwards only the launch, link or file.

**Deep links and file opens** ([links.go](internal/links/links.go), [api/opens.go](internal/api/opens.go), [intents.ts](frontend/src/lib/intents.ts))

| Link | Opens |
|---|---|
| `.mrpack` / `.zip` file | Import preview |
| `https://…` pack link | Import preview |
| `modrinth://modpack/<slug>` | Import preview |
| `modrinth://{mod,resourcepack,shader,datapack,plugin}/<slug>` | Project page |
| `curseforge://install?addonId=N[&fileId=M]` | Pack preview or project page |
| `quartz://import?url=<https>` | Import preview |
| `quartz://open/<instanceId>` | Instance page |

- **No link installs or launches anything by itself.** Modrinth and CurseForge links are ignored when "Open Modrinth and CurseForge links" is off.
- **Waiting for the UI:** opens queue until the UI is ready (up to 16, 10-minute TTL).
- **No paths to the page:** packs stay in Go, and the UI only gets an opaque id.

**OS registration** ([desktop.go](internal/desktop/desktop.go), [linux.go](internal/desktop/linux.go), [windows.go](internal/desktop/windows.go), [darwin.go](internal/desktop/darwin.go))
- **What gets registered** (per user, no admin rights):
  - The `modrinth://`, `curseforge://` and `quartz://` schemes.
  - The `.mrpack` file type.
  - The app-menu entry.
  - Start with the computer (minimized).
- **Per OS:**
  - **Linux:** a `.desktop` entry, MIME type, 512px icon and autostart entry.
  - **Windows:** HKCU classes, a ProgID, Default Apps capabilities and a Run value. Another app's handler is left alone.
  - **macOS:** Info.plist declarations and a LaunchAgent.
- **Idempotent:** an unchanged start does no work. Turning a setting off removes only entries that point at this launcher. Dev builds never register.

**Instance shortcuts** ([shortcuts/service.go](internal/shortcuts/service.go), [ShortcutDialog.svelte](frontend/src/lib/overlays/ShortcutDialog.svelte))
- **Where:** Desktop, App menu, or both.
- **Options:** a fixed account (optional), and a world or server to open into.
- **Per OS:**
  - Linux: a `.desktop` file, marked trusted.
  - Windows: a `.lnk` created through COM, without cgo.
  - macOS: a minimal `.app` bundle.
- **Icon:** the instance's picture, or a drawn tile in its colour with its letter. [shortcuts/icon.go](internal/shortcuts/icon.go)
- **Lifecycle:**
  - Shortcuts are hidden when the instance is trashed and come back on restore.
  - If the exe moves, they're rewritten.
  - Only files this launcher wrote are ever removed.

**Discord Rich Presence** ([discord/client.go](internal/discord/client.go), [presence.go](internal/discord/presence.go), [app/discord.go](internal/app/discord.go))
- **Connection:** a native IPC client with no library and no cgo. Finds Discord's socket for native, Flatpak, Snap and Vesktop installs.
- **What it shows:** the instance name, "Minecraft X with &lt;Loader&gt;", elapsed time, and a loader icon.
- **When:** connects only while a game runs, with a backoff of 5 to 60 s.
- **Switches:** a global one, plus one per instance.

**Self-update** ([update/update.go](internal/update/update.go), [swap.go](internal/update/swap.go), [app/update.go](internal/app/update.go), [UpdateBanner.svelte](frontend/src/lib/overlays/UpdateBanner.svelte))
- **Source:** GitHub releases of `conspiracy1337/quartz`.
- **Signature first:** `update.json`'s minisign signature (Ed25519 over BLAKE2b-512) is verified *before* the file is parsed. [minisign.go](internal/core/minisign/minisign.go)
- **Validation:** the version must be newer. The per-platform file is at most 1 GB and must match its size and SHA-256. The download runs as a job.
- **Banner:** "Launcher X is ready. Signature verified.", with What changed, Later (installs on quit) and Restart now.
- **Swap:**
  - The new file is placed next to the old one, with lock and fail marker files (Prism's scheme).
  - On the next start a toast says "Updated to X" or reports that the update failed.
  - An interrupted swap restores the old binary.
  - macOS swaps the whole `.app`.
  - The updater never elevates.
- **Off** for dev builds and builds without a signing key.

**Data folder** ([paths.go](internal/core/paths/paths.go), [pointer.go](internal/core/paths/pointer.go))
- **Priority:** `--dir`, then `QUARTZ_DIR`, then **portable mode** (`portable.txt` next to the exe), then the folder chosen in setup, then the OS default. On Windows the default is LocalAppData, so game files don't roam.
- **Moving it** is applied on the next start, before anything opens. Across drives it copies.

**Image server** ([imagesrv.go](internal/imagesrv/imagesrv.go), [remoteimg.go](internal/core/remoteimg/remoteimg.go), [localimg.go](internal/core/localimg/localimg.go))
- **Route:** same-origin `/tex/<kind>/<name>.png` serves skins, icons, content icons, screenshots, thumbnails, world icons, favicons and remote pictures.
  - GET and HEAD only, image types only, `nosniff`, cached immutable.
- **Remote pictures:** proxied from an allowlist of hosts (Modrinth CDN, forgecdn, Mojang, imgur, GitHub uploads). A redirect may only lead to another allowed host. The UI holds only tokens, so it isn't an open proxy.
  - The real file type is sniffed.
  - At most 10 MB per file; the disk cache is an LRU of 256 MB. [disklru.go](internal/core/disklru/disklru.go)
- **Local pictures:** tokens tied to path, size and mtime; thumbnails are made on demand.

---

## 22. Security measures

Gathered here from the sections above for quick review.

- **Tokens and secrets**
  - **Storage:** OS keychain only (Secret Service, Credential Manager, macOS Keychain). There is no file fallback, and long values are chunked for Windows. [secrets.go](internal/core/secrets/secrets.go)
  - **Never on argv:** the access token goes to the game over stdin through the wrapper jar. [wrapperjar.go](internal/wrapperjar/wrapperjar.go)
  - **Redactor:** one redactor covers the launcher log, game logs, saved commands and mclo.gs uploads. It removes JWTs, `--accessToken` arguments, `*_token` pairs, Authorization headers, `code=`/`token=` in URLs, emails, the home user name, and registered literal secrets. [redact.go](internal/core/logging/redact.go)
  - **Sign-in events:** `accounts:signin` events never carry tokens.
  - **Proxy password:** keychain only, never sent back to the UI.
- **OAuth**
  - PKCE S256 with a constant-time state check.
  - The loopback server answers only `GET /`.
  - The browser reply page is script-free with a strict CSP. [browser.go](internal/auth/msa/browser.go)
- **Untrusted input and paths**
  - **Path safety:**
    - `fsx.SafeJoin` and `os.Root` for every extraction and read.
    - Archive caps: 100k files, 1 GB per file, 4 GB total; links must stay inside.
    - Modpack `CleanPath` refuses traversal, drive letters, launcher folders and reserved names.
    - [archive.go](internal/core/archive/archive.go), [fsx.go](internal/core/fsx/fsx.go), [modpack/pack.go](internal/modpack/pack.go)
  - **Paths never come from the page:**
    - Drops, file opens and dialogs give the UI opaque, single-use, expiring ids.
    - Native dialogs are the only way a click picks a path.
    - [drops.go](internal/api/drops.go), [opens.go](internal/api/opens.go), [dialogs.go](internal/api/dialogs.go)
  - **Parsers with caps:** NBT (depth 512, 32 MB), SQLite (depth 32, loop guard), PNG (canonical re-encode), Markdown/HTML changelogs flattened to plain text. [nbt.go](internal/core/nbt/nbt.go), [sqlitero.go](internal/core/sqlitero/sqlitero.go), [pngx.go](internal/core/pngx/pngx.go), [docs.go](internal/core/docs/docs.go)
  - **Strict CLI and link parsing:**
    - Links are checked against an allowlist of shapes.
    - No link can launch a game.
    - `OpenURL` opens https only.
    - [intent.go](internal/intent/intent.go), [links.go](internal/links/links.go), [api/app.go](internal/api/app.go)
- **Network**
  - **https only for downloads.** No https→http redirects.
  - **Host allowlists:** mrpack hosts, platform CDNs (`ValidURL`) and image proxy hosts.
  - **API key scoping:** the CurseForge key is sent only to its own host.
  - **Integrity:** every file is hash-verified, and SHA-1-pinned metadata is checked.
  - [netx.go](internal/core/netx/netx.go), [modplatform.go](internal/modplatform/modplatform.go)
- **Code execution**
  - **Env stripping:** JVM-injection variables are stripped from the game, probes and processors. [java/env.go](internal/java/env.go)
  - **Log4Shell:** `-Dlog4j2.formatMsgNoLookups=true` is always set.
  - **No shell:** player commands and processors run as argv. [shlex.go](internal/core/shlex/shlex.go)
  - **Imports never take commands:** commands and JVM args from other launchers and instance zips are never imported.
  - **Unknown jars flagged:** jars from unrecognised hosts in pack overrides are flagged.
  - **Sandbox:** bwrap + seccomp, Seatbelt or AppContainer, and it fails closed.
- **Updates and files**
  - **Self-update:** the minisign signature is checked before the manifest is parsed. [update.go](internal/update/update.go)
  - **Store integrity:** blobs are read-only, and a sha1 collision is refused.
  - **Atomic writes:** temp file, fsync, rename. [jsonfile.go](internal/core/jsonfile/jsonfile.go)
  - **Exports:** private files excluded, CSV injection guarded.
- **UI**
  - Custom CSS goes in through `textContent`.
  - Rich text renders only through `Inline.svelte`, from plain blocks.
  - Links open in the system browser.

---

## 23. Performance measures

- **Fast checks:**
  - The verified index makes game-file checks a stat, not a hash. [verified.go](internal/core/download/verified.go)
  - Content scans are incremental and stat-only for unchanged files, with up to 8 workers and merged concurrent scans. [scan.go](internal/content/scan.go)
  - Loader processor steps are skipped when their outputs already match. [processing.go](internal/loader/processing.go)
  - A fully installed version starts with no network.
- **No duplicate files:**
  - Content-addressed store with reflink → hardlink → copy placement: FICLONE, APFS `clonefile`, ReFS block cloning, or `copy_file_range`. [linkx.go](internal/core/linkx/linkx.go)
  - Natives and client jars are shared across loaders and versions where possible.
- **Caching:**
  - HTTP metadata cache with merged in-flight requests and offline serving.
  - Disk LRU caches for images.
  - TTL caches in Browse.
  - Java probes cached by size and mtime.
- **Concurrency:**
  - Parallel downloads (configurable).
  - Concurrent Java probes and loader lookups.
  - singleflight for account refreshes, server pings and scans.
- **Quiet when idle:**
  - Events are coalesced: task snapshots at most every 100 ms, console batches every 100 ms, watchers debounced.
  - Timers instead of polling; nothing ticks while idle. Discord connects only while a game runs.
- **Frontend:**
  - Skeletons show only on first load; reloads keep the old data. [query.svelte.ts](frontend/src/lib/query.svelte.ts)
  - The console is virtualised.
  - Lists refresh by revision counters driven by Go events. [revs.svelte.ts](frontend/src/lib/revs.svelte.ts)
- **No cgo** outside the window. No Electron.

---

## 24. Developer tooling

- **Mock mode:** without the Go runtime (Vite on :5173, Playwright, tests), every API call returns cloned fixtures.
  - `?slow` adds 1.2 s of latency so the skeletons show.
  - [bridge.ts](frontend/src/lib/bridge.ts), [api/mock/](frontend/src/lib/api/mock/)
- **Build tags:**
  - `fakeauth`: a local fake of Microsoft, Xbox and Minecraft. [fakeauth.go](internal/app/fakeauth.go), [authtest.go](internal/auth/authtest/authtest.go)
  - `e2e`: native dialogs answered from `QUARTZ_E2E_DIALOGS`. [dialogs_e2e.go](internal/api/dialogs_e2e.go)
- **Tests:** Go integration tests cover browse, content, game, home, host, Java, launchers and loaders. [internal/integration/](internal/integration/)
- **Design parity:** Playwright screenshots of the design boards and the mock app, ranked by pixel and text difference. [tools/parity/](tools/parity/)
- **Release tool:** [tools/release/](tools/release/)
  - `keygen`: a minisign key sealed with scrypt + XChaCha20-Poly1305.
  - `build`: every OS/arch with ldflags; the macOS app is zipped and ad-hoc signed.
  - `manifest`: a signed `update.json`.
  - `sign`.
- **Wrapper jar:** a reproducible Java 8 build of the stdin launcher. [tools/wrapperjar/](tools/wrapperjar/)
- **Builds:**
  - [build.sh](build.sh) makes a one-command stripped production build.
  - Build-time IDs (Microsoft client ID, CurseForge key, Discord ID, update public key) come in through ldflags with env overrides. A malformed ID turns its feature off. [brand.go](internal/brand/brand.go)
