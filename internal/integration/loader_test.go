//go:build integration

package integration

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"quartz/internal/core/sysinfo"
	"quartz/internal/core/tasks"
	"quartz/internal/install"
	"quartz/internal/java"
	"quartz/internal/loader"
	"quartz/internal/mojang"
)

// TestInstallLoaders installs the latest stable build of every loader on the
// Minecraft versions people play most, Forge back to 1.7.10, then proves a
// second pass plans nothing and runs no installer step.
func TestInstallLoaders(t *testing.T) {
	cases := []struct {
		kind loader.Kind
		mc   string
	}{
		{loader.Fabric, "1.21.1"}, {loader.Quilt, "1.21.1"},
		{loader.Forge, "1.20.1"}, {loader.Forge, "1.16.5"}, {loader.Forge, "1.12.2"}, {loader.Forge, "1.7.10"},
		{loader.NeoForge, "1.21.1"}, {loader.NeoForge, "1.20.1"},
	}
	h := newHarness(t)
	in, reg := h.installer(), h.loaders()
	env := mojang.CurrentEnv(sysinfo.OSVersion())
	for _, c := range cases {
		t.Run(string(c.kind)+"-"+c.mc, func(t *testing.T) {
			v, err := reg.Latest(h.ctx(), c.kind, c.mc)
			if err != nil {
				t.Fatal(err)
			}
			target := install.Target{Minecraft: c.mc, Loader: c.kind, LoaderVersion: v.ID}
			gameDir := filepath.Join(h.root, "instances", string(c.kind)+c.mc, "game")
			start := time.Now()
			res, err := in.Resolve(h.ctx(), target, gameDir, env)
			if err != nil {
				t.Fatal(err)
			}
			own := java.Request{Major: res.Version.JavaMajor(), Component: res.Version.JavaComponent()}
			_, plan, err := h.java.Resolve(h.ctx(), own)
			if err != nil {
				t.Fatal(err)
			}
			procJava := func(ctx context.Context) (string, error) {
				rt, _, err := h.java.Resolve(ctx, own)
				return rt.Path, err
			}
			spec, err := in.Plan(install.Request{Resolved: res, Java: plan, ProcessorJava: procJava, Meta: tasks.Meta{Title: string(c.kind)}})
			if err != nil {
				t.Fatal(err)
			}
			if spec != nil {
				h.runIn(t, spec)
			}
			t.Logf("%s %s installed in %s (main class %s)", loader.Label(c.kind, c.mc, v.ID), c.mc, time.Since(start).Round(time.Millisecond), res.Version.MainClass)

			start = time.Now()
			res, err = in.Resolve(h.ctx(), target, gameDir, env)
			if err != nil {
				t.Fatal(err)
			}
			again, err := in.Plan(install.Request{Resolved: res, ProcessorJava: procJava, Meta: tasks.Meta{Title: string(c.kind)}})
			if err != nil || again != nil {
				t.Fatalf("second plan not empty: %v", err)
			}
			t.Logf("installed check took %s", time.Since(start).Round(time.Microsecond))
		})
	}
}
