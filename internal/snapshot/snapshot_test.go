package snapshot

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"quartz/internal/content"
	"quartz/internal/content/store"
	"quartz/internal/core/events"
	"quartz/internal/instance"
)

type env struct {
	t       *testing.T
	ctx     context.Context
	svc     *Service
	content *content.Service
	store   *store.Store
	insts   *instance.Store
	id      string
	game    string
	now     time.Time
	on      bool
}

func newEnv(t *testing.T, inPlace bool) *env {
	t.Helper()
	root := t.TempDir()
	log := slog.New(slog.DiscardHandler)
	insts, err := instance.Open(filepath.Join(root, "instances"), log)
	if err != nil {
		t.Fatal(err)
	}
	in, err := insts.Create(instance.CreateRequest{Name: "Test", Minecraft: "1.21.1", Loader: "fabric", LoaderVersion: "0.16.9"})
	if err != nil {
		t.Fatal(err)
	}
	if inPlace {
		if _, err := insts.Update(in.ID, func(i *instance.Instance) error { i.InPlace = true; return nil }); err != nil {
			t.Fatal(err)
		}
	}
	st, err := store.Open(filepath.Join(root, "store", "content"), nil, log)
	if err != nil {
		t.Fatal(err)
	}
	bus := events.NewBus()
	cs, err := content.New(content.Deps{Instances: insts, Store: st, Bus: bus, Log: log}, filepath.Join(root, "store", "content"))
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, ctx: context.Background(), content: cs, store: st, insts: insts, id: in.ID, game: insts.GameDir(in.ID),
		now: time.UnixMilli(1_800_000_000_000), on: true}
	e.svc = New(Deps{Instances: insts, Content: cs, Store: st, Bus: bus, Log: log, Enabled: func() bool { return e.on }})
	e.svc.now = func() time.Time { return e.now }
	t.Cleanup(e.svc.Close)
	return e
}

// mod writes a Fabric mod jar.
func (e *env) mod(file, id, version string) {
	e.t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("fabric.mod.json")
	_, _ = w.Write([]byte(`{"id":"` + id + `","name":"` + id + `","version":"` + version + `"}`))
	_ = zw.Close()
	e.write("mods/"+file, buf.String())
}

func (e *env) write(rel, body string) {
	e.t.Helper()
	p := filepath.Join(e.game, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		e.t.Fatal(err)
	}
	_ = os.Chmod(p, 0o600) // a placed mod is a read-only store link
	_ = os.Remove(p)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) read(rel string) string {
	b, err := os.ReadFile(filepath.Join(e.game, filepath.FromSlash(rel)))
	if err != nil {
		return "<missing>"
	}
	return string(b)
}

func (e *env) exists(rel string) bool {
	_, err := os.Lstat(filepath.Join(e.game, filepath.FromSlash(rel)))
	return err == nil
}

func (e *env) take(title string, scope ...string) string {
	e.t.Helper()
	id := e.svc.NewID()
	if err := e.svc.Take(e.ctx, e.id, content.SnapshotReq{ID: id, Title: title, Reason: "update", Scope: scope}); err != nil {
		e.t.Fatal(err)
	}
	e.now = e.now.Add(time.Second)
	return id
}

func (e *env) diff(id string) Diff {
	e.t.Helper()
	d, err := e.svc.Diff(e.ctx, e.id, id)
	if err != nil {
		e.t.Fatal(err)
	}
	return d
}

func row(d Diff, name string) Row {
	for _, r := range d.Rows {
		if r.Name == name {
			return r
		}
	}
	return Row{}
}

// setup is an instance before an update: three mods and their config.
func (e *env) setup() {
	e.mod("a-1.0.jar", "a", "1.0")
	e.mod("b.jar", "b", "1.0")
	e.mod("c.jar", "c", "1.0")
	e.write("config/a.toml", "x=1")
	e.write("config/gone.toml", "old")
}

// update changes everything after a snapshot: a renamed new version, b
// replaced in place, c turned off, d added, config edited.
func (e *env) update() {
	_ = os.Remove(filepath.Join(e.game, "mods", "a-1.0.jar"))
	e.mod("a-2.0.jar", "a", "2.0")
	e.mod("b.jar", "b", "2.0")
	if err := os.Rename(filepath.Join(e.game, "mods", "c.jar"), filepath.Join(e.game, "mods", "c.jar.disabled")); err != nil {
		e.t.Fatal(err)
	}
	e.mod("d.jar", "d", "1.0")
	e.write("config/a.toml", "x=2")
	e.write("config/new.toml", "new")
	_ = os.Remove(filepath.Join(e.game, "config", "gone.toml"))
}

func TestDiffAndPartialRollback(t *testing.T) {
	e := newEnv(t, false)
	e.setup()
	snap := e.take("updating 2 mods", content.ScopeRel("mods/a-1.0.jar"), content.ScopeRel("mods/b.jar"))
	e.update()

	d := e.diff(snap)
	want := map[string][3]string{ // name → change, from, to
		"a": {"changed", "2.0", "1.0"}, "b": {"changed", "2.0", "1.0"}, "c": {"enable", "1.0", "1.0"}, "d": {"remove", "1.0", ""},
		"Config files": {"set", "", ""},
	}
	for name, w := range want {
		r := row(d, name)
		if r.Change != w[0] || r.From != w[1] || r.To != w[2] || r.Unavailable != "" {
			t.Fatalf("%s: %+v", name, r)
		}
	}
	if !row(d, "a").Default || !row(d, "b").Default || row(d, "c").Default || row(d, "d").Default || row(d, "Config files").Default {
		t.Fatalf("defaults follow the scope: %+v", d.Rows)
	}
	if r := row(d, "Config files"); r.Detail != "2 files come back, 1 new file goes" {
		t.Fatalf("files: %q", r.Detail)
	}

	// Only a goes back: the bisecting case.
	res, err := e.svc.Rollback(e.ctx, e.id, snap, []string{row(d, "a").Key})
	if err != nil || res.Restored != 1 || res.Snapshot == "" {
		t.Fatalf("%+v %v", res, err)
	}
	if !e.exists("mods/a-1.0.jar") || e.exists("mods/a-2.0.jar") {
		t.Fatal("a wasn't put back")
	}
	if !e.exists("mods/d.jar") || !e.exists("mods/c.jar.disabled") || e.read("config/a.toml") != "x=2" {
		t.Fatal("a partial rollback changed more than it was asked")
	}

	// Undoing the rollback brings a 2.0 back.
	e.now = e.now.Add(time.Second)
	if _, err := e.svc.Undo(e.ctx, e.id, res.Snapshot); err != nil {
		t.Fatal(err)
	}
	if e.exists("mods/a-1.0.jar") || !e.exists("mods/a-2.0.jar") {
		t.Fatal("the rollback wasn't undone")
	}

	// Everything goes back.
	var keys []string
	for _, r := range e.diff(snap).Rows {
		keys = append(keys, r.Key)
	}
	if _, err := e.svc.Rollback(e.ctx, e.id, snap, keys); err != nil {
		t.Fatal(err)
	}
	for rel, body := range map[string]string{"config/a.toml": "x=1", "config/gone.toml": "old", "config/new.toml": "<missing>"} {
		if got := e.read(rel); got != body {
			t.Fatalf("%s = %q", rel, got)
		}
	}
	for _, rel := range []string{"mods/a-1.0.jar", "mods/b.jar", "mods/c.jar"} {
		if !e.exists(rel) {
			t.Fatalf("%s missing", rel)
		}
	}
	if e.exists("mods/d.jar") || e.exists("mods/c.jar.disabled") || e.exists("mods/a-2.0.jar") {
		t.Fatal("files added since are still there")
	}
	if rows := e.diff(snap).Rows; len(rows) != 0 {
		t.Fatalf("nothing left to roll back: %+v", rows)
	}
	entries, _ := os.ReadDir(filepath.Join(e.game, "mods"))
	for _, de := range entries {
		if filepath.Ext(de.Name()) != ".jar" && filepath.Ext(de.Name()) != ".disabled" && de.Name() != ".index" {
			t.Fatalf("left over: %s", de.Name())
		}
	}
}

func TestUnchangedConfigIsLinkedNotCopied(t *testing.T) {
	e := newEnv(t, false)
	e.setup()
	first := e.take("first")
	e.write("config/a.toml", "x=2")
	second := e.take("second")
	same := func(rel string) bool {
		a, err1 := os.Stat(filepath.Join(e.svc.dir(e.id), first, filesDir, rel))
		b, err2 := os.Stat(filepath.Join(e.svc.dir(e.id), second, filesDir, rel))
		return err1 == nil && err2 == nil && os.SameFile(a, b)
	}
	if !same("config/gone.toml") {
		t.Fatal("an unchanged file was copied again")
	}
	if same("config/a.toml") {
		t.Fatal("a changed file was linked")
	}
	sn, err := read(e.svc.dir(e.id), second)
	if err != nil || sn.Size != int64(len("x=2")) {
		t.Fatalf("size counts copies only: %+v %v", sn.Size, err)
	}
}

func TestVersionRollsBackWithTheMods(t *testing.T) {
	e := newEnv(t, false)
	e.setup()
	snap := e.take("moving to Minecraft 1.21.4", content.ScopeVersion)
	if _, err := e.insts.SetVersion(e.id, "1.21.4", "fabric", "0.16.10"); err != nil {
		t.Fatal(err)
	}
	r := row(e.diff(snap), "Minecraft and loader")
	if r.From != "1.21.4, Fabric 0.16.10" || r.To != "1.21.1, Fabric 0.16.9" || !r.Default {
		t.Fatalf("%+v", r)
	}
	if _, err := e.svc.Undo(e.ctx, e.id, snap); err != nil {
		t.Fatal(err)
	}
	if in, _ := e.insts.Get(e.id); in.Minecraft != "1.21.1" || in.LoaderVersion != "0.16.9" {
		t.Fatalf("%+v", in)
	}
}

func TestFailedCommitPutsEveryFileBack(t *testing.T) {
	e := newEnv(t, false)
	e.setup()
	snap := e.take("updating a")
	e.update()
	if _, err := e.insts.SetVersion(e.id, "1.21.4", "fabric", "0.16.10"); err != nil {
		t.Fatal(err)
	}
	// A version the instance store refuses fails the commit, last.
	dir := filepath.Join(e.svc.dir(e.id), snap)
	sn, _ := read(e.svc.dir(e.id), snap)
	sn.Version.Minecraft = "not a version!"
	if err := write(dir, sn); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Rollback(e.ctx, e.id, snap, []string{"version", row(e.diff(snap), "a").Key, "files"}); err == nil {
		t.Fatal("the rollback succeeded")
	}
	if !e.exists("mods/a-2.0.jar") || e.exists("mods/a-1.0.jar") || e.read("config/a.toml") != "x=2" || !e.exists("config/new.toml") {
		t.Fatal("files weren't put back after the commit failed")
	}
}

func TestInPlaceFilesAreKeptAndRestoredAsCopies(t *testing.T) {
	e := newEnv(t, true)
	e.setup()
	snap := e.take("updating a")
	sn, _ := read(e.svc.dir(e.id), snap)
	var a content.StateItem
	for _, it := range sn.State.Items {
		if it.Name == "a" {
			a = it
		}
	}
	if _, ok := e.store.Blob(a.SHA1); !ok || a.Lost {
		t.Fatal("the in-place file wasn't kept in the store")
	}
	e.update()
	if _, err := e.svc.Rollback(e.ctx, e.id, snap, []string{row(e.diff(snap), "a").Key}); err != nil {
		t.Fatal(err)
	}
	got, err1 := os.Stat(filepath.Join(e.game, "mods", "a-1.0.jar"))
	blob, err2 := os.Stat(e.store.Path(a.SHA1))
	if err1 != nil || err2 != nil || os.SameFile(got, blob) {
		t.Fatal("another launcher's folder got a hardlink")
	}
}

func TestExpiryKeepAndTurningOff(t *testing.T) {
	e := newEnv(t, false)
	defer func(age time.Duration) { releaseAge = age }(releaseAge)
	releaseAge = 0 // the store's clock is the real one
	e.setup()
	old := e.take("updating a")
	kept := e.take("updating b")
	if _, err := e.svc.SetKeep(e.id, kept, true); err != nil {
		t.Fatal(err)
	}
	sn, _ := read(e.svc.dir(e.id), old)
	var a content.StateItem
	for _, it := range sn.State.Items {
		if it.Name == "a" {
			a = it
		}
	}
	e.update() // a-1.0 is now only in the snapshots
	if err := e.content.Scan(e.ctx, e.id, content.Mods); err != nil {
		t.Fatal(err)
	}
	e.now = e.now.Add(Life + time.Hour)
	if got := e.svc.List(e.id); len(got) != 1 || got[0].ID != kept || got[0].Expires != 0 {
		t.Fatalf("list %+v", got)
	}
	e.svc.Purge(e.now)
	if _, err := os.Stat(filepath.Join(e.svc.dir(e.id), old)); !os.IsNotExist(err) {
		t.Fatal("an expired snapshot was kept")
	}
	if _, ok := e.store.Blob(a.SHA1); !ok {
		t.Fatal("a kept snapshot's file was freed")
	}
	e.on = false
	e.svc.Purge(e.now)
	if len(e.svc.List(e.id)) != 1 {
		t.Fatal("turning snapshots off deleted a kept one")
	}
	if err := e.svc.Delete(e.id, kept); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.store.Blob(a.SHA1); ok {
		t.Fatal("deleting the last snapshot didn't free its old file")
	}
	if e.svc.NewID() != "" {
		t.Fatal("snapshots are off")
	}
}

func TestTrashedInstanceKeepsItsSnapshotsReferenced(t *testing.T) {
	e := newEnv(t, false)
	e.setup()
	e.take("updating a")
	if _, err := e.insts.Trash(e.id, e.now); err != nil {
		t.Fatal(err)
	}
	var refs []string
	_ = e.svc.Refs(func(h string) { refs = append(refs, h) })
	if len(refs) != 3 {
		t.Fatalf("refs %v", refs)
	}
}

func TestTamperedSnapshotWritesNothingOutside(t *testing.T) {
	e := newEnv(t, false)
	e.setup()
	snap := e.take("updating a")
	dir := filepath.Join(e.svc.dir(e.id), snap)
	sn, _ := read(e.svc.dir(e.id), snap)
	good := sn.State.Items[0]
	for _, rel := range []string{"../../evil.jar", "mods/../../evil.jar", "/abs/evil.jar", "saves/w/level.jar", "mods/sub/x.jar", "config/x.jar"} {
		it := good
		it.Rel, it.ID = rel, "0123456789abcdef"
		sn.State.Items = append(sn.State.Items, it)
	}
	for _, rel := range []string{"../evil.toml", "saves/w/level.dat", "mods/x.jar", ".quartz/removed/x", "config/x.toml" + instance.NewSuffix} {
		sn.Files = append(sn.Files, File{Rel: rel, Size: 1, SHA1: good.SHA1})
	}
	// A kept copy swapped for a link out of the snapshot.
	outside := filepath.Join(t.TempDir(), "secret")
	_ = os.WriteFile(outside, []byte("old"), 0o600)
	link := filepath.Join(dir, filesDir, "config", "gone.toml")
	_ = os.Remove(link)
	if err := os.Symlink(outside, link); err != nil {
		t.Skip("no symlinks here")
	}
	if err := write(dir, sn); err != nil {
		t.Fatal(err)
	}
	got, err := read(e.svc.dir(e.id), snap)
	if err != nil || len(got.State.Items) != 3 || slices.ContainsFunc(got.Files, func(f File) bool { return !inTree(f.Rel, Trees) }) {
		t.Fatalf("tampered entries were read: %+v", got)
	}
	_ = os.Remove(filepath.Join(e.game, "config", "gone.toml"))
	if _, err := e.svc.Rollback(e.ctx, e.id, snap, []string{"files"}); err == nil {
		t.Fatalf("a linked copy was restored: %v", err)
	}
	if e.exists("config/gone.toml") {
		t.Fatal("a file came back from outside the snapshot")
	}
	// A snapshot folder whose json names another id is ignored.
	sn.ID = "1111111111111-abcd"
	data, _ := json.Marshal(sn)
	_ = os.WriteFile(filepath.Join(dir, metaName), data, 0o600)
	if _, err := e.svc.Diff(e.ctx, e.id, snap); !errors.Is(err, ErrUnknown) {
		t.Fatalf("mismatched id read: %v", err)
	}
}

func TestAroundSnapshotsAndRecordsTheLoaderUpdate(t *testing.T) {
	e := newEnv(t, false)
	e.setup()
	err := e.svc.Around(e.ctx, e.id, "updating Fabric 0.16.10", "Updated Fabric 0.16.10", func() error {
		_, err := e.insts.Update(e.id, func(i *instance.Instance) error { i.LoaderVersion = "0.16.10"; return nil })
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	list := e.svc.List(e.id)
	if len(list) != 1 || list[0].Title != "updating Fabric 0.16.10" || list[0].Reason != "loader" {
		t.Fatalf("%+v", list)
	}
	if a := e.content.Activity(e.id); len(a) != 1 || a[0].Snapshot != list[0].ID || a[0].Title != "Updated Fabric 0.16.10" {
		t.Fatalf("%+v", a)
	}
	if r := row(e.diff(list[0].ID), "Minecraft and loader"); !r.Default || r.To != "1.21.1, Fabric 0.16.9" {
		t.Fatalf("%+v", r)
	}
}
