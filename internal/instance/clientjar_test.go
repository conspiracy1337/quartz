package instance

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const jarSHA1 = "0123456789abcdef0123456789abcdef01234567"

func writeJar(body string) func(string) error {
	return func(dst string) error { return os.WriteFile(dst, []byte(body), 0o600) }
}

func TestClientJar(t *testing.T) {
	s, _ := newStore(t)
	in := create(t, s, "Modified")
	got, err := s.SetClientJar(in.ID, ClientJar{SHA1: jarSHA1, Name: "../My\x00 Client.jar", Size: 3}, writeJar("jar"))
	if err != nil {
		t.Fatal(err)
	}
	if got.ClientJar == nil || got.ClientJar.Name != "My Client.jar" {
		t.Fatalf("jar = %+v", got.ClientJar)
	}
	if data, err := os.ReadFile(s.ClientJarPath(in.ID)); err != nil || string(data) != "jar" {
		t.Fatalf("client.jar = %q, %v", data, err)
	}
	if _, err := os.Stat(s.ClientJarPath(in.ID) + ".part"); err == nil {
		t.Fatal("the partial file was left behind")
	}

	// The jar pins the version and keeps loaders out.
	if _, err := s.SetVersion(in.ID, "1.21.4", "", ""); !errors.Is(err, ErrClientJarLocked) {
		t.Errorf("version change = %v", err)
	}
	if _, err := s.SetVersion(in.ID, in.Minecraft, "fabric", "0.16.10"); !errors.Is(err, ErrClientJarLocked) {
		t.Errorf("loader = %v", err)
	}
	if got, _ := s.SetVersionOptions(in.ID, true, true); got.FollowLatest {
		t.Error("a custom jar can't follow the latest release")
	}

	// A duplicate brings its own copy.
	cp, err := s.Duplicate(context.Background(), in.ID, nil, nil)
	if err != nil || cp.ClientJar == nil {
		t.Fatalf("duplicate = %+v, %v", cp.ClientJar, err)
	}
	if data, err := os.ReadFile(s.ClientJarPath(cp.ID)); err != nil || string(data) != "jar" {
		t.Fatalf("copy's client.jar = %q, %v", data, err)
	}

	got, err = s.ClearClientJar(in.ID)
	if err != nil || got.ClientJar != nil {
		t.Fatalf("clear = %+v, %v", got.ClientJar, err)
	}
	if _, err := os.Stat(s.ClientJarPath(in.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("client.jar is still there")
	}
	if _, err := s.SetVersion(in.ID, "1.21.4", "", ""); err != nil {
		t.Fatalf("version change after clearing = %v", err)
	}
}

func TestClientJarRefusals(t *testing.T) {
	s, _ := newStore(t)
	modded, err := s.Create(CreateRequest{Name: "Fabric", Minecraft: "1.21.1", Loader: "fabric", LoaderVersion: "0.16.10"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetClientJar(modded.ID, ClientJar{SHA1: jarSHA1, Name: "a.jar", Size: 1}, writeJar("x")); !errors.Is(err, ErrClientJarLocked) {
		t.Errorf("with a loader = %v", err)
	}
	in := create(t, s, "Vanilla")
	if _, err := s.SetClientJar(in.ID, ClientJar{SHA1: "nope", Name: "a.jar", Size: 1}, writeJar("x")); !errors.Is(err, ErrInvalidSettings) {
		t.Errorf("bad sha1 = %v", err)
	}
	failed := errors.New("disk full")
	if _, err := s.SetClientJar(in.ID, ClientJar{SHA1: jarSHA1, Name: "a.jar", Size: 1}, func(string) error { return failed }); !errors.Is(err, failed) {
		t.Errorf("failed place = %v", err)
	}
	if got, _ := s.Get(in.ID); got.ClientJar != nil {
		t.Error("a failed place recorded the jar")
	}
	entries, _ := os.ReadDir(s.Dir(in.ID))
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".part" || e.Name() == clientJarFile {
			t.Errorf("left behind %s", e.Name())
		}
	}
}

func TestNormalizeClientJar(t *testing.T) {
	in := normalize(Instance{ID: "a", Name: "A", Loader: "fabric", LoaderVersion: "0.16.10", FollowLatest: true, ClientJar: &ClientJar{SHA1: jarSHA1, Name: "x.jar", Size: 9}})
	if in.ClientJar == nil || in.Loader != "" || in.FollowLatest {
		t.Fatalf("a jar wins over a hand-added loader: %+v", in)
	}
	for _, bad := range []ClientJar{{SHA1: "zz", Name: "x", Size: 1}, {SHA1: jarSHA1, Name: "x", Size: 0}} {
		if got := normalize(Instance{ID: "a", Name: "A", ClientJar: &bad}); got.ClientJar != nil {
			t.Errorf("kept %+v", bad)
		}
	}
}
