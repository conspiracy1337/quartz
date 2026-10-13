//go:build !windows

package discord

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"path/filepath"
	"testing"
)

func writeFrame(t *testing.T, w io.Writer, op uint32, v any) {
	t.Helper()
	b, _ := json.Marshal(v)
	head := make([]byte, 8)
	binary.LittleEndian.PutUint32(head, op)
	binary.LittleEndian.PutUint32(head[4:], uint32(len(b)))
	if _, err := w.Write(append(head, b...)); err != nil {
		t.Error(err)
	}
}

func readFrame(t *testing.T, r io.Reader) (uint32, map[string]any) {
	t.Helper()
	head := make([]byte, 8)
	if _, err := io.ReadFull(r, head); err != nil {
		t.Error(err)
		return 0, nil
	}
	b := make([]byte, binary.LittleEndian.Uint32(head[4:]))
	_, _ = io.ReadFull(r, b)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return binary.LittleEndian.Uint32(head), m
}

func TestClientAgainstAFakeDiscord(t *testing.T) {
	dir := t.TempDir()
	for _, e := range []string{"XDG_RUNTIME_DIR", "TMPDIR", "TMP", "TEMP"} {
		t.Setenv(e, dir)
	}
	ln, err := net.Listen("unix", filepath.Join(dir, "discord-ipc-0"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	got := make(chan map[string]any, 4)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = c.Close() }()
		op, hs := readFrame(t, c)
		if op != opHandshake || hs["client_id"] != "123" || hs["v"] != float64(1) {
			t.Errorf("handshake %d %v", op, hs)
		}
		writeFrame(t, c, opPing, map[string]any{"x": 1})
		if op, _ := readFrame(t, c); op != opPong {
			t.Errorf("want a pong, got op %d", op)
		}
		writeFrame(t, c, opFrame, map[string]any{"cmd": "DISPATCH", "evt": "READY"})
		for range 2 {
			_, req := readFrame(t, c)
			got <- req
			writeFrame(t, c, opFrame, map[string]any{"cmd": "SET_ACTIVITY", "nonce": req["nonce"]})
		}
	}()
	cl, err := Connect("123")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cl.Close() }()
	if err := cl.SetActivity(42, &Activity{Details: "My Pack", State: "Minecraft 1.21.4"}); err != nil {
		t.Fatal(err)
	}
	req := <-got
	args := req["args"].(map[string]any)
	if req["cmd"] != "SET_ACTIVITY" || args["pid"] != float64(42) || args["activity"].(map[string]any)["details"] != "My Pack" {
		t.Fatalf("%v", req)
	}
	if err := cl.SetActivity(42, nil); err != nil {
		t.Fatal(err)
	}
	if _, has := (<-got)["args"].(map[string]any)["activity"]; has {
		t.Fatal("clearing sends no activity")
	}
}

func TestConnectWithoutDiscord(t *testing.T) {
	dir := t.TempDir()
	for _, e := range []string{"XDG_RUNTIME_DIR", "TMPDIR", "TMP", "TEMP"} {
		t.Setenv(e, dir)
	}
	if _, err := Connect("123"); err == nil {
		t.Fatal("no socket, no connection")
	}
}
