package launch

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"quartz/internal/core/fsx"
	"quartz/internal/gamelog"
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

// Record is one launch in an instance's history. Its evidence sits next to
// it: <id>.log (the console), <id>.cmd.txt (the command line, redacted) and,
// after a crash, <id>.crash.txt (the game's crash report, redacted).
type Record struct {
	ID        string      `json:"id"`
	StartedAt int64       `json:"startedAt"` // unix ms
	LengthMin int         `json:"lengthMin"`
	ExitCode  int         `json:"exitCode"`
	Result    Result      `json:"result"`
	Java      string      `json:"java"`
	Memory    MemoryRange `json:"memory"`
	Account   string      `json:"account"`           // the player name, as it was at launch
	AccountID string      `json:"accountId"`         // the account it was
	Mods      int         `json:"mods"`              // enabled mods loaded
	Sandbox   string      `json:"sandbox,omitempty"` // the sandbox it ran in ("bubblewrap"); "" is none
	// Filled in after the game exits (see Finisher).
	World  string `json:"world,omitempty"`  // the world played, by its name
	Server string `json:"server,omitempty"` // the server joined last, host[:port]
	Shots  int    `json:"shots,omitempty"`  // screenshots taken
	Crash  *Crash `json:"crash,omitempty"`  // the likely cause of a crash
}

// Crash summarises a crash for the history and the activity feed; the full
// analysis is made again from the evidence when the crash dialog opens.
type Crash struct {
	Title  string `json:"title"`            // "Entity Culling 1.7.2 failed to load"
	Detail string `json:"detail,omitempty"` // "Mixin apply failed in entityculling"
	Report bool   `json:"report"`           // a crash report was saved with the run
}

// Finished is what a Finisher sees once a game has exited.
type Finished struct {
	InstanceID string
	GameDir    string
	RunsDir    string
	Started    time.Time
	Ended      time.Time
	Record     *Record
	// Stopped is set when the player stopped the run: whatever it left
	// behind, it didn't crash.
	Stopped bool
	// Tail is the end of the console, newest last.
	Tail []gamelog.Line
}

// Finisher adds to a run's record after the game exits and before it is
// saved: the world played, screenshots taken, the cause of a crash.
// Finishers run in order on the exit path, so they must be quick.
type Finisher interface {
	FinishRun(f *Finished)
}

// Evidence files saved with a run.
const (
	EvidenceLog     = "log"
	EvidenceCommand = "cmd"
	EvidenceCrash   = "crash"
)

var evidenceExt = map[string]string{EvidenceLog: ".log", EvidenceCommand: ".cmd.txt", EvidenceCrash: ".crash.txt"}

// EvidencePath is where a run's evidence file of a kind lives.
func EvidencePath(runsDir, runID, kind string) (string, error) {
	ext, ok := evidenceExt[kind]
	if !ok || !runIDRe.MatchString(runID) {
		return "", ErrNoRun
	}
	return filepath.Join(runsDir, runID+ext), nil
}

// ErrNoRun is returned for a run or evidence file that doesn't exist.
var ErrNoRun = errors.New("launch: no such run")

var runIDRe = regexp.MustCompile(`^[0-9]{8}-[0-9]{6}\.[0-9]{3}$`)

// ValidRunID reports whether s is a run id.
func ValidRunID(s string) bool { return runIDRe.MatchString(s) }

const (
	maxRecords  = 200
	maxEvidence = 50 // runs that keep their log, command and crash report
)

func newRunID(t time.Time) string { return t.UTC().Format("20060102-150405.000") }

func saveRecord(dir string, r Record) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if err := fsx.AtomicWrite(filepath.Join(dir, r.ID+".json"), data, fsx.FilePerm); err != nil {
		return err
	}
	pruneRuns(dir)
	return nil
}

// pruneRuns keeps the newest maxRecords records, and evidence for the newest
// maxEvidence of them.
func pruneRuns(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var ids []string
	for _, e := range entries {
		if id, ok := strings.CutSuffix(e.Name(), ".json"); ok && runIDRe.MatchString(id) {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	slices.Reverse(ids)
	for i, id := range ids {
		if i < maxEvidence {
			continue
		}
		for _, ext := range evidenceExt {
			_ = os.Remove(filepath.Join(dir, id+ext))
		}
		if i >= maxRecords {
			_ = os.Remove(filepath.Join(dir, id+".json"))
		}
	}
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
