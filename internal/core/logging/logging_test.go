package logging

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRedact(t *testing.T) {
	r := NewRedactor("alice")
	jwt := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"
	tests := []struct{ in, mustNot, must string }{
		{"token " + jwt, jwt, Redacted},
		{"java --accessToken abc123def --version 1.21", "abc123def", "--accessToken " + Redacted},
		{`{"access_token": "secretvalue"}`, "secretvalue", Redacted},
		{"Authorization: Bearer secretvalue", "secretvalue", Redacted},
		{"https://login.live.com/oauth?code=M.C123&state=x", "M.C123", "code=" + Redacted},
		{"mail me at alice@example.com", "alice@example.com", Redacted},
		{"/home/alice/.local/share/Quartz", "/home/alice", "/home/<user>"},
		{`C:\Users\alice\AppData`, `\Users\alice`, `\Users\<user>`},
		{`"path": "C:\\Users\\alice\\AppData"`, `\\Users\\alice`, `\\Users\\<user>`},
	}
	for _, tt := range tests {
		got := r.Redact(tt.in)
		if strings.Contains(got, tt.mustNot) || !strings.Contains(got, tt.must) {
			t.Errorf("Redact(%q) = %q", tt.in, got)
		}
	}
	if got := r.Redact("exit code=1 on 1.21.4"); got != "exit code=1 on 1.21.4" {
		t.Errorf("changed harmless text: %q", got)
	}
}

func TestLoggerRedactsAttrs(t *testing.T) {
	var buf bytes.Buffer
	l := slog.New(&redactHandler{next: slog.NewJSONHandler(&buf, nil), r: NewRedactor("bob")})
	l.With("path", "/home/bob/x").Info("redirect /cb?code=XYZ", "err", errors.New("for bob@mail.com"), slog.Group("g", "tok", "--accessToken T0K"))
	out := buf.String()
	for _, leak := range []string{"XYZ", "bob@mail.com", "T0K", "/home/bob"} {
		if strings.Contains(out, leak) {
			t.Errorf("log leaked %q: %s", leak, out)
		}
	}
}

func TestRotation(t *testing.T) {
	dir := t.TempDir()
	w, err := openRotating(filepath.Join(dir, fileName))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Close() }()
	chunk := bytes.Repeat([]byte("x"), maxSize/2+1)
	for range 3 {
		if _, err := w.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, fileName+".1")); err != nil {
		t.Fatalf("no rotated file: %v", err)
	}
}

func TestAddSecret(t *testing.T) {
	r := NewRedactor("")
	r.AddSecret("0")
	r.AddSecret("hunter2-proxy")
	got := r.Redact("pw=hunter2-proxy count=0")
	if got != "pw=[redacted] count=0" {
		t.Fatalf("got %q", got)
	}
	r.RemoveSecret("hunter2-proxy")
	if r.Redact("hunter2-proxy") != "hunter2-proxy" {
		t.Fatal("secret not removed")
	}
}
