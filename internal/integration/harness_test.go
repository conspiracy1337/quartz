//go:build integration

// Package integration runs the real services against the real internet:
// go test -tags integration ./internal/integration
package integration

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"quartz/internal/core/download"
	"quartz/internal/core/httpcache"
	"quartz/internal/core/netx"
	"quartz/internal/core/tasks"
	"quartz/internal/java"
	"quartz/internal/meta"
)

type harness struct {
	t      *testing.T
	root   string
	http   *netx.Client
	cache  *httpcache.Cache
	meta   *meta.Service
	dl     *download.Downloader
	tasks  *tasks.Engine
	java   *java.Manager
	vendor string
	log    *slog.Logger
}

// newHarness wires the services over root (QUARTZ_IT_ROOT keeps files
// between runs, so a second run can prove it needs no network).
func newHarness(t *testing.T) *harness {
	t.Helper()
	root := os.Getenv("QUARTZ_IT_ROOT")
	if root == "" {
		root = t.TempDir()
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	h := &harness{t: t, root: root, vendor: "temurin", log: log}
	var err error
	if h.http, err = netx.New(netx.Proxy{Mode: netx.ProxySystem}, netx.Options{}); err != nil {
		t.Fatal(err)
	}
	if h.cache, err = httpcache.Open(filepath.Join(root, "cache"), h.http, log); err != nil {
		t.Fatal(err)
	}
	h.meta = meta.New(h.cache)
	ver, err := download.OpenVerified(filepath.Join(root, "verified.json"), root, log)
	if err != nil {
		t.Fatal(err)
	}
	h.dl = download.New(h.http, ver)
	if h.tasks, err = tasks.New(tasks.Config{Downloader: h.dl, Root: root, HistoryPath: filepath.Join(root, "downloads.json"), Parallel: 8, Log: log}); err != nil {
		t.Fatal(err)
	}
	h.java, err = java.New(java.Config{
		Dir: filepath.Join(root, "store", "java"), StatePath: filepath.Join(root, "java.json"), BinDir: filepath.Join(root, "store", "bin"),
		Meta: h.meta, Cache: h.cache, Vendor: func() string { return h.vendor }, Log: log,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = h.tasks.Close()
		_ = ver.Flush()
		_ = h.cache.Flush()
		_ = h.java.Flush()
	})
	return h
}

func (h *harness) ctx() context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	h.t.Cleanup(cancel)
	return ctx
}

// run submits a job and waits for it.
func (h *harness) run(spec *tasks.Spec) {
	h.t.Helper()
	id, err := h.tasks.Submit(spec)
	if err != nil {
		h.t.Fatal(err)
	}
	if err := h.tasks.Wait(h.ctx(), id); err != nil {
		h.t.Fatal(err)
	}
}
