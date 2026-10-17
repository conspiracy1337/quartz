package api

import (
	"errors"
	"testing"
)

// A tab can only pick one of the page's own folders, never a path.
func TestOpenTabFolderRejectsUnknownTabs(t *testing.T) {
	s := &Instances{}
	for _, tab := range []string{"", "..", "../..", "mods/../..", "/etc", "Mods", "config"} {
		if err := s.OpenTabFolder("abc", tab); !errors.Is(err, ErrBadTab) {
			t.Fatalf("tab %q: %v", tab, err)
		}
	}
}
