package java

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"quartz/internal/wrapperjar"
)

func newManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	m, err := New(Config{
		Dir: filepath.Join(dir, "java"), StatePath: filepath.Join(dir, "java.json"), BinDir: filepath.Join(dir, "bin"),
		Vendor: func() string { return "temurin" },
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func touch(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestCandidates(t *testing.T) {
	home := t.TempDir()
	touch(t, filepath.Join(home, ".jdks", "temurin-21", "bin", "java"))
	touch(t, filepath.Join(home, ".sdkman", "candidates", "java", "17.0.9-tem", "bin", "java"))
	touch(t, filepath.Join(home, ".minecraft", "runtime", "java-runtime-delta", "linux", "java-runtime-delta", "bin", "java"))
	env := searchEnv{goos: "linux", home: home, getenv: func(k string) string {
		if k == "JAVA_HOME" {
			return "/opt/custom-jdk"
		}
		return ""
	}}
	var got []string
	for _, c := range candidates(env) {
		if _, err := os.Stat(c.path); err == nil || c.path == "/opt/custom-jdk/bin/java" {
			got = append(got, string(c.source)+":"+c.path)
		}
	}
	for _, want := range []string{
		"Found:" + filepath.Join(home, ".jdks", "temurin-21", "bin", "java"),
		"SDKMAN:" + filepath.Join(home, ".sdkman", "candidates", "java", "17.0.9-tem", "bin", "java"),
		"Found:" + filepath.Join(home, ".minecraft", "runtime", "java-runtime-delta", "linux", "java-runtime-delta", "bin", "java"),
		"Found:/opt/custom-jdk/bin/java",
	} {
		if !slices.Contains(got, want) {
			t.Errorf("missing %s in %v", want, got)
		}
	}
}

func TestParseProbe(t *testing.T) {
	out := "MS44\nMS44LjBfNDAy\nT3JhY2xlIENvcnBvcmF0aW9u\nYW1kNjQ=\n" // 1.8 / 1.8.0_402 / Oracle Corporation / amd64
	rt, err := parseProbe("/x/java", out)
	if err != nil {
		t.Fatal(err)
	}
	if rt.Major != 8 || rt.Version != "1.8.0_402" || rt.Vendor != "Oracle Corporation" || rt.Arch != "x86_64" {
		t.Fatalf("got %+v", rt)
	}
	for _, bad := range []string{"", "MjE=\n", "!!\n!!\n!!\n!!\n", "-\n-\n-\n-\n"} {
		if _, err := parseProbe("/x/java", bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	if n, _ := majorOf("21"); n != 21 {
		t.Fatal("majorOf(21)")
	}
}

func TestCleanEnv(t *testing.T) {
	got := CleanEnv([]string{"PATH=/bin", "JAVA_TOOL_OPTIONS=-javaagent:/evil.jar", "_JAVA_OPTIONS=-Xmx1", "java_home=/x", "HOME=/h"})
	if !slices.Equal(got, []string{"PATH=/bin", "HOME=/h"}) {
		t.Fatalf("got %v", got)
	}
}

func TestManagedMarkers(t *testing.T) {
	m := newManager(t)
	dir := filepath.Join(m.cfg.Dir, "temurin-21-linux-amd64")
	touch(t, filepath.Join(dir, "jdk-21.0.5+11-jre", "bin", exe(runtime.GOOS)))
	if err := finishInstall(dir, "temurin", 21, "21.0.5+11"); err != nil {
		t.Fatal(err)
	}
	touch(t, filepath.Join(m.cfg.Dir, "broken", "bin", "java")) // no marker: ignored
	got := m.managed()
	if len(got) != 1 || got[0].Major != 21 || got[0].Source != Managed || got[0].Vendor != "Eclipse Temurin" {
		t.Fatalf("managed = %+v", got)
	}
	if err := writeMarker(dir, marker{Vendor: "temurin", Major: 21, Java: "../../../bin/sh"}); err != nil {
		t.Fatal(err)
	}
	if len(m.managed()) != 0 {
		t.Fatal("marker pointing outside its folder accepted")
	}
}

func TestPlaceLinkStaysInside(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "lib", "libjli.so"))
	if err := placeLink(root, filepath.Join(root, "bin", "libjli.so"), "../lib/libjli.so"); err != nil {
		t.Fatal(err)
	}
	if err := placeLink(root, filepath.Join(root, "bin", "evil"), "../../../etc/passwd"); err == nil {
		t.Fatal("escaping link accepted")
	}
	links := map[string]string{filepath.Join(root, "d"): ".", filepath.Join(root, "d", "l"): "../x"}
	for l, tgt := range links {
		_ = placeLink(root, l, tgt)
	}
	if err := checkLinks(root, links); err == nil {
		t.Fatal("link chain escaping the runtime accepted")
	}
}

// TestProbeRealJava runs when QUARTZ_TEST_JAVA points at a java binary.
func TestProbeRealJava(t *testing.T) {
	bin := os.Getenv("QUARTZ_TEST_JAVA")
	if bin == "" {
		t.Skip("set QUARTZ_TEST_JAVA to a java binary")
	}
	m := newManager(t)
	rt, err := m.Probe(context.Background(), bin)
	if err != nil {
		t.Fatal(err)
	}
	if rt.Major < 8 || rt.Version == "" || rt.Arch == "" {
		t.Fatalf("probe = %+v", rt)
	}
	if _, err := os.Stat(filepath.Join(m.cfg.BinDir, "wrapper-"+wrapperjar.SHA256[:12]+".jar")); err != nil {
		t.Fatal("wrapper jar not installed")
	}
	t.Logf("probed %+v", rt)
}
