package launch

import (
	"slices"
	"testing"
)

func TestZGCArgs(t *testing.T) {
	for _, c := range []struct {
		user  []string
		major int
		want  []string
	}{
		{[]string{"-XX:+UseZGC"}, 21, []string{"-XX:+ZGenerational"}},
		{[]string{"-XX:+UseZGC"}, 22, []string{"-XX:+ZGenerational"}},
		{[]string{"-XX:+UseZGC"}, 25, nil},                       // generational by default
		{[]string{"-XX:+UseZGC"}, 17, nil},                       // no generational ZGC yet
		{[]string{"-XX:+UseZGC", "-XX:-ZGenerational"}, 21, nil}, // the player's choice
		{[]string{"-XX:+UseG1GC"}, 21, nil},
	} {
		if got := zgcArgs(c.user, c.major); !slices.Equal(got, c.want) {
			t.Errorf("zgcArgs(%v, %d) = %v, want %v", c.user, c.major, got, c.want)
		}
	}
}
