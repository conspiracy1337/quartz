package sandbox

import (
	"strings"
	"testing"
)

func TestSeatbeltProfile(t *testing.T) {
	sb := seatbelt{
		Read: []string{"/Users/a/Library/Application Support/Quartz/store/libraries"}, Write: []string{`/Users/a/"odd"\dir`}, Protect: []string{`/Users/a/"odd"\dir/mods`},
		Discord: []string{"/private/var/folders/x/T/discord-ipc-0"},
		Temp:    "/private/var/folders/x/T", Cache: "/private/var/folders/x/C",
	}
	p, err := sb.profile()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`(deny default)`,
		`(subpath "/Users/a/Library/Application Support/Quartz/store/libraries"))`,
		`(subpath "/Users/a/\"odd\"\\dir"))`,
		`(allow network-outbound (literal "/private/var/folders/x/T/discord-ipc-0"))`,
		`(prefix "/private/var/folders/x/T/hsperfdata_")`,
	} {
		if !strings.Contains(p, want) {
			t.Errorf("profile lacks %s", want)
		}
	}
	// The mods folder stays read-only inside the writable game folder, and
	// nothing hands a file to an app outside the sandbox.
	if i, j := strings.Index(p, `(allow file-write* file-link`), strings.Index(p, `(deny file-write* file-link (subpath "/Users/a/\"odd\"\\dir/mods"))`); j < 0 || i >= j {
		t.Error("the mods folder isn't denied after the game folder's write rule")
	}
	if strings.Contains(p, "process-fork lsopen") || !strings.Contains(p, "(deny lsopen)") || !strings.Contains(p, "(deny appleevent-send)") {
		t.Error("the profile lets the game open files in other apps")
	}
	if !strings.Contains(p, "(allow network-outbound (remote ip))") {
		t.Error("network rule missing")
	}
	if _, err := (seatbelt{Read: []string{"/a\n(allow default)"}}).profile(); err == nil {
		t.Error("a path broke out of its string")
	}
}
