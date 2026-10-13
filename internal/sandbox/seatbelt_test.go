package sandbox

import (
	"strings"
	"testing"
)

func TestSeatbeltProfile(t *testing.T) {
	sb := seatbelt{
		Read: []string{"/Users/a/Library/Application Support/Quartz/store/libraries"}, Write: []string{`/Users/a/"odd"\dir`},
		HomeRead: "/Users/a", Hide: "/Users/a/Library/Application Support/Quartz", Discord: []string{"/private/var/folders/x/T/discord-ipc-0"},
		Temp: "/private/var/folders/x/T", Cache: "/private/var/folders/x/C",
	}
	p, err := sb.profile()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`(deny default)`,
		`(allow file-read* (subpath "/Users/a"))`,
		`(deny file-read* file-write* (subpath "/Users/a/Library/Application Support/Quartz"))`,
		`(subpath "/Users/a/Library/Application Support/Quartz/store/libraries"))`,
		`(subpath "/Users/a/\"odd\"\\dir"))`,
		`(allow network-outbound (literal "/private/var/folders/x/T/discord-ipc-0"))`,
		`(prefix "/private/var/folders/x/T/hsperfdata_")`,
	} {
		if !strings.Contains(p, want) {
			t.Errorf("profile lacks %s", want)
		}
	}
	if strings.Contains(p, "(remote ip))") {
		t.Error("network allowed without the rule")
	}
	// Later rules win: the data folder is hidden after the home folder is
	// opened, and its allowed parts come back after that.
	if i, j, k := strings.Index(p, `(subpath "/Users/a"))`), strings.Index(p, "(deny file-read* file-write*"), strings.Index(p, "store/libraries"); i >= j || j >= k {
		t.Error("rule order")
	}
	sb.Network = true
	if p, _ := sb.profile(); !strings.Contains(p, "(allow network-outbound (remote ip))") {
		t.Error("network rule missing")
	}
	if _, err := (seatbelt{Read: []string{"/a\n(allow default)"}}).profile(); err == nil {
		t.Error("a path broke out of its string")
	}
}
