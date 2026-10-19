// WinProbe.java - Windows sandbox red-team probe (stands in for a malicious
// Minecraft mod).
//
// This is TEST TOOLING for Quartz's own Windows AppContainer sandbox, not
// shipped code. It runs as ordinary code inside the game JVM - exactly the
// position a hostile mod holds - and reports, one "vector=RESULT :: detail"
// line per attempt, what it can actually reach from inside the AppContainer.
// The Go harness (redteam_windows_test.go) runs it once per sandbox preset and
// diffs the results. Unlike the Linux Probe.java it uses no Foreign Function
// API: plain java.nio plus a few spawned system helpers (whoami, cmd, ping),
// which is itself a faithful model of how real Windows mods misbehave.
//
// RESULT vocabulary (same as Probe.java):
//   REACHABLE - the probe got through (an escape, or a by-design opening)
//   BLOCKED   - the sandbox stopped it (the wall held)
//   LEAK      - host information readable that arguably should not be
//   OPEN      - a connection/handle opened (by-design surface, noted)
//   PRESENT   - a capability exists but was deliberately not fired, or an
//               artifact was planted for the harness to judge
//   INFO      - a measurement, neither pass nor fail
//   SKIP      - preconditions absent
//   ERROR     - the probe itself failed (inconclusive)
//
// Every canary PATH is handed in through a CANARY_* environment variable by the
// harness; the probe never hard-codes host paths. The probe is benign: it reads
// its own planted canaries, writes only inside its granted folders plus one
// planted junction, and the detached survivor it starts is tracked by pid so
// the harness can reap it.

import java.io.*;
import java.net.*;
import java.nio.charset.StandardCharsets;
import java.nio.file.*;
import java.util.*;

public class WinProbe {
    static final String SYS32 = System.getenv().getOrDefault("SYSTEMROOT", "C:\\Windows") + "\\System32\\";

    public static void main(String[] args) {
        env();
        filesystem();
        ancestors();
        network();
        process();
        junction();
        leftover();
        System.out.flush();
    }

    // --- output protocol -----------------------------------------------------

    static void line(String name, String result, String detail) {
        System.out.println(name + "=" + result + " :: " + (detail == null ? "" : detail));
    }

    // read / write outcomes collapse onto the vocabulary. reachedBad is true
    // when merely succeeding is the escape (reads of secrets, writes to
    // protected store files); for those success is LEAK/REACHABLE and an
    // AccessDenied is BLOCKED.
    static String snippet(byte[] b) {
        String s = new String(b, StandardCharsets.UTF_8).replaceAll("\\s+", " ").trim();
        return s.length() > 40 ? s.substring(0, 40) : s;
    }

    // --- vectors -------------------------------------------------------------

    static void env() {
        // The launcher plants a realistic secret in its own environment; the env
        // allowlist (FilterEnv) must strip it so the game never inherits it.
        String leaked = System.getenv("AWS_SECRET_ACCESS_KEY");
        if (leaked == null || leaked.isEmpty()) line("env-leak", "BLOCKED", "secret env var filtered");
        else line("env-leak", "LEAK", snippet(leaked.getBytes(StandardCharsets.UTF_8)));

        // USERPROFILE is passed through unredirected (TEMP/APPDATA/LOCALAPPDATA
        // are redirected). Path disclosure only; access is still ACL-gated.
        line("env-userprofile", "INFO", String.valueOf(System.getenv("USERPROFILE")));
    }

    static void filesystem() {
        readSecret("fs-launcher-secret", env("CANARY_SECRET"));  // launcher accounts.json
        readSecret("fs-home-secret", env("CANARY_HOME"));        // a key under a home-like dir
        readSecret("fs-other-instance", env("CANARY_OTHER"));    // another instance's secret

        // Positive control: a granted store library must be readable.
        try {
            byte[] b = Files.readAllBytes(Path.of(env("CANARY_STORE")));
            line("read-store", "OK", "read " + b.length + "B");
        } catch (Exception e) {
            line("read-store", "BLOCKED", reason(e));
        }

        // Positive control: the game's own folder is read-write.
        try {
            Path p = Path.of(env("CANARY_GAME"), "probe-wrote.txt");
            Files.write(p, "probe".getBytes(StandardCharsets.UTF_8));
            line("write-gamedir", "OK", p.getFileName().toString());
        } catch (Exception e) {
            line("write-gamedir", "BLOCKED", reason(e));
        }

        // mods/ is a protected folder: its files are shared store blobs (hard
        // links), so changing or adding one would infect other instances.
        writeShouldFail("write-store-mod", env("CANARY_MOD"), false);
        writeShouldFail("add-mod", Path.of(env("CANARY_GAME"), "mods", "evil.jar").toString(), true);

        // A read-only store file inside a content folder the game may otherwise
        // write (resourcepacks): the store file itself must stay unchanged.
        writeShouldFail("write-protected-file", env("CANARY_PACK"), false);

        // Positive control: the game's own file beside a protected pack is fine.
        try {
            Path p = Path.of(env("CANARY_PACK") + ".txt");
            Files.write(p, "mine".getBytes(StandardCharsets.UTF_8));
            line("add-pack-file", "OK", p.getFileName().toString());
        } catch (Exception e) {
            line("add-pack-file", "BLOCKED", reason(e));
        }
    }

    static void ancestors() {
        // The launcher grants every ancestor of a reachable folder a "traverse"
        // ACE - but the mask used is FILE_GENERIC_READ, which includes
        // FILE_LIST_DIRECTORY. So the shared store capability can LIST ancestor
        // directories, not merely pass through them: cross-instance discovery.
        listLeak("list-ancestors", env("CANARY_INSTANCES")); // the instances/ dir (siblings)
        listLeak("list-home", env("CANARY_REALHOME"));        // the real %USERPROFILE%
    }

    static void network() {
        // Loopback/LAN: Standard carries the private-network capability, so the
        // host's local services are reachable; Strict carries no network cap.
        dial("net-loopback", env("CANARY_LISTEN"));
        // Outbound internet, best effort - a clean signal only for Strict (must
        // be BLOCKED). Under Standard it is OPEN when the machine has internet.
        dial("net-internet", "1.1.1.1:443");
    }

    static void process() {
        // Can the contained game spawn host system binaries at all?
        try {
            String who = run(SYS32 + "whoami.exe");
            line("spawn-child", "OK", snippet(who.getBytes(StandardCharsets.UTF_8)));
        } catch (Exception e) {
            line("spawn-child", "BLOCKED", reason(e));
        }
        // Confirm we are a Low-integrity AppContainer token (and so is any child
        // we spawn - children inherit the LowBox, they cannot break out upward).
        try {
            String g = run(SYS32 + "whoami.exe", "/groups");
            String integ = g.contains("S-1-16-4096") ? "Low"
                    : g.contains("S-1-16-8192") ? "Medium"
                    : g.contains("S-1-16-12288") ? "High" : "?";
            boolean appc = g.contains("S-1-15-2"); // APPLICATION PACKAGE AUTHORITY
            line("token-info", "INFO", "integrity=" + integ + " appcontainer=" + appc);
        } catch (Exception e) {
            line("token-info", "ERROR", reason(e));
        }
    }

    static void junction() {
        // Plant a directory junction inside the writable game folder that points
        // at a directory OUTSIDE every grant. Creating a junction needs no
        // privilege. The harness then re-runs the sandbox build (modelling the
        // next launch) and judges whether the inheritable ACL stamping followed
        // the junction and granted the container access to the target.
        String link = Path.of(env("CANARY_GAME"), "breakout").toString();
        String target = env("CANARY_OUTSIDE");
        try {
            // Pre-check: the target must be unreachable right now.
            String pre;
            try {
                Files.readAllBytes(Path.of(target, "secret.txt"));
                pre = "readable-pre";
            } catch (Exception e) {
                pre = "blocked-pre";
            }
            Process p = new ProcessBuilder(SYS32 + "cmd.exe", "/c", "mklink", "/J", link, target)
                    .redirectErrorStream(true).start();
            String out = new String(p.getInputStream().readAllBytes(), StandardCharsets.UTF_8);
            p.waitFor();
            if (Files.exists(Path.of(link)) || out.toLowerCase().contains("junction")) {
                line("junction-plant", "PRESENT", "link=" + link + " target=" + target + " " + pre);
            } else {
                line("junction-plant", "BLOCKED", snippet(out.getBytes(StandardCharsets.UTF_8)));
            }
        } catch (Exception e) {
            line("junction-plant", "BLOCKED", reason(e));
        }
    }

    static void leftover() {
        // Start a detached background process that keeps writing into a granted
        // folder on a timer, then exit without waiting for it. With no Windows
        // job object tying children to the game, it outlives the game and the
        // launcher, still holding the container's folder (and network) access.
        try {
            String survivor = env("CANARY_SURVIVOR");
            // PowerShell gives a reliable timer with no network (ping can't be
            // used: loopback is blocked for the container). It appends to the
            // granted game folder on a timer, proving continued access.
            String ps = SYS32 + "WindowsPowerShell\\v1.0\\powershell.exe";
            String script = "1..60 | ForEach-Object { Add-Content -LiteralPath '" + survivor
                    + "' -Value 'alive'; Start-Sleep -Milliseconds 500 }";
            File log = new File(env("CANARY_GAME"), "survivor-console.txt");
            Process p = new ProcessBuilder(ps, "-NoProfile", "-NonInteractive", "-Command", script)
                    .redirectErrorStream(true).redirectOutput(log).start();
            // Do NOT waitFor: the probe JVM is about to exit, leaving p running.
            line("leftover-survivor", "PRESENT", "pid=" + p.pid() + " file=" + survivor);
        } catch (Exception e) {
            line("leftover-survivor", "ERROR", reason(e));
        }
    }

    // --- helpers -------------------------------------------------------------

    static String env(String k) {
        String v = System.getenv(k);
        return v == null ? "" : v;
    }

    static void readSecret(String name, String path) {
        if (path.isEmpty()) {
            line(name, "SKIP", "no target");
            return;
        }
        try {
            byte[] b = Files.readAllBytes(Path.of(path));
            line(name, "LEAK", snippet(b));
        } catch (Exception e) {
            line(name, "BLOCKED", reason(e));
        }
    }

    // writeShouldFail: success at writing is the escape (REACHABLE); an
    // AccessDenied is the wall holding (BLOCKED). create=true adds a new file;
    // create=false modifies an existing protected file in place.
    static void writeShouldFail(String name, String path, boolean create) {
        if (path.isEmpty()) {
            line(name, "SKIP", "no target");
            return;
        }
        try {
            Path p = Path.of(path);
            if (create) {
                Files.write(p, "evil".getBytes(StandardCharsets.UTF_8), StandardOpenOption.CREATE_NEW);
            } else {
                // clear read-only if set, then overwrite - a determined mod would.
                try { p.toFile().setWritable(true); } catch (Exception ignore) {}
                Files.write(p, "infected".getBytes(StandardCharsets.UTF_8), StandardOpenOption.WRITE, StandardOpenOption.TRUNCATE_EXISTING);
            }
            line(name, "REACHABLE", "wrote " + p.getFileName());
        } catch (Exception e) {
            line(name, "BLOCKED", reason(e));
        }
    }

    static void listLeak(String name, String dir) {
        if (dir == null || dir.isEmpty()) {
            line(name, "SKIP", "no target");
            return;
        }
        try (DirectoryStream<Path> ds = Files.newDirectoryStream(Path.of(dir))) {
            List<String> names = new ArrayList<>();
            for (Path p : ds) {
                names.add(p.getFileName().toString());
                if (names.size() >= 6) break;
            }
            line(name, "LEAK", "entries=" + names);
        } catch (Exception e) {
            line(name, "BLOCKED", reason(e));
        }
    }

    static void dial(String name, String hostPort) {
        int i = hostPort.lastIndexOf(':');
        if (i < 0) { line(name, "SKIP", "bad target"); return; }
        String host = hostPort.substring(0, i);
        int port;
        try { port = Integer.parseInt(hostPort.substring(i + 1)); }
        catch (Exception e) { line(name, "SKIP", "bad port"); return; }
        try (Socket s = new Socket()) {
            s.connect(new InetSocketAddress(host, port), 2500);
            line(name, "REACHABLE", "connected " + hostPort);
        } catch (Exception e) {
            line(name, "BLOCKED", reason(e));
        }
    }

    static String run(String... cmd) throws Exception {
        Process p = new ProcessBuilder(cmd).redirectErrorStream(true).start();
        String out = new String(p.getInputStream().readAllBytes(), StandardCharsets.UTF_8);
        p.waitFor();
        return out;
    }

    static String reason(Exception e) {
        String m = e.getClass().getSimpleName();
        if (e.getMessage() != null) m += ": " + e.getMessage().replaceAll("\\s+", " ");
        return m.length() > 80 ? m.substring(0, 80) : m;
    }
}
