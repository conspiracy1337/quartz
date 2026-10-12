package wrapper;

import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Base64;
import java.util.List;

/**
 * Starts Minecraft with arguments read from stdin, so the session token never
 * appears on the command line (where any local user could read it with ps).
 * Also answers Java probes. Ported from Pandora's LaunchWrapper; values are
 * base64 so arguments may contain any character, including newlines.
 *
 * Protocol, one command per line, fields separated by one space:
 *   arg <value>          append a game argument
 *   prop <key> <value>   set a system property
 *   print <key>          print the property's value (base64, "-" if unset)
 *   launch <class>       call class.main(arguments) and stop reading
 *   exit                 stop
 * Must stay compatible with Java 8.
 */
public final class Main {
    private Main() {
    }

    public static void main(String[] ignored) throws Throwable {
        BufferedReader in = new BufferedReader(new InputStreamReader(System.in, StandardCharsets.UTF_8));
        Base64.Decoder dec = Base64.getDecoder();
        Base64.Encoder enc = Base64.getEncoder();
        List<String> args = new ArrayList<String>();
        String line;
        while ((line = in.readLine()) != null) {
            String[] f = line.split(" ", -1);
            String cmd = f[0];
            if (cmd.equals("exit")) {
                return;
            } else if (cmd.equals("arg") && f.length == 2) {
                args.add(text(dec, f[1]));
            } else if (cmd.equals("prop") && f.length == 3) {
                System.setProperty(text(dec, f[1]), text(dec, f[2]));
            } else if (cmd.equals("print") && f.length == 2) {
                String v = System.getProperty(text(dec, f[1]));
                System.out.println(v == null ? "-" : enc.encodeToString(v.getBytes(StandardCharsets.UTF_8)));
                System.out.flush();
            } else if (cmd.equals("launch") && f.length == 2) {
                String[] a = args.toArray(new String[0]);
                Class.forName(text(dec, f[1])).getMethod("main", String[].class).invoke(null, (Object) a);
                return;
            } else {
                throw new IllegalArgumentException("wrapper: bad command");
            }
        }
    }

    private static String text(Base64.Decoder dec, String b64) {
        return new String(dec.decode(b64), StandardCharsets.UTF_8);
    }
}
