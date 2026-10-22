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

func TestWrongTypeKeepsTheRest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.json")
	if err := os.WriteFile(path, []byte(`{"name":"mine","count":"7"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(opts(path))
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Get(); got.Name != "mine" || got.Count != 1 {
		t.Fatalf("got %+v, want the name kept and the bad count at its default", got)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("the file must stay in place")
	}
	if _, err := os.Stat(path + ".corrupt"); err != nil {
		t.Fatalf("a copy should be kept aside: %v", err)
	}
	if got, err := Read(opts(path)); err != nil || got.Name != "mine" {
		t.Fatalf("Read: %+v %v", got, err)
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

func TestReadLeavesTheFileAlone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.json")
	if got, err := Read(opts(path)); err != nil || got.Name != "default" {
		t.Fatalf("missing: %+v %v", got, err)
	}
	if err := os.WriteFile(path, []byte(`{"name":"a","count":-3}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := Read(opts(path)); err != nil || got.Name != "a" || got.Count != 0 {
		t.Fatalf("normalized: %+v %v", got, err)
	}
	if err := os.WriteFile(path, []byte(`{`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(opts(path)); err == nil {
		t.Fatal("a corrupt file must be an error")
	}
	if _, err := os.Stat(path + ".corrupt"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("Read must not move a corrupt file aside")
	}
}

func TestRelocateKeepsPendingSave(t *testing.T) {
	dir := t.TempDir()
	from, to := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	_ = os.MkdirAll(from, 0o700)
	o := opts(filepath.Join(from, "x.json"))
	o.Delay = time.Hour
	s, err := Open(o)
	if err != nil {
		t.Fatal(err)
	}
	s.Set(doc{Name: "moved"})
	if err := s.Relocate(filepath.Join(to, "x.json"), func() error { return os.Rename(from, to) }); err != nil {
		t.Fatal(err)
	}
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	if got, err := Read(opts(filepath.Join(to, "x.json"))); err != nil || got.Name != "moved" {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := os.Stat(from); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the save landed in the old folder")
	}
	// A failed move keeps the old path.
	if err := s.Relocate(filepath.Join(dir, "c", "x.json"), func() error { return errors.New("in use") }); err == nil {
		t.Fatal("failed move reported success")
	}
}
