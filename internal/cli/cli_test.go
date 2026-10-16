package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"quartz/internal/core/paths"
	"quartz/internal/intent"
)

func fixture(t *testing.T) paths.Paths {
	t.Helper()
	p := paths.Paths{Root: t.TempDir()}
	dir := filepath.Join(p.Instances(), "inst1")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	inst := `{"version":2,"id":"inst1","name":"Survival","minecraft":"1.21.4","loader":"fabric","loaderVersion":"0.16.9","playtimeMin":95,"created":"2026-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(dir, "instance.json"), []byte(inst), 0o600); err != nil {
		t.Fatal(err)
	}
	acc := `{"accounts":[{"id":"0123456789abcdef0123456789abcdef","name":"Steve","kind":"offline"}],"default":"0123456789abcdef0123456789abcdef"}`
	if err := os.WriteFile(p.Accounts(), []byte(acc), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestListJSON(t *testing.T) {
	p := fixture(t)
	var out bytes.Buffer
	if code := Run(intent.Args{Command: intent.List, JSON: true}, p, &out); code != OK {
		t.Fatalf("exit %d: %s", code, out.String())
	}
	var r struct {
		OK   bool       `json:"ok"`
		Data []Instance `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &r); err != nil || !r.OK || len(r.Data) != 1 || r.Data[0].Name != "Survival" || r.Data[0].PlaytimeMin != 95 {
		t.Fatalf("%s %v", out.String(), err)
	}
}

func TestTables(t *testing.T) {
	p := fixture(t)
	var out bytes.Buffer
	Run(intent.Args{Command: intent.List}, p, &out)
	if !strings.Contains(out.String(), "Survival") || !strings.Contains(out.String(), "1.21.4 fabric") || !strings.Contains(out.String(), "1h 35m") {
		t.Fatal(out.String())
	}
	out.Reset()
	Run(intent.Args{Command: intent.Accounts}, p, &out)
	if !strings.Contains(out.String(), "Steve") || !strings.Contains(out.String(), "yes") {
		t.Fatal(out.String())
	}
}

func TestErrors(t *testing.T) {
	var out bytes.Buffer
	_, err := intent.Parse([]string{"--bogus"}, "")
	if code := Error(&out, "", true, err); code != Usage || !strings.Contains(out.String(), `"code":"usage"`) {
		t.Fatalf("%d %s", code, out.String())
	}
}
