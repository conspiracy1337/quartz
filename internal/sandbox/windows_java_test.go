//go:build windows

package sandbox

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"quartz/internal/core/procx"
	"quartz/internal/sandbox/rules"
)

// TestJavaSecurityLoads starts a real Java inside the AppContainer and makes
// it load its security properties. Since JDK 24 that goes through
// Path.toRealPath(), which on Windows looks up every folder of java.home in
// its parent (FindFirstFile), so the container must be able to see each one;
// without that every Java 25 game (Minecraft 26.x) dies at startup with
// "Error loading java.security file". Needs a real Java: QUARTZ_JAVA_BIN names
// its java.exe (use 24 or newer to cover the toRealPath walk).
func TestJavaSecurityLoads(t *testing.T) {
	javaBin := os.Getenv("QUARTZ_JAVA_BIN")
	if javaBin == "" {
		t.Skip("set QUARTZ_JAVA_BIN to a java.exe to run the sandboxed Java check")
	}
	root := jailRoot(t, "java-")
	game, data, scratch := filepath.Join(root, "instances", "a", "game"), filepath.Join(root, "instances", "a", "sandbox"), filepath.Join(root, "instances", "a", "runs", "1.view")
	for _, d := range []string{game, scratch} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	r := Run{Instance: "java-security-test", Rules: rules.Default(), GameDir: game, Scratch: scratch, Data: data, Java: JavaRoot(javaBin)}
	spec, j, err := New(Config{DataRoot: root}).Wrap(r, procx.Spec{
		Argv: []string{javaBin, "-XshowSettings:security:properties", "-version"}, Dir: game, Env: FilterEnv(os.Environ()),
	})
	if err != nil {
		t.Fatalf("Wrap: %v", err)
	}
	c, err := procx.Start(spec)
	if err != nil {
		j.Close()
		t.Fatalf("Start: %v", err)
	}
	j.Started(c.Process.Pid)
	_ = c.Stdin.Close()
	var out strings.Builder
	done := make(chan struct{})
	go func() { _, _ = io.Copy(&out, c.Stderr); close(done) }()
	_, _ = io.Copy(&out, c.Stdout)
	<-done
	werr := c.Wait()
	j.Close()
	if werr != nil || !strings.Contains(out.String(), "security.provider.1") {
		t.Fatalf("sandboxed Java couldn't load its security properties (%v):\n%s", werr, out.String())
	}
}
