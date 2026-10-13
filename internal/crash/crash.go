// Package crash explains why a game crashed. A table of rules (rules.go)
// reads the game's crash report and the end of the run's log for the usual
// causes (a mixin that failed, a missing dependency, a duplicate mod, the
// wrong Java, too little memory, a graphics driver), names the mod through
// the content catalog, and offers the fixes the crash dialog shows: update
// that mod, or disable it, then launch again.
package crash

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"quartz/internal/content"
	"quartz/internal/core/fsx"
	"quartz/internal/core/logging"
	"quartz/internal/gamelog"
	"quartz/internal/instance"
	"quartz/internal/launch"
)

// Limits.
const (
	maxReport  = 2 << 20
	maxLog     = 1 << 20
	traceLines = 40
)

// ErrNoCrash is returned when there is no crash to explain.
var ErrNoCrash = errors.New("crash: nothing to explain")

var reportNameRe = regexp.MustCompile(`^crash-[\w.\-]{1,96}\.txt$`)

// ValidReportName reports whether name is a file in crash-reports/.
func ValidReportName(name string) bool { return reportNameRe.MatchString(name) }

// Info is what the crash dialog shows.
type Info struct {
	InstanceID string `json:"instanceId"`
	RunID      string `json:"runId,omitempty"`
	File       string `json:"file,omitempty"` // the report in crash-reports/, when known
	LengthMin  int    `json:"lengthMin"`
	ExitCode   int    `json:"exitCode"`
	Cause      *Cause `json:"cause,omitempty"`
	Trace      string `json:"trace"`
}

// Cause is the likely reason, in words, with the fixes that apply.
type Cause struct {
	Title  string   `json:"title"`
	Text   string   `json:"text"`
	Detail string   `json:"detail"`
	Mod    *Mod     `json:"mod,omitempty"`
	Fixes  []string `json:"fixes"` // "update", "disable"
}

// Mod is the mod a cause names, as the Mods tab knows it.
type Mod struct {
	Item    string `json:"item"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Icon    string `json:"icon,omitempty"`
	Update  string `json:"update,omitempty"` // a newer version the last check found
	Enabled bool   `json:"enabled"`
}

// Mods finds a mod in an instance's Mods tab (content.Service).
type Mods interface {
	ByModID(ctx context.Context, id, modID, file string) (content.Row, bool)
}

// Runs reads an instance's launch history (launch.Launcher).
type Runs interface {
	Run(id, runID string) (launch.Record, error)
	Evidence(id, runID, kind string) (string, error)
}

// Deps wires a Service.
type Deps struct {
	Instances *instance.Store
	Mods      Mods
	Runs      Runs
	Redactor  *logging.Redactor
}

// Service is safe for concurrent use.
type Service struct{ d Deps }

// New returns a Service.
func New(d Deps) *Service { return &Service{d: d} }

// FinishRun keeps the crash report the run produced (redacted) and, when the
// run crashed, notes its likely cause in the record.
func (s *Service) FinishRun(f *launch.Finished) {
	report := ""
	if p := newestReport(f.GameDir, f.Started); p != "" {
		if data, err := fsx.ReadCapped(p, maxReport); err == nil {
			report = s.redact(string(data))
			if dest, err := launch.EvidencePath(f.RunsDir, f.Record.ID, launch.EvidenceCrash); err == nil {
				if fsx.AtomicWrite(dest, []byte(report), fsx.FilePerm) == nil {
					f.Record.Crash = &launch.Crash{Title: "Minecraft crashed", Report: true}
				}
			}
		}
	}
	if r := f.Record.Result; r != launch.ResultCrash && r != launch.ResultFailed {
		return
	}
	finding, ok := Find(report, joinTail(f.Tail))
	if !ok {
		return
	}
	cause := s.explain(context.Background(), f.InstanceID, finding)
	f.Record.Crash = &launch.Crash{Title: cause.Title, Detail: cause.Detail, Report: report != ""}
}

// ForRun explains a run that crashed, from what was saved with it.
func (s *Service) ForRun(ctx context.Context, id, runID string) (Info, error) {
	rec, err := s.d.Runs.Run(id, runID)
	if err != nil {
		return Info{}, err
	}
	info := Info{InstanceID: id, RunID: runID, LengthMin: rec.LengthMin, ExitCode: rec.ExitCode}
	report := s.evidence(id, runID, launch.EvidenceCrash, maxReport, false)
	log := s.evidence(id, runID, launch.EvidenceLog, maxLog, true)
	s.fill(ctx, &info, report, log)
	return info, nil
}

// ForFile explains a report in the instance's crash-reports folder.
func (s *Service) ForFile(ctx context.Context, id, name string) (Info, error) {
	if !ValidReportName(name) {
		return Info{}, ErrNoCrash
	}
	data, err := fsx.ReadCapped(filepath.Join(s.d.Instances.GameDir(id), "crash-reports", name), maxReport)
	if err != nil {
		return Info{}, ErrNoCrash
	}
	info := Info{InstanceID: id, File: name, ExitCode: -1}
	s.fill(ctx, &info, s.redact(string(data)), "")
	return info, nil
}

func (s *Service) fill(ctx context.Context, info *Info, report, log string) {
	info.Trace = Trace(report, log)
	if f, ok := Find(report, log); ok {
		c := s.explain(ctx, info.InstanceID, f)
		info.Cause = &c
	}
}

// explain turns a finding into words, with the mod as the Mods tab names it.
func (s *Service) explain(ctx context.Context, id string, f Finding) Cause {
	var mod *Mod
	if f.ModID != "" || f.File != "" {
		if r, ok := s.d.Mods.ByModID(ctx, id, f.ModID, f.File); ok {
			mod = &Mod{Item: r.ID, Name: r.Name, Version: r.Version, Icon: r.Icon, Enabled: r.Enabled}
			if r.Status != nil && r.Status.Kind == "update" {
				mod.Update = r.Status.Version
			}
		}
	}
	name := f.ModID
	full := f.ModID
	if mod != nil {
		name = mod.Name
		full = strings.TrimSpace(mod.Name + " " + mod.Version)
	}
	c := Cause{Detail: f.Detail, Mod: mod}
	switch f.Rule {
	case "mixin":
		c.Title = full + " failed to load"
		c.Text = "Its mixin into " + className(f.Target) + " did not apply. This usually means it doesn't support this Minecraft version or another of your mods yet."
	case "mod":
		c.Title = full + " failed to load"
		c.Text = "The game stopped while loading it."
		if f.Text != "" {
			c.Text = f.Text + "."
		}
	case "dependency":
		c.Title = name + " needs " + f.Dep
		c.Text = f.Dep + " isn't installed, or its version doesn't fit. Add it from Browse, or turn " + name + " off."
	case "duplicate":
		c.Title = name + " is installed twice"
		c.Text = "Two files add the same mod"
		if len(f.Files) > 0 {
			c.Text += ": " + strings.Join(f.Files, " and ")
		}
		c.Text += ". Keep one of them."
	case "java":
		c.Title = fmt.Sprintf("This needs Java %d", f.Java)
		c.Text = fmt.Sprintf("Something here was built for Java %d, but the game ran on Java %d. Let the launcher pick Java in instance settings, or choose Java %d there.", f.Java, f.Ran, f.Java)
	case "memory":
		c.Title = "The game ran out of memory"
		c.Text = "Give it more memory in instance settings, or remove some mods."
	case "graphics":
		c.Title = "The graphics driver couldn't start the game"
		c.Text = "Update your graphics driver. On a laptop, make sure the game runs on the dedicated graphics card."
	}
	if mod != nil {
		if mod.Update != "" {
			c.Text += " " + mod.Name + " " + mod.Update + " is out and may fix this."
			c.Fixes = append(c.Fixes, "update")
		}
		if mod.Enabled {
			c.Fixes = append(c.Fixes, "disable")
		}
	}
	if c.Fixes == nil {
		c.Fixes = []string{}
	}
	return c
}

// className names a class for people: "net.minecraft.client.render.entity.EntityRenderer"
// is "EntityRenderer"; obfuscated names ("class_897") are "a game class".
func className(c string) string {
	c = c[strings.LastIndexAny(c, "./")+1:]
	if c == "" || strings.HasPrefix(c, "class_") || len(c) <= 3 {
		return "a game class"
	}
	return c
}

// Trace is the stack trace the dialog shows: the exception, its first frames,
// each "Caused by" with its first frames, and the mod file lines, at most 40.
func Trace(report, log string) string {
	src := report
	if src == "" {
		src = log
	}
	var out []string
	frames := 0
	started := false
	for _, line := range strings.Split(src, "\n") {
		t := strings.TrimRight(line, "\r")
		trim := strings.TrimSpace(t)
		switch {
		case isException(trim):
			started, frames = true, 0
			if strings.HasPrefix(trim, "Caused by:") {
				out = append(out, "  "+trim)
			} else {
				out = append(out, trim)
			}
		case started && strings.HasPrefix(trim, "at "):
			frames++
			if frames <= 3 {
				out = append(out, "    "+trim)
			}
		case strings.HasPrefix(trim, "Mod File:") || strings.HasPrefix(trim, "Mod file:"):
			out = append(out, "  "+trim)
		}
		if len(out) >= traceLines {
			break
		}
	}
	return strings.Join(out, "\n")
}

var exceptionRe = regexp.MustCompile(`^(?:Caused by: )?[\w$.]+(?:Exception|Error|Throwable)\b`)

func isException(line string) bool { return exceptionRe.MatchString(line) }

func (s *Service) redact(text string) string {
	if s.d.Redactor == nil {
		return text
	}
	return s.d.Redactor.Redact(text)
}

// evidence reads one of a run's saved files; tail keeps its end.
func (s *Service) evidence(id, runID, kind string, limit int64, tail bool) string {
	p, err := s.d.Runs.Evidence(id, runID, kind)
	if err != nil {
		return ""
	}
	if !tail {
		data, err := fsx.ReadCapped(p, limit)
		if err != nil {
			return ""
		}
		return string(data)
	}
	f, err := os.Open(p) //nolint:gosec // a run's own log, found through Evidence
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		return ""
	}
	off := max(0, fi.Size()-limit)
	buf := make([]byte, fi.Size()-off)
	if _, err := f.ReadAt(buf, off); err != nil {
		return ""
	}
	return string(buf)
}

// newestReport is the crash report the game wrote since a run started.
func newestReport(gameDir string, since time.Time) string {
	entries, err := os.ReadDir(filepath.Join(gameDir, "crash-reports"))
	if err != nil {
		return ""
	}
	var best string
	var at time.Time
	for _, e := range entries {
		if !ValidReportName(e.Name()) || !e.Type().IsRegular() {
			continue
		}
		fi, err := e.Info()
		if err != nil || fi.ModTime().Before(since.Add(-time.Second)) || !fi.ModTime().After(at) {
			continue
		}
		best, at = filepath.Join(gameDir, "crash-reports", e.Name()), fi.ModTime()
	}
	return best
}

func joinTail(lines []gamelog.Line) string {
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l.Message)
		b.WriteByte('\n')
	}
	return b.String()
}
