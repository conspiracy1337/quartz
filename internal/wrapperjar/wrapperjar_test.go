package wrapperjar

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEmbeddedJarMatchesPin(t *testing.T) {
	if got := embeddedHash(); got != SHA256 {
		t.Fatalf("wrapper.jar hash %s, pinned %s: rebuild with tools/wrapperjar/build.sh and update SHA256", got, SHA256)
	}
}

func TestInstallRepairsTampering(t *testing.T) {
	dir := t.TempDir()
	p, err := Install(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("evil"), 0o600); err != nil {
		t.Fatal(err)
	}
	if p2, err := Install(dir); err != nil || p2 != p {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Clean(p)); string(data) == "evil" {
		t.Fatal("tampered jar not replaced")
	}
}

func TestScript(t *testing.T) {
	got := string(new(Script).Arg("--username").Arg("a b\nc").Prop("k", "v").Print("java.version").Launch("net.minecraft.client.main.Main").Bytes())
	want := "arg LS11c2VybmFtZQ==\narg YSBiCmM=\nprop aw== dg==\nprint amF2YS52ZXJzaW9u\nlaunch bmV0Lm1pbmVjcmFmdC5jbGllbnQubWFpbi5NYWlu\n"
	if got != want {
		t.Fatalf("script = %q", got)
	}
	if v, ok, err := ParseValue("MjE=\n"); err != nil || !ok || v != "21" {
		t.Fatalf("ParseValue = %q %v %v", v, ok, err)
	}
	if _, ok, err := ParseValue("-"); err != nil || ok {
		t.Fatal("unset property misparsed")
	}
	if _, _, err := ParseValue("!!"); err == nil {
		t.Fatal("garbage accepted")
	}
}
