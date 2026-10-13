package gamelog

import (
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

type noRedact struct{}

func (noRedact) Redact(s string) string { return s }

func TestConsole(t *testing.T) {
	var mu sync.Mutex
	var batches [][]Line
	var seen int
	c := NewConsole(2, noRedact{}, time.Millisecond, func(Line) { seen++ }, func(l []Line) {
		mu.Lock()
		batches = append(batches, l)
		mu.Unlock()
	})
	outR, outW := io.Pipe()
	wait := c.Start(outR, strings.NewReader("oops\n"))
	select {
	case <-c.First():
	case <-time.After(time.Second):
		t.Fatal("First not closed after output")
	}
	_, _ = io.WriteString(outW, "[21:40:02] [Server thread/INFO]: Done (4.2s)!\n[21:40:03] [Server thread/INFO]: a\n")
	_ = outW.Close()
	wait()
	var all []Line
	for _, b := range batches {
		all = append(all, b...)
	}
	if len(all) != 3 || seen != 3 {
		t.Fatalf("delivered %d lines (hook saw %d): %+v", len(all), seen, all)
	}
	if kept := c.Lines(); len(kept) != 2 {
		t.Fatalf("the ring keeps the newest 2: %+v", kept)
	}
}
