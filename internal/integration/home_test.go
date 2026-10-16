//go:build integration

package integration

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"quartz/internal/core/remoteimg"
	"quartz/internal/core/slp"
	"quartz/internal/home"
	"quartz/internal/instance"
)

// TestPingPublicServer asks a large public server for its status.
func TestPingPublicServer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for _, addr := range []string{"mc.hypixel.net", "play.cubecraft.net"} {
		a, _ := slp.Parse(addr)
		st, err := slp.Ping(ctx, a, 769)
		if err != nil {
			t.Logf("%s: %v", addr, err)
			continue
		}
		t.Logf("%s: %q protocol %d, %d/%d players, %v, favicon %d bytes, motd %q", addr, st.Version, st.Protocol, st.Online, st.Max, st.Latency, len(st.Favicon), st.MOTD)
		if st.Online <= 0 || st.Version == "" || st.Latency <= 0 {
			t.Fatalf("%s: implausible status %+v", addr, st)
		}
		return
	}
	t.Fatal("no public server answered")
}

// TestNewsFeed reads the official launcher's feeds and picks a Java article or patch note.
func TestNewsFeed(t *testing.T) {
	h := newHarness(t)
	insts, err := instance.Open(filepath.Join(t.TempDir(), "instances"), nil)
	if err != nil {
		t.Fatal(err)
	}
	pics := remoteimg.New(filepath.Join(t.TempDir(), "pics"), h.http, nil)
	svc := home.New(home.Deps{Instances: insts, Meta: h.meta, Pictures: pics, DataDir: t.TempDir()})
	n, err := svc.News(h.ctx())
	if err != nil || n == nil {
		t.Fatalf("%+v %v", n, err)
	}
	t.Logf("%s (%s) %s image=%q", n.Title, time.UnixMilli(n.Published).Format(time.DateOnly), n.URL, n.Image)
	if !strings.HasPrefix(n.URL, "https://www.minecraft.net/") || n.Image == "" {
		t.Fatalf("%+v", n)
	}
	token := strings.TrimSuffix(strings.TrimPrefix(n.Image, "/tex/remote/"), ".png")
	p, err := pics.Path(h.ctx(), token)
	if err != nil {
		t.Fatalf("picture not fetched: %v", err)
	}
	t.Logf("picture cached at %s", p)
}
