package accounts

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOfflineUUIDMatchesJava(t *testing.T) {
	// From java.util.UUID.nameUUIDFromBytes("OfflinePlayer:" + name), with
	// the parity of UUID.hashCode that picks the default skin.
	cases := []struct {
		name, uuid string
		alex       bool
	}{
		{"Notch", "b50ad385-829d-3141-a216-7e7d7539ba7f", false},
		{"jeb_", "a762f560-4fce-3236-812a-b80efff0b62b", true},
		{"Steve", "5627dd98-e6be-3c21-b8a8-e92344183641", true},
		{"Alex_2", "3a4192db-3a36-3ae7-9410-a2056ece9f53", false},
	}
	for _, c := range cases {
		got := OfflineUUID(c.name)
		if got != c.uuid {
			t.Errorf("OfflineUUID(%q) = %s, want %s", c.name, got, c.uuid)
		}
		a := Account{ID: strings.ReplaceAll(got, "-", "")}
		if (a.DefaultSkin() == alex) != c.alex {
			t.Errorf("%s: default skin parity wrong", c.name)
		}
	}
}

func TestStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	s, err := Open(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Default(); !errors.Is(err, ErrNone) {
		t.Fatalf("empty store default: %v", err)
	}
	for _, bad := range []string{"ab", "has space", "seventeen_chars__", "../x", ""} {
		if _, err := s.AddOffline(bad); !errors.Is(err, ErrBadName) {
			t.Errorf("AddOffline(%q) = %v", bad, err)
		}
	}
	a, err := s.AddOffline("Steve")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddOffline("Steve"); !errors.Is(err, ErrExists) {
		t.Fatal("duplicate accepted")
	}
	b, _ := s.AddOffline("Alex_2")
	if d, _ := s.Default(); d.ID != a.ID {
		t.Fatal("first account is not the default")
	}
	if err := s.SetDefault(b.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(b.ID); err != nil {
		t.Fatal(err)
	}
	if d, _ := s.Default(); d.ID != a.ID {
		t.Fatal("default not moved after removing it")
	}
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	if sess := a.Session(); sess.UUID != a.ID || sess.Name != "Steve" || len(sess.UUID) != 32 {
		t.Fatalf("session %+v", sess)
	}
}

func TestHandEditedFileIsSanitised(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	data := `{"accounts":[{"id":"../../etc","name":"x","kind":"offline"},{"id":"b50ad385829d3141a2167e7d7539ba7f","name":"Notch","kind":"offline"}],"default":"missing"}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	list, def := s.List()
	if len(list) != 1 || def != "b50ad385829d3141a2167e7d7539ba7f" {
		t.Fatalf("list=%+v default=%q", list, def)
	}
}
