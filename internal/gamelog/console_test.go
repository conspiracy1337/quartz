package gamelog

import (
	"io"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
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

// A line longer than any buffer is cut, and the lines after it still arrive:
// the reader never stops draining the pipe.
func TestEachLineHugeLine(t *testing.T) {
	huge := strings.Repeat("x", 2<<20)
	var got []string
	eachLine(strings.NewReader("first\r\n"+huge+"\nafter\nlast"), func(l string) { got = append(got, l) })
	if len(got) != 4 || got[0] != "first" || got[2] != "after" || got[3] != "last" {
		t.Fatalf("lines: %d, %q %q", len(got), got[0], got[len(got)-1])
	}
	if len(got[1]) != maxLine+len(lineCut) || !strings.HasSuffix(got[1], lineCut) {
		t.Fatalf("the huge line is cut at %d bytes: %d", maxLine, len(got[1]))
	}
}

// A cut never splits a multi-byte character.
func TestEachLineCutKeepsUTF8(t *testing.T) {
	var got []string
	eachLine(strings.NewReader(strings.Repeat("x", maxLine-1)+"ü\n"), func(l string) { got = append(got, l) })
	if len(got) != 1 || !utf8.ValidString(got[0]) {
		t.Fatalf("cut line is valid UTF-8: %v", len(got))
	}
}
