package safe

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestRunRecovers(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	if Run(log, "boom", func() { panic("kaputt") }) {
		t.Fatal("a panic reported success")
	}
	if !strings.Contains(buf.String(), "kaputt") || !strings.Contains(buf.String(), "work=boom") {
		t.Fatalf("panic not logged: %s", buf.String())
	}
	if !Run(log, "fine", func() {}) {
		t.Fatal("a normal return reported failure")
	}
}
