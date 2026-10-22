package instance

import (
	"testing"

	"quartz/internal/settings"
)

func TestWithGC(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"", ZGC},
		{"-XX:+UseG1GC -XX:+ParallelRefProcEnabled", ZGC + " -XX:+ParallelRefProcEnabled"},
		{"-Dx=1  -XX:+UseG1GC -XX:+UseParallelGC", "-Dx=1 " + ZGC},
		{"-XX:+ParallelRefProcEnabled", "-XX:+ParallelRefProcEnabled " + ZGC},
		{"-XX:+UseGCOverheadLimit", "-XX:+UseGCOverheadLimit " + ZGC}, // not a collector
	} {
		if got := WithGC(c.in, ZGC); got != c.want {
			t.Errorf("WithGC(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPreferGC(t *testing.T) {
	g := settings.Java{AutoPick: true, MemoryMinMB: 512, MemoryMaxMB: 4096, JVMArgs: "-XX:+UseG1GC -XX:+ParallelRefProcEnabled"}

	// Following the globals: its own copy, the collector swapped.
	var in Instance
	if !PreferGC(&in, g, ZGC) {
		t.Fatal("no change")
	}
	if j := in.Settings.Java; !j.Override || j.MemoryMaxMB != 4096 || j.JVMArgs != ZGC+" -XX:+ParallelRefProcEnabled" || j.JavaPath != "" {
		t.Fatalf("%+v", j)
	}
	// Its own settings with a collector: left alone (the player picked it).
	in.Settings.Java.JVMArgs = "-XX:+UseShenandoahGC"
	if PreferGC(&in, g, ZGC) || in.Settings.Java.JVMArgs != "-XX:+UseShenandoahGC" {
		t.Fatal("a picked collector was changed")
	}
	// Its own settings without one: added.
	in.Settings.Java.JVMArgs = "-Dfoo=bar"
	if !PreferGC(&in, g, ZGC) || in.Settings.Java.JVMArgs != "-Dfoo=bar "+ZGC {
		t.Fatalf("%q", in.Settings.Java.JVMArgs)
	}
	// A pinned global Java carries over.
	var pinned Instance
	PreferGC(&pinned, settings.Java{DefaultPath: "/opt/java/bin/java", JVMArgs: "-XX:+UseG1GC"}, ZGC)
	if pinned.Settings.Java.JavaPath != "/opt/java/bin/java" {
		t.Fatal("pinned java lost")
	}
}
