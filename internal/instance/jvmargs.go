package instance

import (
	"regexp"
	"slices"
	"strings"

	"quartz/internal/core/shlex"
	"quartz/internal/settings"
)

// ZGC selects the Z garbage collector (generational on Java 23+).
const ZGC = "-XX:+UseZGC"

// gcFlag matches a flag that selects a garbage collector ("-XX:+UseG1GC").
var gcFlag = regexp.MustCompile(`^-XX:\+Use[A-Za-z0-9]+GC$`)

// gcFlags lists the collectors args select; nil when they can't be split.
func gcFlags(args string) []string {
	words, err := shlex.Split(args)
	if err != nil {
		return nil
	}
	var out []string
	for _, w := range words {
		if gcFlag.MatchString(w) {
			out = append(out, w)
		}
	}
	return out
}

// HasGC reports whether args select a garbage collector.
func HasGC(args string) bool { return len(gcFlags(args)) > 0 }

// WithGC puts gc in place of the collector args select, or adds it when
// they select none. Other words keep their order (runs of spaces between
// words become one).
func WithGC(args, gc string) string {
	old := gcFlags(args)
	if len(old) == 0 {
		return strings.TrimSpace(args + " " + gc)
	}
	var out []string
	placed := false
	for _, w := range strings.Fields(args) {
		if !slices.Contains(old, w) {
			out = append(out, w)
		} else if !placed {
			out, placed = append(out, gc), true
		}
	}
	return strings.Join(out, " ")
}

// OwnJava is the Java card's override switched on from the global values,
// as switching it on in settings does: the instance then runs as it did.
func OwnJava(g settings.Java) JavaSettings {
	j := JavaSettings{Override: true, MemoryMinMB: g.MemoryMinMB, MemoryMaxMB: g.MemoryMaxMB, JVMArgs: g.JVMArgs}
	if !g.AutoPick {
		j.JavaPath = g.DefaultPath
	}
	return j
}

// PreferGC gives the instance gc unless the player picked a collector: an
// instance following the global Java settings gets its own copy of them with
// the global collector swapped for gc; one with its own settings gets gc
// only when its arguments select none. It reports whether anything changed.
func PreferGC(in *Instance, g settings.Java, gc string) bool {
	j := in.Settings.Java
	switch {
	case !j.Override:
		j = OwnJava(g)
	case HasGC(j.JVMArgs):
		return false
	}
	if args := WithGC(j.JVMArgs, gc); args != in.Settings.Java.JVMArgs || !in.Settings.Java.Override {
		j.JVMArgs = args
		in.Settings.Java = j
		return true
	}
	return false
}
