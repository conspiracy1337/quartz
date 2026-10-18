// Probe.java - sandbox red-team probe (stands in for a malicious Minecraft mod).
//
// This is TEST TOOLING for Quartz's own sandbox, not shipped code. It runs as
// ordinary code inside the game JVM - exactly the position a hostile mod holds -
// and reports, one "vector=RESULT :: detail" line per attempt, what it can
// actually reach from inside the bubblewrap jail. The Go harness
// (redteam_test.go) runs it once per sandbox preset and diffs the results.
//
// RESULT vocabulary:
//   REACHABLE - the probe got through (an escape, or a by-design opening)
//   BLOCKED   - the sandbox stopped it (the wall held)
//   LEAK      - host information readable that arguably should not be
//   OPEN      - a device/handle opened (by-design attack surface, noted)
//   PRESENT   - a capability exists but was deliberately not fired (safety)
//   INFO      - a measurement, neither pass nor fail
//   SKIP      - preconditions absent (e.g. no display)
//   ERROR     - the probe itself failed (inconclusive)
//
// The X11 vectors speak the wire protocol directly over the bound path socket,
// using only the standard library and the bound .Xauthority cookie - no AWT, no
// native code. They are CANARY-ONLY: capture targets a pixmap the probe creates
// itself, injection is a benign zero-delta motion, and keylogging is reported by
// extension presence only. The probe never reads real window pixels, titles, or
// keystrokes.

import java.awt.image.BufferedImage;
import java.io.*;
import java.net.*;
import java.nio.*;
import java.nio.channels.*;
import java.nio.charset.StandardCharsets;
import java.nio.file.*;
import java.util.*;
import javax.imageio.ImageIO;

public class Probe {
    static void line(String name, String result, String detail) {
        System.out.println(name + "=" + result + " :: " + detail);
        System.out.flush();
    }
    static String env(String k) { String v = System.getenv(k); return v == null ? "" : v; }

    public static void main(String[] args) {
        // Hard watchdog: nothing this probe does may hang the harness.
        Thread wd = new Thread(() -> { try { Thread.sleep(60000); } catch (InterruptedException ignore) {} Runtime.getRuntime().halt(0); });
        wd.setDaemon(true); wd.start();

        try { fsWall(); } catch (Throwable t) { line("fs", "ERROR", t.toString()); }
        try { hostInfoLeaks(); } catch (Throwable t) { line("leaks", "ERROR", t.toString()); }
        try { noNewPrivs(); } catch (Throwable t) { line("no-new-privs", "ERROR", t.toString()); }
        try { procIsolation(); } catch (Throwable t) { line("proc-pids", "ERROR", t.toString()); }
        try { loopback(); } catch (Throwable t) { line("net-loopback", "ERROR", t.toString()); }
        try { netlink(); } catch (Throwable t) { line("net-netlink", "ERROR", t.toString()); }
        try { new X11().run(); } catch (Throwable t) { line("x11", "ERROR", t.toString()); }
        try { seccomp(); } catch (Throwable t) { line("seccomp", "ERROR", t.toString()); }
        try { devices(); } catch (Throwable t) { line("devices", "ERROR", t.toString()); }
        System.out.flush();
        Runtime.getRuntime().halt(0);
    }

    // --- Filesystem wall: these reads must fail inside the jail. ---
    static void fsWall() {
        String secret = env("CANARY_SECRET");
        if (!secret.isEmpty()) {
            try {
                byte[] b = Files.readAllBytes(Paths.get(secret));
                line("fs-launcher-secret", "REACHABLE", "read " + b.length + " bytes of the launcher's account store: " + secret);
            } catch (Exception e) {
                line("fs-launcher-secret", "BLOCKED", "cannot read launcher data root (" + e.getClass().getSimpleName() + ")");
            }
        }
        String home = env("CANARY_HOME");
        if (!home.isEmpty()) {
            try {
                byte[] b = Files.readAllBytes(Paths.get(home));
                line("fs-home-secret", "REACHABLE", "read " + b.length + " bytes of a real home file: " + home);
            } catch (Exception e) {
                line("fs-home-secret", "BLOCKED", "real home not visible (" + e.getClass().getSimpleName() + ")");
            }
        }
    }

    // --- Host fingerprint leaks (readable by design; flagged as info leak). ---
    static void hostInfoLeaks() {
        try {
            String id = Files.readString(Paths.get("/etc/machine-id")).trim();
            line("leak-machine-id", "LEAK", "stable host identifier readable: " + id);
        } catch (Exception e) {
            line("leak-machine-id", "BLOCKED", e.getClass().getSimpleName());
        }
        StringBuilder sb = new StringBuilder();
        for (String f : new String[]{"product_name", "board_vendor", "sys_vendor", "product_serial"}) {
            try {
                String v = Files.readString(Paths.get("/sys/class/dmi/id/" + f)).trim();
                if (!v.isEmpty()) sb.append(f).append('=').append(v).append("; ");
            } catch (Exception ignore) {}
        }
        if (sb.length() > 0) line("leak-dmi", "LEAK", "host hardware fingerprint readable: " + sb);
        else line("leak-dmi", "BLOCKED", "DMI id not readable");
    }

    // --- Verify no_new_privs is set (bwrap's implicit behaviour). ---
    static void noNewPrivs() throws Exception {
        for (String ln : Files.readAllLines(Paths.get("/proc/self/status"))) {
            if (ln.startsWith("NoNewPrivs")) {
                String v = ln.split("\\s+")[1].trim();
                line("no-new-privs", v.equals("1") ? "INFO" : "ERROR", "NoNewPrivs=" + v + (v.equals("1") ? " (set, suid binaries neutralised)" : " (NOT SET - suid escalation possible)"));
                return;
            }
        }
        line("no-new-privs", "ERROR", "NoNewPrivs line absent");
    }

    // --- PID namespace: the jail should see only its own processes. ---
    static void procIsolation() throws Exception {
        int pids = 0; StringBuilder names = new StringBuilder();
        try (DirectoryStream<Path> ds = Files.newDirectoryStream(Paths.get("/proc"))) {
            for (Path p : ds) {
                String n = p.getFileName().toString();
                if (!n.isEmpty() && n.chars().allMatch(Character::isDigit)) {
                    pids++;
                    try { names.append(n).append(':').append(Files.readString(p.resolve("comm")).trim()).append(' '); } catch (Exception ignore) {}
                }
            }
        }
        line("proc-pids", pids <= 4 ? "INFO" : "LEAK", "visible processes=" + pids + " [" + names.toString().trim() + "]" + (pids <= 4 ? " (PID namespace isolates host)" : " (host PIDs visible!)"));
    }

    // --- Shared host network namespace: reach a host-local service. ---
    static void loopback() {
        String addr = env("CANARY_LISTEN");
        if (addr.isEmpty()) { line("net-loopback", "SKIP", "no CANARY_LISTEN"); return; }
        int c = addr.lastIndexOf(':');
        String host = addr.substring(0, c); int port = Integer.parseInt(addr.substring(c + 1));
        try (Socket s = new Socket()) {
            s.connect(new InetSocketAddress(host, port), 2000);
            line("net-loopback", "REACHABLE", "connected to host loopback service " + addr + " - the game shares the host network namespace");
        } catch (Exception e) {
            line("net-loopback", "BLOCKED", "host loopback service unreachable (" + e.getClass().getSimpleName() + ") - network namespace isolated");
        }
    }

    // --- Netlink via `ip`: shared netns exposes host interfaces/routes. ---
    static void netlink() {
        String ip = env("CANARY_IP");
        if (ip.isEmpty()) { line("net-netlink", "SKIP", "no ip binary"); return; }
        try {
            Process p = new ProcessBuilder(ip, "-o", "link", "show").redirectErrorStream(true).start();
            String out = new String(p.getInputStream().readAllBytes(), StandardCharsets.UTF_8);
            p.waitFor();
            TreeSet<String> ifs = new TreeSet<>();
            for (String ln : out.split("\n")) {
                String[] parts = ln.split(":\\s*");
                if (parts.length >= 2) { String nm = parts[1].split("@")[0].trim(); if (!nm.isEmpty()) ifs.add(nm); }
            }
            boolean hostVisible = ifs.stream().anyMatch(n -> !n.equals("lo"));
            line("net-netlink", hostVisible ? "REACHABLE" : "BLOCKED",
                 "interfaces visible via netlink: " + ifs + (hostVisible ? " - host interfaces exposed" : " - loopback only, isolated"));
        } catch (Exception e) {
            line("net-netlink", "ERROR", e.toString());
        }
    }

    // --- seccomp denylist: these syscalls must be refused. ---
    static void seccomp() {
        execDenied("seccomp-unshare", env("CANARY_UNSHARE"), "-Urm", "/bin/true");
        execDenied("seccomp-mount", env("CANARY_MOUNT"), "-t", "tmpfs", "none", env("CANARY_GAME"));
        execDenied("seccomp-ptrace", env("CANARY_STRACE"), "-f", "/bin/true");
    }
    static void execDenied(String name, String bin, String... rest) {
        if (bin == null || bin.isEmpty()) { line(name, "SKIP", "host binary absent"); return; }
        try {
            ArrayList<String> cmd = new ArrayList<>(); cmd.add(bin); cmd.addAll(Arrays.asList(rest));
            Process p = new ProcessBuilder(cmd).redirectErrorStream(true).start();
            String out = new String(p.getInputStream().readAllBytes(), StandardCharsets.UTF_8).trim();
            int rc = p.waitFor();
            String lo = out.toLowerCase();
            boolean denied = rc != 0 || lo.contains("permitted") || lo.contains("denied");
            line(name, denied ? "BLOCKED" : "REACHABLE",
                 "exit=" + rc + " \"" + out.replace('\n', '|') + "\"" + (denied ? " - syscall refused" : " - SUCCEEDED (escape!)"));
        } catch (Exception e) {
            line(name, "BLOCKED", "could not even exec: " + e.getClass().getSimpleName());
        }
    }

    // --- Device nodes handed to the game (by design, for graphics/audio). ---
    static void devices() {
        ArrayList<String> open = new ArrayList<>();
        ArrayList<String> glob = new ArrayList<>(Arrays.asList("/dev/dri/card0", "/dev/dri/renderD128", "/dev/snd/seq"));
        try (DirectoryStream<Path> ds = Files.newDirectoryStream(Paths.get("/dev"), "nvidia*")) {
            for (Path p : ds) glob.add(p.toString());
        } catch (Exception ignore) {}
        for (String d : glob) {
            try (RandomAccessFile f = new RandomAccessFile(d, "r")) { open.add(d); } catch (Exception ignore) {}
        }
        line("devices", open.isEmpty() ? "BLOCKED" : "OPEN",
             open.isEmpty() ? "no GPU/audio device nodes" : "device nodes open to the game: " + open + " (by design; direct host-hardware surface)");
    }

    // --- Raw X11 client over the bound path socket. ---
    static final class X11 {
        SocketChannel ch;
        int root, rootDepth, ridBase, ridMask, ridNext = 1, seq = 0, screenW, screenH;

        void run() {
            String display = env("DISPLAY");
            if (display.isEmpty()) { line("x11-handshake", "SKIP", "no DISPLAY"); return; }
            int dpy = 0;
            try { String a = display.substring(display.indexOf(':') + 1); dpy = Integer.parseInt(a.split("\\.")[0]); } catch (Exception ignore) {}
            String sockPath = "/tmp/.X11-unix/X" + dpy;
            if (!Files.exists(Paths.get(sockPath))) { line("x11-handshake", "BLOCKED", "X socket " + sockPath + " not bound into jail"); return; }
            try {
                ch = SocketChannel.open(StandardProtocolFamily.UNIX);
                ch.connect(UnixDomainSocketAddress.of(sockPath));
                if (!handshake()) return;
                visibility();
                capture();
                injection();
                keylogExt();
            } catch (Exception e) {
                line("x11", "ERROR", e.toString());
            } finally {
                try { if (ch != null) ch.close(); } catch (IOException ignore) {}
            }
        }

        boolean handshake() throws IOException {
            byte[] cookie = readXauthCookie(env("XAUTHORITY"));
            byte[] name = "MIT-MAGIC-COOKIE-1".getBytes(StandardCharsets.US_ASCII);
            byte[] data = cookie == null ? new byte[0] : cookie;
            ByteBuffer req = ByteBuffer.allocate(12 + pad4(name.length) + pad4(data.length)).order(ByteOrder.LITTLE_ENDIAN);
            req.put((byte) 0x6c).put((byte) 0).putShort((short) 11).putShort((short) 0);
            req.putShort((short) name.length).putShort((short) data.length).putShort((short) 0);
            req.put(name); pad(req, name.length); req.put(data); pad(req, data.length);
            req.flip(); while (req.hasRemaining()) ch.write(req);

            ByteBuffer hdr = readN(8).order(ByteOrder.LITTLE_ENDIAN);
            int status = hdr.get(0) & 0xff;
            int addLen = 4 * (hdr.getShort(6) & 0xffff);
            ByteBuffer add = readN(addLen).order(ByteOrder.LITTLE_ENDIAN);
            if (status != 1) {
                int reasonLen = hdr.get(1) & 0xff;
                String reason = new String(add.array(), 0, Math.min(reasonLen, add.capacity()), StandardCharsets.US_ASCII);
                line("x11-handshake", status == 0 ? "BLOCKED" : "ERROR",
                     (status == 0 ? "X server refused auth" : "further auth required") + ": " + reason.trim() + (cookie == null ? " (no cookie found in XAUTHORITY)" : ""));
                return false;
            }
            int vendorLen = add.getShort(16) & 0xffff;
            int nScreens = add.get(20) & 0xff;
            int nFormats = add.get(21) & 0xff;
            ridBase = add.getInt(4); ridMask = add.getInt(8);
            int off = 32 + pad4(vendorLen) + 8 * nFormats; // first SCREEN
            root = add.getInt(off);
            screenW = add.getShort(off + 20) & 0xffff;
            screenH = add.getShort(off + 22) & 0xffff;
            rootDepth = add.get(off + 38) & 0xff;
            String vendor = new String(add.array(), 32, Math.min(vendorLen, add.capacity() - 32), StandardCharsets.US_ASCII);
            line("x11-handshake", "REACHABLE",
                 "authenticated to X server '" + vendor.trim() + "', screens=" + nScreens + ", root=" + screenW + "x" + screenH + " depth " + rootDepth + " - a mod drives your display directly over the bound socket");
            return true;
        }

        void visibility() throws IOException {
            ByteBuffer r = request(15, 0, new int[]{root}, null); // QueryTree
            int nChildren = r.order(ByteOrder.LITTLE_ENDIAN).getShort(16) & 0xffff;
            line("x11-visibility", nChildren > 0 ? "REACHABLE" : "INFO",
                 nChildren + " other top-level windows visible in the X tree (titles/content not read) - cross-client visibility");
        }

        void capture() {
            try {
                int w = 64, h = 64, pix = newId(), gc = newId();
                // CreatePixmap(depth, pid, drawable=root, w, h)
                send(53, rootDepth, new int[]{pix, root}, shorts(w, h));
                // CreateGC(cid, drawable=pix, mask=GCForeground(0x4), value=color)
                send(55, 0, new int[]{gc, pix, 0x4, 0x2040C0}, null);
                // PolyFillRectangle(pix, gc, rect{0,0,w,h})
                send(70, 0, new int[]{pix, gc}, shorts(0, 0, w, h));
                // GetImage(format=ZPixmap(2), drawable=pix, x,y,w,h, plane-mask=all)
                ByteBuffer img = request(73, 2, new int[]{pix}, concat(shorts(0, 0, w, h), intLE(0xFFFFFFFF)));
                img.order(ByteOrder.LITTLE_ENDIAN);
                int depth = img.get(1) & 0xff;
                int dataLen = img.getInt(4) * 4;
                int bpp = depth > 16 ? 4 : (depth > 8 ? 2 : 1);
                byte[] raw = new byte[dataLen];
                img.position(32); img.get(raw, 0, Math.min(dataLen, img.remaining()));
                BufferedImage out = new BufferedImage(w, h, BufferedImage.TYPE_INT_RGB);
                for (int y = 0; y < h; y++) for (int x = 0; x < w; x++) {
                    int i = (y * w + x) * bpp;
                    int rr = 0, gg = 0, bb = 0;
                    if (i + bpp <= raw.length && bpp == 4) { bb = raw[i] & 0xff; gg = raw[i + 1] & 0xff; rr = raw[i + 2] & 0xff; }
                    out.setRGB(x, y, (rr << 16) | (gg << 8) | bb);
                }
                String dir = env("CANARY_GAME"); if (dir.isEmpty()) dir = ".";
                File png = new File(dir, "redteam-canary-capture.png");
                ImageIO.write(out, "png", png);
                line("x11-capture", "REACHABLE",
                     "GetImage returned " + dataLen + " bytes from a drawable the mod created -> " + png.getAbsolutePath() +
                     "; the identical request targets the root window (" + screenW + "x" + screenH + ") = full-screen capture");
            } catch (Exception e) {
                line("x11-capture", "ERROR", e.toString());
            }
        }

        void injection() {
            try {
                int major = queryExtension("XTEST");
                if (major < 0) { line("x11-inject", "BLOCKED", "XTEST extension not available"); return; }
                // XTestFakeInput (36-byte request, length 9: sz_xXTestFakeInputReq):
                // type=MotionNotify(6), detail=relative(1), rootX=rootY=0 => benign no-op.
                ByteBuffer b = ByteBuffer.allocate(36).order(ByteOrder.LITTLE_ENDIAN);
                b.put((byte) major).put((byte) 2).putShort((short) 9);
                b.put((byte) 6).put((byte) 1).putShort((short) 0); // type, detail(relative), pad0
                b.putInt(0);            // time = CurrentTime
                b.putInt(0);            // root = None (current)
                b.putInt(0).putInt(0);  // pad1, pad2
                b.putShort((short) 0).putShort((short) 0); // rootX, rootY (relative 0,0 = no movement)
                b.putInt(0).putShort((short) 0).put((byte) 0).put((byte) 0); // pad3, pad4, pad5, deviceid
                b.flip(); seq++; while (b.hasRemaining()) ch.write(b);
                // Sync barrier: if the FakeInput was rejected, the error surfaces before this reply.
                request(14, 0, new int[]{root}, null); // GetGeometry(root)
                line("x11-inject", "REACHABLE",
                     "XTEST FakeInput accepted by the server (benign 0-pixel motion fired); keystroke/click injection into the focused window is the identical call");
            } catch (Exception e) {
                line("x11-inject", e.toString().contains("X error") ? "BLOCKED" : "ERROR", e.toString());
            }
        }

        void keylogExt() {
            try {
                int rec = queryExtension("RECORD");
                int xi = queryExtension("XInputExtension");
                boolean any = rec >= 0 || xi >= 0;
                line("x11-keylog-ext", any ? "PRESENT" : "BLOCKED",
                     "global input-capture extensions present: " + (rec >= 0 ? "RECORD " : "") + (xi >= 0 ? "XInput2 " : "") +
                     "- passive keylogging across all X clients is available (capability only; not exercised)");
            } catch (Exception e) {
                line("x11-keylog-ext", "ERROR", e.toString());
            }
        }

        int queryExtension(String ext) throws IOException {
            byte[] nm = ext.getBytes(StandardCharsets.US_ASCII);
            ByteBuffer extra = ByteBuffer.allocate(4 + pad4(nm.length)).order(ByteOrder.LITTLE_ENDIAN);
            extra.putShort((short) nm.length).putShort((short) 0).put(nm); // pad handled by capacity
            ByteBuffer r = request(98, 0, null, extra.array()).order(ByteOrder.LITTLE_ENDIAN);
            boolean present = (r.get(8) & 0xff) == 1;
            return present ? (r.get(9) & 0xff) : -1;
        }

        int newId() { return ridBase | (ridNext++ & ridMask); }

        // send: a request with no reply.
        void send(int opcode, int data1, int[] words, byte[] trailing) throws IOException {
            ch.write(buildRequest(opcode, data1, words, trailing));
            seq++;
        }
        // request: a request whose reply we read back (draining events/errors).
        ByteBuffer request(int opcode, int data1, int[] words, byte[] trailing) throws IOException {
            ch.write(buildRequest(opcode, data1, words, trailing));
            seq++;
            return readReply();
        }
        ByteBuffer buildRequest(int opcode, int data1, int[] words, byte[] trailing) {
            int body = (words == null ? 0 : words.length * 4) + (trailing == null ? 0 : pad4(trailing.length));
            int total = 4 + body;
            ByteBuffer b = ByteBuffer.allocate(total).order(ByteOrder.LITTLE_ENDIAN);
            b.put((byte) opcode).put((byte) data1).putShort((short) (total / 4));
            if (words != null) for (int w : words) b.putInt(w);
            if (trailing != null) { b.put(trailing); for (int i = trailing.length; i < pad4(trailing.length); i++) b.put((byte) 0); }
            b.flip(); return b;
        }
        // readReply reads 32-byte packets, skipping events, throwing on X errors,
        // until the reply for the most recent request arrives.
        ByteBuffer readReply() throws IOException {
            while (true) {
                ByteBuffer p = readN(32).order(ByteOrder.LITTLE_ENDIAN);
                int type = p.get(0) & 0xff;
                if (type == 0) { // Error
                    int code = p.get(1) & 0xff;
                    throw new IOException("X error code " + code + " (request seq " + (p.getShort(2) & 0xffff) + ")");
                }
                if (type == 1) { // Reply
                    int extra = p.getInt(4) * 4;
                    if (extra == 0) return p;
                    ByteBuffer full = ByteBuffer.allocate(32 + extra).order(ByteOrder.LITTLE_ENDIAN);
                    full.put(p.array(), 0, 32);
                    ByteBuffer more = readN(extra);
                    full.put(more.array(), 0, extra);
                    full.rewind(); return full;
                }
                // else: an event (type >= 2) - ignore and keep reading.
            }
        }
        ByteBuffer readN(int n) throws IOException {
            ByteBuffer b = ByteBuffer.allocate(n);
            while (b.hasRemaining()) { if (ch.read(b) < 0) throw new EOFException("short X read"); }
            b.flip(); return b;
        }
    }

    static byte[] readXauthCookie(String path) {
        if (path == null || path.isEmpty()) return null;
        try {
            byte[] b = Files.readAllBytes(Paths.get(path));
            ByteBuffer bb = ByteBuffer.wrap(b).order(ByteOrder.BIG_ENDIAN); // xauth is big-endian
            while (bb.remaining() >= 2) {
                bb.getShort(); // family
                byte[] addr = readXauthField(bb);
                byte[] num = readXauthField(bb);
                byte[] nm = readXauthField(bb);
                byte[] data = readXauthField(bb);
                if (addr == null || num == null || nm == null || data == null) break;
                if (new String(nm, StandardCharsets.US_ASCII).equals("MIT-MAGIC-COOKIE-1")) return data;
            }
        } catch (Exception ignore) {}
        return null;
    }
    static byte[] readXauthField(ByteBuffer bb) {
        if (bb.remaining() < 2) return null;
        int len = bb.getShort() & 0xffff;
        if (bb.remaining() < len) return null;
        byte[] out = new byte[len]; bb.get(out); return out;
    }
    static int pad4(int n) { return (n + 3) & ~3; }
    static void pad(ByteBuffer b, int n) { for (int i = n; i < pad4(n); i++) b.put((byte) 0); }
    static byte[] shorts(int... vals) {
        ByteBuffer b = ByteBuffer.allocate(vals.length * 2).order(ByteOrder.LITTLE_ENDIAN);
        for (int v : vals) b.putShort((short) v);
        return b.array();
    }
    static byte[] intLE(int v) { return ByteBuffer.allocate(4).order(ByteOrder.LITTLE_ENDIAN).putInt(v).array(); }
    static byte[] concat(byte[] a, byte[] b) { byte[] o = new byte[a.length + b.length]; System.arraycopy(a, 0, o, 0, a.length); System.arraycopy(b, 0, o, a.length, b.length); return o; }
}
