package jsonfile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type doc struct {
	Name  string   `json:"name"`
	Count int      `json:"count"`
	Tags  []string `json:"tags"`
}

func opts(path string) Options[doc] {
	return Options[doc]{
		Path:    path,
		Default: func() doc { return doc{Name: "default", Count: 1} },
		Normalize: func(d doc) doc {
			if d.Count < 0 {
				d.Count = 0
			}
			return d
		},
		Delay: 5 * time.Millisecond,
	}
}

func TestMissingFileYieldsDefault(t *testing.T) {
	s, err := Open(opts(filepath.Join(t.TempDir(), "x.json")))
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Get(); got.Name != "default" || got.Count != 1 {
		t.Fatalf("got %+v", got)
	}
}

func TestDecodeKeepsDefaultsAndNormalizes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.json")
	if err := os.WriteFile(path, []byte(`{"count":-5,"extra":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(opts(path))
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Get(); got.Name != "default" || got.Count != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestCorruptFileIsKeptAside(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.json")
	if err := os.WriteFile(path, []byte(`{not json`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(opts(path))
	if err != nil {
		t.Fatal(err)
	}
	if s.Get().Name != "default" {
		t.Fatal("corrupt file not replaced by default")
	}
	if _, err := os.Stat(path + ".corrupt"); err != nil {
		t.Fatalf("corrupt copy missing: %v", err)
	}
}

func TestUpdateDeepCopiesAndPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.json")
	s, err := Open(opts(path))
	if err != nil {
		t.Fatal(err)
	}
	s.Set(doc{Name: "a", Tags: []string{"x"}})
	before := s.Get()
	if _, err := s.Update(func(d *doc) error { d.Tags[0] = "changed"; d.Count = 7; return nil }); err != nil {
		t.Fatal(err)
	}
	if before.Tags[0] != "x" {
		t.Fatal("Update mutated a value a reader holds")
	}
	if _, err := s.Update(func(d *doc) error { d.Count = 99; return errors.New("no") }); err == nil {
		t.Fatal("error not returned")
	}
	if s.Get().Count != 7 {
		t.Fatal("failed update was applied")
	}
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	re, err := Open(opts(path))
	if err != nil {
		t.Fatal(err)
	}
	if got := re.Get(); got.Count != 7 || got.Tags[0] != "changed" {
		t.Fatalf("reloaded %+v", got)
	}
}

func TestDebouncedSaveLands(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.json")
	s, err := Open(opts(path))
	if err != nil {
		t.Fatal(err)
	}
	s.Set(doc{Name: "later"})
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("debounced save never happened")
}
