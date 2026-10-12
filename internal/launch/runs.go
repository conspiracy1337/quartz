package launch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"quartz/internal/core/fsx"
)

// Result says how a run ended (the UI's RunResult).
type Result string

const (
	ResultOK     Result = "ok"
	ResultCrash  Result = "crash"
	ResultKilled Result = "killed"
	ResultFailed Result = "failed"
)

// MemoryRange is in MB.
type MemoryRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// Record is one launch in an instance's history; its log sits next to it.
type Record struct {
	ID        string      `json:"id"`
	StartedAt int64       `json:"startedAt"` // unix ms
	LengthMin int         `json:"lengthMin"`
	ExitCode  int         `json:"exitCode"`
	Result    Result      `json:"result"`
	Java      string      `json:"java"`
	Memory    MemoryRange `json:"memory"`
	Account   string      `json:"account"`
}

const maxRecords = 200

func newRunID(t time.Time) string { return t.UTC().Format("20060102-150405.000") }

func saveRecord(dir string, r Record) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return fsx.AtomicWrite(filepath.Join(dir, r.ID+".json"), data, fsx.FilePerm)
}

// listRecords returns an instance's runs, newest first.
func listRecords(dir string) []Record {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []Record{}
	}
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	slices.Sort(names)
	slices.Reverse(names)
	out := make([]Record, 0, min(len(names), maxRecords))
	for _, n := range names[:min(len(names), maxRecords)] {
		data, err := fsx.ReadCapped(filepath.Join(dir, n), 64<<10)
		if err != nil {
			continue
		}
		var r Record
		if json.Unmarshal(data, &r) == nil && r.ID+".json" == n {
			out = append(out, r)
		}
	}
	return out
}
