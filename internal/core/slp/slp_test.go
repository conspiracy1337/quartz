package slp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

// fakeServer answers status requests with js, replies to pings unless noPong,
// and can send a raw first packet instead.
func fakeServer(t *testing.T, js string, noPong bool, raw []byte) (Address, *atomic.Int32) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	var conns atomic.Int32
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			conns.Add(1)
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				r := bufio.NewReader(c)
				hs, err := readPacket(r)
				if err != nil || hs[0] != 0x00 {
					return
				}
				if _, err := readPacket(r); err != nil {
					return
				}
				if raw != nil {
					_, _ = c.Write(raw)
					return
				}
				var b bytes.Buffer
				putVarInt(&b, 0)
				putString(&b, js)
				_ = writePacket(c, b.Bytes())
				p, err := readPacket(r)
				if err != nil || noPong {
					return
				}
				_ = writePacket(c, p)
			}(c)
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	return Address{Host: "127.0.0.1", Port: port}, &conns
}

func pngData(t *testing.T, side int) string {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, side, side))); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}

func status(t *testing.T, fav string) string {
	b, _ := json.Marshal(map[string]any{
		"version":     map[string]any{"name": "§aPaper 1.21.4", "protocol": 769},
		"players":     map[string]any{"max": 20, "online": 6},
		"description": map[string]any{"text": "Friends ", "extra": []any{map[string]any{"text": "§lSMP"}}},
		"favicon":     fav,
	})
	return string(b)
}

func TestPing(t *testing.T) {
	addr, _ := fakeServer(t, status(t, pngData(t, 64)), false, nil)
	st, err := Ping(context.Background(), addr, -1)
	if err != nil {
		t.Fatal(err)
	}
	if st.Version != "Paper 1.21.4" || st.Protocol != 769 || st.Online != 6 || st.Max != 20 || st.MOTD != "Friends SMP" {
		t.Fatalf("status %+v", st)
	}
	if len(st.Favicon) == 0 || st.Latency <= 0 {
		t.Fatal("favicon or latency missing")
	}
}

func TestNoPongStillAnswers(t *testing.T) {
	addr, _ := fakeServer(t, status(t, ""), true, nil)
	st, err := Ping(context.Background(), addr, 769)
	if err != nil || st.Latency <= 0 || st.Favicon != nil {
		t.Fatalf("%+v %v", st, err)
	}
}

func TestHostileServers(t *testing.T) {
	big := pngData(t, 512) // over 64 px: dropped
	addr, _ := fakeServer(t, status(t, big), false, nil)
	if st, err := Ping(context.Background(), addr, -1); err != nil || st.Favicon != nil {
		t.Fatalf("oversized favicon kept: %v", err)
	}
	addr, _ = fakeServer(t, status(t, "data:image/svg+xml;base64,PHN2Zz4="), false, nil)
	if st, _ := Ping(context.Background(), addr, -1); st == nil || st.Favicon != nil {
		t.Fatal("non-PNG favicon kept")
	}
	// A packet length far over the cap.
	var huge bytes.Buffer
	putVarInt(&huge, 1<<28)
	addr, _ = fakeServer(t, "", false, huge.Bytes())
	if _, err := Ping(context.Background(), addr, -1); err == nil {
		t.Fatal("oversized packet accepted")
	}
	addr, _ = fakeServer(t, "{not json", false, nil)
	if _, err := Ping(context.Background(), addr, -1); err == nil {
		t.Fatal("bad json accepted")
	}
}

func TestTimeout(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer func() { _ = ln.Close() }()
	go func() {
		c, err := ln.Accept()
		if err == nil {
			time.Sleep(5 * time.Second)
			_ = c.Close()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := Ping(ctx, Address{Host: "127.0.0.1", Port: ln.Addr().(*net.TCPAddr).Port}, -1); err == nil {
		t.Fatal("silent server answered")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("deadline ignored")
	}
}

func TestPingerCachesAndMerges(t *testing.T) {
	p := NewPinger()
	var calls atomic.Int32
	p.ping = func(context.Context, Address, int) (*Status, error) {
		calls.Add(1)
		time.Sleep(50 * time.Millisecond)
		return &Status{Online: 1}, nil
	}
	a := Address{Host: "Play.Example.net.", Port: 25565}
	done := make(chan Result, 4)
	for range 4 {
		go func() { done <- p.Get(context.Background(), a, -1, false) }()
	}
	for range 4 {
		if r := <-done; r.Status == nil {
			t.Fatal("no status")
		}
	}
	p.Get(context.Background(), Address{Host: "play.example.net"}, -1, false)
	if calls.Load() != 1 {
		t.Fatalf("pinged %d times, want 1", calls.Load())
	}
	p.Get(context.Background(), a, -1, true)
	if calls.Load() != 2 {
		t.Fatal("force did not ping")
	}
	p.ping = func(context.Context, Address, int) (*Status, error) { return nil, errors.New("down") }
	if r := p.Get(context.Background(), Address{Host: "other"}, -1, false); r.Err == nil {
		t.Fatal("error lost")
	}
}

func TestParse(t *testing.T) {
	good := map[string]Address{
		"mc.hypixel.net":      {Host: "mc.hypixel.net"},
		"smp.kestrel.gg:25570": {Host: "smp.kestrel.gg", Port: 25570},
		"[::1]:25565":         {Host: "::1", Port: 25565},
		"::1":                 {Host: "::1"},
		" 10.0.0.2 ":          {Host: "10.0.0.2"},
	}
	for in, want := range good {
		got, err := Parse(in)
		if err != nil || got != want {
			t.Errorf("%q: %+v %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "a b", "host:0", "host:70000", "host:x", "user@host", "http://x", ":25565"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if (Address{Host: "Play.Example.NET.", Port: 25565}).Key() != "play.example.net" {
		t.Error("key not normalised")
	}
}
