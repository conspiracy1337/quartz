package launch

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestLdConf(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "ld.so.conf.d"), 0o700)
	_ = os.WriteFile(filepath.Join(dir, "ld.so.conf"), []byte("# system\ninclude ld.so.conf.d/*.conf\n/opt/top/lib\nrelative/ignored\n"), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "ld.so.conf.d", "x86_64-linux-gnu.conf"), []byte("/usr/local/lib/x86_64-linux-gnu # multiarch\n"), 0o600)
	got := ldConf(filepath.Join(dir, "ld.so.conf"), 0)
	if !slices.Equal(got, []string{"/usr/local/lib/x86_64-linux-gnu", "/opt/top/lib"}) {
		t.Fatalf("%q", got)
	}
}
