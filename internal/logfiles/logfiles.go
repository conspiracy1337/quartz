// Package logfiles reads an instance's game logs and crash reports back for
// the Log files page, cleans up old compressed logs, builds "Copy debug
// info", and uploads a log to mclo.gs. Everything shown or uploaded passes
// through the redactor first (tokens, e-mail addresses, the home folder's
// user name), and uploads also hide every account's player name.
package logfiles

import (
	"bytes"
	"cmp"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"quartz/internal/brand"
	"quartz/internal/content"
	"quartz/internal/core/fsx"
	"quartz/internal/core/logging"
	"quartz/internal/core/netx"
	"quartz/internal/core/sysinfo"
	"quartz/internal/crash"
	"quartz/internal/instance"
	"quartz/internal/launch"
)

// Limits.
const (
	maxText    = 8 << 20 // a file as shown, after decompressing
	maxUpload  = 10 << 20
	maxLines   = 25_000 // mclo.gs keeps at most this many lines
	OldAfter   = 30 * 24 * time.Hour
	uploadURL  = "https://api.mclo.gs/1/log"
	uploadHost = "mclo.gs"
)

// Errors.
var (
	ErrNoFile = errors.New("logfiles: no such log")
	ErrUpload = errors.New("mclo.gs didn't take the log; try again later")
)

var logNameRe = regexp.MustCompile(`^[\w.\-]{1,96}\.log(?:\.gz)?$`)

// File is one entry on the Log files page.
type File struct {
	Name     string `json:"name"`
	Bytes    int64  `json:"bytes"`
	Crash    bool   `json:"crash"`
	Modified int64  `json:"modified"`
}

// Runs is the launch history (launch.Launcher).
type Runs interface {
	Runs(id string) []launch.Record
	Run(id, runID string) (launch.Record, error)
	Evidence(id, runID, kind string) (string, error)
}

// Deps wires a Service.
type Deps struct {
	Instances *instance.Store
	Runs      Runs
	// Mods lists an instance's mods, for debug info.
	Mods     func(ctx context.Context, id string) []content.Row
	HTTP     *netx.Client
	Redactor *logging.Redactor
	// Names are the player names of every account, hidden from uploads.
	Names func() []string
	Now   func() time.Time
	// uploadURL lets tests point at a local server.
	uploadURL string
}

// Service is safe for concurrent use.
type Service struct{ d Deps }

// New returns a Service.
func New(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.uploadURL == "" {
		d.uploadURL = uploadURL
	}
	return &Service{d: d}
}

func (s *Service) dirs(id string) (logs, crashes string) {
	game := s.d.Instances.GameDir(id)
	return filepath.Join(game, "logs"), filepath.Join(game, "crash-reports")
}

// Files lists the logs (latest first, then the dated ones newest first)
// and the crash reports.
func (s *Service) Files(id string) []File {
	logs, crashes := s.dirs(id)
	out := []File{}
	add := func(dir string, ok func(string) bool, isCrash bool) {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if !e.Type().IsRegular() || !ok(e.Name()) {
				continue
			}
			if fi, err := e.Info(); err == nil {
				out = append(out, File{Name: e.Name(), Bytes: fi.Size(), Crash: isCrash, Modified: fi.ModTime().UnixMilli()})
			}
		}
	}
	add(logs, logNameRe.MatchString, false)
	add(crashes, crash.ValidReportName, true)
	rank := func(f File) int {
		switch {
		case f.Name == "latest.log":
			return 0
		case f.Name == "debug.log":
			return 1
		case f.Crash:
			return 3
		}
		return 2
	}
	slices.SortStableFunc(out, func(a, b File) int {
		if ra, rb := rank(a), rank(b); ra != rb {
			return ra - rb
		}
		return cmp.Compare(b.Modified, a.Modified)
	})
	return out
}

// path finds a file by name: crash reports in crash-reports/, logs in logs/.
func (s *Service) path(id, name string) (string, error) {
	logs, crashes := s.dirs(id)
	var p string
	switch {
	case crash.ValidReportName(name):
		p = filepath.Join(crashes, name)
	case logNameRe.MatchString(name):
		p = filepath.Join(logs, name)
	default:
		return "", ErrNoFile
	}
	if fi, err := os.Lstat(p); err != nil || !fi.Mode().IsRegular() {
		return "", ErrNoFile
	}
	return p, nil
}

// Text returns a file's content, decompressed and redacted.
func (s *Service) Text(id, name string) (string, error) {
	p, err := s.path(id, name)
	if err != nil {
		return "", err
	}
	return s.read(p)
}

// RunText returns one of a run's saved files (its log, command or crash report).
func (s *Service) RunText(id, runID, kind string) (string, error) {
	p, err := s.d.Runs.Evidence(id, runID, kind)
	if err != nil {
		return "", ErrNoFile
	}
	return s.read(p)
}

func (s *Service) read(p string) (string, error) {
	if strings.HasSuffix(p, ".gz") {
		data, err := fsx.ReadCapped(p, maxText)
		if err != nil {
			return "", err
		}
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return "", err
		}
		data, err = io.ReadAll(io.LimitReader(zr, maxText))
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
			return "", err
		}
		return s.d.Redactor.Redact(string(data)), nil
	}
	data, err := fsx.ReadTail(p, maxText)
	if err != nil {
		return "", err
	}
	return s.d.Redactor.Redact(string(data)), nil
}

// old lists compressed logs older than 30 days in one instance.
func (s *Service) old(id string) ([]string, int64) {
	logs, _ := s.dirs(id)
	entries, _ := os.ReadDir(logs)
	cutoff := s.d.Now().Add(-OldAfter)
	var paths []string
	var n int64
	for _, e := range entries {
		if !e.Type().IsRegular() || !strings.HasSuffix(e.Name(), ".log.gz") || !logNameRe.MatchString(e.Name()) {
			continue
		}
		if fi, err := e.Info(); err == nil && fi.ModTime().Before(cutoff) {
			paths = append(paths, filepath.Join(logs, e.Name()))
			n += fi.Size()
		}
	}
	return paths, n
}

// OldSize is how much "Delete old logs" frees; id "" counts every instance.
func (s *Service) OldSize(id string) int64 {
	var n int64
	for _, x := range s.ids(id) {
		_, b := s.old(x)
		n += b
	}
	return n
}

// DeleteOld deletes compressed logs older than 30 days; id "" does every instance.
func (s *Service) DeleteOld(id string) int64 {
	var n int64
	for _, x := range s.ids(id) {
		paths, _ := s.old(x)
		for _, p := range paths {
			if fi, err := os.Lstat(p); err == nil && os.Remove(p) == nil {
				n += fi.Size()
			}
		}
	}
	return n
}

func (s *Service) ids(id string) []string {
	if id != "" {
		return []string{id}
	}
	var out []string
	for _, in := range s.d.Instances.List() {
		out = append(out, in.ID)
	}
	return out
}

// ---- mclo.gs ----

// Upload sends text to mclo.gs and returns the page it is on. The text is
// redacted again, player names included, and cut to mclo.gs's limits.
func (s *Service) Upload(ctx context.Context, text string) (string, error) {
	text = s.forUpload(text)
	var out struct {
		Success bool   `json:"success"`
		URL     string `json:"url"`
		Error   string `json:"error"`
	}
	if err := s.d.HTTP.PostForm(ctx, s.d.uploadURL, url.Values{"content": {text}}, &out); err != nil {
		return "", fmt.Errorf("%w (%w)", ErrUpload, err)
	}
	u, err := url.Parse(out.URL)
	if !out.Success || err != nil || u.Scheme != "https" || (u.Host != uploadHost && s.d.uploadURL == uploadURL) {
		return "", ErrUpload
	}
	return out.URL, nil
}

// UploadFile uploads one of the Log files page's files.
func (s *Service) UploadFile(ctx context.Context, id, name string) (string, error) {
	text, err := s.Text(id, name)
	if err != nil {
		return "", err
	}
	return s.Upload(ctx, text)
}

// UploadRun uploads what a run saved: its crash report when it has one,
// else its log. runID "" picks the instance's latest run.
func (s *Service) UploadRun(ctx context.Context, id, runID string) (string, error) {
	if runID == "" {
		runs := s.d.Runs.Runs(id)
		if len(runs) == 0 {
			return "", ErrNoFile
		}
		runID = runs[0].ID
	}
	text, err := s.RunText(id, runID, launch.EvidenceCrash)
	if err != nil {
		text, err = s.RunText(id, runID, launch.EvidenceLog)
	}
	if err != nil {
		return "", err
	}
	return s.Upload(ctx, text)
}

func (s *Service) forUpload(text string) string {
	text = s.d.Redactor.Redact(text)
	if s.d.Names != nil {
		for _, n := range s.d.Names() {
			if len(n) < 3 {
				continue
			}
			re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(n) + `\b`)
			text = re.ReplaceAllString(text, "<player>")
		}
	}
	if lines := strings.Split(text, "\n"); len(lines) > maxLines {
		text = strings.Join(lines[len(lines)-maxLines:], "\n")
	}
	if len(text) > maxUpload {
		text = text[len(text)-maxUpload:]
	}
	return text
}

// ---- debug info ----

// DebugInfo is a short, redacted summary for bug reports: the launcher, the
// system, the instance, the run (runID "" is the latest) and the mods.
func (s *Service) DebugInfo(ctx context.Context, id, runID string) (string, error) {
	in, err := s.d.Instances.Get(id)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", brand.Name, brand.Version)
	fmt.Fprintf(&b, "System: %s %s %s\n", runtime.GOOS, runtime.GOARCH, sysinfo.OSVersion())
	if mem, err := sysinfo.ReadMemory(); err == nil && mem.Total > 0 {
		fmt.Fprintf(&b, "Memory: %.1f GB\n", float64(mem.Total)/(1<<30))
	}
	loader := "Vanilla"
	if in.Loader != "" {
		loader = in.Loader + " " + in.LoaderVersion
	}
	fmt.Fprintf(&b, "Instance: Minecraft %s, %s\n", in.Minecraft, loader)
	var rec *launch.Record
	if runID != "" {
		if r, err := s.d.Runs.Run(id, runID); err == nil {
			rec = &r
		}
	} else if runs := s.d.Runs.Runs(id); len(runs) > 0 {
		rec = &runs[0]
	}
	if rec != nil {
		fmt.Fprintf(&b, "Run: %s, %d min, exit code %d, %s\n", time.UnixMilli(rec.StartedAt).UTC().Format(time.RFC3339), rec.LengthMin, rec.ExitCode, rec.Result)
		fmt.Fprintf(&b, "Java: %s, memory %d to %d MB\n", rec.Java, rec.Memory.Min, rec.Memory.Max)
		if rec.Crash != nil && rec.Crash.Title != "" {
			fmt.Fprintf(&b, "Crash: %s\n", rec.Crash.Title)
		}
	}
	if s.d.Mods != nil {
		mods := s.d.Mods(ctx, id)
		on := 0
		var lines []string
		for _, m := range mods {
			state := ""
			if !m.Enabled {
				state = " (off)"
			} else {
				on++
			}
			lines = append(lines, "  "+m.Name+" "+m.Version+state)
		}
		fmt.Fprintf(&b, "Mods: %d, %d on\n%s\n", len(mods), on, strings.Join(lines, "\n"))
	}
	return s.forUpload(b.String()), nil
}
