package modpack

import (
	"bytes"
	"context"
	"crypto/sha1" //nolint:gosec // test fixtures
	"crypto/sha512"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"quartz/internal/content/store"
	"quartz/internal/core/download"
	"quartz/internal/core/tasks"
	"quartz/internal/instance"
	"quartz/internal/manual"
)

type cdn map[string][]byte

func (c cdn) Do(req *http.Request) (*http.Response, error) {
	body, ok := c[req.URL.String()]
	if !ok {
		return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body)), Header: http.Header{}, Request: req}, nil
}

type installEnv struct {
	svc   *Service
	insts *instance.Store
	eng   *tasks.Engine
	root  string
}

func newInstallEnv(t *testing.T, files cdn) *installEnv {
	t.Helper()
	root := t.TempDir()
	log := slog.New(slog.DiscardHandler)
	insts, err := instance.Open(filepath.Join(root, "instances"), log)
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(root, "store", "content"), nil, log)
	if err != nil {
		t.Fatal(err)
	}
	ver, err := download.OpenVerified(filepath.Join(root, "verified.json"), root, log)
	if err != nil {
		t.Fatal(err)
	}
	eng, err := tasks.New(tasks.Config{Downloader: download.New(files, ver), Root: root, HistoryPath: filepath.Join(root, "downloads.json"), Parallel: 4})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	svc := NewService(InstallDeps{Reader: NewReader(Deps{}), Instances: insts, Store: st, Tasks: eng,
		Manual: manual.New(manual.Deps{Store: st, Tasks: eng}), Staging: filepath.Join(root, "instances", ".staging"), Log: log}, UpdateDeps{})
	return &installEnv{svc: svc, insts: insts, eng: eng, root: root}
}

func hashes(b []byte) (string, string) {
	s1 := sha1.Sum(b) //nolint:gosec // fixtures
	s5 := sha512.Sum512(b)
	return hex.EncodeToString(s1[:]), hex.EncodeToString(s5[:])
}

func TestInstallCreatesTheInstanceLast(t *testing.T) {
	jar := []byte("a mod jar")
	s1, s5 := hashes(jar)
	url := "https://cdn.modrinth.com/data/x/mod.jar"
	env := newInstallEnv(t, cdn{url: jar})
	index := js(map[string]any{"formatVersion": 1, "game": "minecraft", "versionId": "2.0", "name": "Vanilla Plus",
		"dependencies": map[string]string{"minecraft": "1.21.1"},
		"files":        []any{map[string]any{"path": "mods/mod.jar", "hashes": map[string]string{"sha1": s1, "sha512": s5}, "downloads": []string{url}, "fileSize": len(jar)}}})
	file := zipOf(t, entry{name: "modrinth.index.json", body: index}, entry{name: "overrides/config/a.toml", body: "a=1"})
	p, err := env.svc.d.Reader.File(context.Background(), file)
	if err != nil {
		t.Fatal(err)
	}
	p.Origin = Origin{Source: "modrinth", Project: "P1", Version: "V2"}
	tok := env.svc.Stage(p)
	job, err := env.svc.Install(context.Background(), tok, "My Pack", Look{Art: instance.Art{Background: "#3ecf8e", Initial: "M"}, PackIcon: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.svc.Install(context.Background(), tok, "again", Look{Art: instance.Art{}, PackIcon: true}, nil); err == nil {
		t.Fatal("a token was used twice")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := env.eng.Wait(ctx, job); err != nil {
		t.Fatal(err)
	}
	list := env.insts.List()
	if len(list) != 1 || list[0].Name != "My Pack" || list[0].Pack == nil || list[0].Pack.VersionName != "2.0" || list[0].Minecraft != "1.21.1" {
		t.Fatalf("instances %+v", list)
	}
	game := env.insts.GameDir(list[0].ID)
	for _, rel := range []string{"mods/mod.jar", "config/a.toml"} {
		if _, err := os.Stat(filepath.Join(game, rel)); err != nil {
			t.Errorf("%s missing", rel)
		}
	}
	m, err := ReadManifest(env.insts.Dir(list[0].ID))
	if err != nil || m.Files["mods/mod.jar"].SHA1 != s1 || !m.Files["config/a.toml"].Layer || m.Origin.Project != "P1" {
		t.Fatalf("manifest %+v %v", m, err)
	}
	time.Sleep(50 * time.Millisecond)
	if left, _ := os.ReadDir(filepath.Join(env.root, "instances", ".staging")); len(left) != 0 {
		t.Fatalf("staging left behind: %v", left)
	}
}

func TestCancelledInstallLeavesNothing(t *testing.T) {
	env := newInstallEnv(t, cdn{})
	p := &Pack{Name: "Blocked", Minecraft: "1.21.1", Files: []File{{Path: "mods/b.jar", SHA1: strings.Repeat("b", 40), Size: 4, Page: "https://www.curseforge.com/x"}}}
	job, err := env.svc.Install(context.Background(), env.svc.Stage(p), "", Look{Art: instance.Art{Background: "#3ecf8e"}, PackIcon: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if err := env.eng.Cancel(job); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = env.eng.Wait(ctx, job)
	time.Sleep(50 * time.Millisecond)
	if len(env.insts.List()) != 0 {
		t.Fatal("a cancelled install created an instance")
	}
	if left, _ := os.ReadDir(filepath.Join(env.root, "instances", ".staging")); len(left) != 0 {
		t.Fatalf("staging left behind: %v", left)
	}
}
