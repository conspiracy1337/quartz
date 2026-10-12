package tasks

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"quartz/internal/core/download"
	"quartz/internal/core/events"
	"quartz/internal/core/hashx"
	"quartz/internal/core/netx"
)

func TestMain(m *testing.M) {
	backoff = [...]time.Duration{5 * time.Millisecond, 10 * time.Millisecond, 20 * time.Millisecond}
	os.Exit(m.Run())
}

// srv serves named payloads and can fail, block or count concurrent requests.
type srv struct {
	*httptest.Server
	mu       sync.Mutex
	files    map[string][]byte
	failures map[string]int // remaining 500s per path
	missing  map[string]bool
	gate     chan struct{} // when set, requests block until it is closed
	inflight atomic.Int32
	peak     atomic.Int32
	hits     atomic.Int32
}

func newSrv(t *testing.T) *srv {
	s := &srv{files: map[string][]byte{}, failures: map[string]int{}, missing: map[string]bool{}}
	s.Server = httptest.NewTLSServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

func (s *srv) serve(w http.ResponseWriter, r *http.Request) {
	s.hits.Add(1)
	n := s.inflight.Add(1)
	defer s.inflight.Add(-1)
	for {
		p := s.peak.Load()
		if n <= p || s.peak.CompareAndSwap(p, n) {
			break
		}
	}
	s.mu.Lock()
	body, gate := s.files[r.URL.Path], s.gate
	fail := s.failures[r.URL.Path] > 0
	if fail {
		s.failures[r.URL.Path]--
	}
	missing := s.missing[r.URL.Path]
	s.mu.Unlock()
	switch {
	case missing:
		http.NotFound(w, r)
		return
	case fail:
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if gate != nil {
		w.Header().Set("ETag", `"x"`)
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		if r.Header.Get("Range") == "" {
			_, _ = w.Write(body[:len(body)/2])
			w.(http.Flusher).Flush()
		}
		select {
		case <-gate:
		case <-r.Context().Done():
		}
		return
	}
	w.Header().Set("ETag", `"x"`)
	http.ServeContent(w, r, "", time.Unix(1_700_000_000, 0), bytes.NewReader(body))
}

func (s *srv) setGate(g chan struct{}) { s.mu.Lock(); s.gate = g; s.mu.Unlock() }

type fixture struct {
	t    *testing.T
	root string
	srv  *srv
	eng  *Engine
	bus  *events.Bus
	dl   *download.Downloader
	nc   *netx.Client
}

func newFixture(t *testing.T, parallel int) *fixture {
	t.Helper()
	s := newSrv(t)
	nc, err := netx.New(netx.Proxy{Mode: netx.ProxyNone}, netx.Options{TLS: &tls.Config{
		RootCAs: s.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs, MinVersion: tls.VersionTLS12,
	}})
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, root: t.TempDir(), srv: s, bus: events.NewBus(), nc: nc}
	f.open(parallel)
	return f
}

func (f *fixture) open(parallel int) {
	ver, err := download.OpenVerified(filepath.Join(f.root, "verified.json"), f.root, slog.New(slog.DiscardHandler))
	if err != nil {
		f.t.Fatal(err)
	}
	f.dl = download.New(f.nc, ver)
	eng, err := New(Config{Downloader: f.dl, Root: f.root, HistoryPath: filepath.Join(f.root, "downloads.json"), Parallel: parallel, Bus: f.bus})
	if err != nil {
		f.t.Fatal(err)
	}
	f.eng = eng
	f.t.Cleanup(func() { _ = eng.Close() })
}

func (f *fixture) item(name string, size int) download.Item {
	body := bytes.Repeat([]byte{byte(len(name))}, size)
	f.srv.mu.Lock()
	f.srv.files["/"+name] = body
	f.srv.mu.Unlock()
	h := hashx.New(hashx.SHA1)
	h.Write(body)
	return download.Item{URLs: []string{f.srv.URL + "/" + name}, Dest: filepath.Join(f.root, "store", name), Size: int64(size), Sum: hashx.Of(hashx.SHA1, hashx.Hex(h))}
}

func (f *fixture) items(prefix string, n, size int) []Work {
	out := make([]Work, n)
	for i := range out {
		out[i] = Download(f.item(prefix+strconv.Itoa(i), size))
	}
	return out
}

func (f *fixture) wait(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return f.eng.Wait(ctx, id)
}

func (f *fixture) eventually(what string, cond func() bool) {
	f.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			f.t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (f *fixture) state(id string) State {
	for _, v := range f.eng.Snapshot().Active {
		if v.ID == id {
			return v.State
		}
	}
	for _, v := range f.eng.History() {
		if v.ID == id {
			return v.State
		}
	}
	return ""
}

func TestJobRunsStepsAndRecordsHistory(t *testing.T) {
	f := newFixture(t, 4)
	var snaps atomic.Int32
	f.bus.Subscribe(events.TasksChanged, func(any) { snaps.Add(1) })
	ran := false
	spec := NewJob(Meta{Title: "Minecraft 1.21.4"}).
		Step("Libraries", Bytes, f.items("lib", 5, 4096)...).
		Step("Assets", Count, f.items("asset", 20, 512)...).
		After("Natives", Count, Func("extract", func(context.Context) error { ran = true; return nil }))
	id, err := f.eng.Submit(spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.wait(id); err != nil {
		t.Fatal(err)
	}
	if !ran {
		t.Fatal("barrier step never ran")
	}
	h := f.eng.History()
	if len(h) != 1 || h[0].State != Done || h[0].DoneBytes != h[0].TotalBytes || h[0].TotalBytes != 5*4096+20*512 {
		t.Fatalf("history = %+v", h)
	}
	if len(f.eng.Snapshot().Active) != 0 {
		t.Fatal("finished job still active")
	}
	f.eventually("a tasks:changed event", func() bool { return snaps.Load() > 0 })

	// The same files again: nothing to do, no job, no request.
	hits := f.srv.hits.Load()
	id, err = f.eng.Submit(NewJob(Meta{Title: "again"}).Step("Libraries", Bytes, f.items("lib", 5, 4096)...))
	if err != nil || id != "" || f.wait(id) != nil || f.srv.hits.Load() != hits {
		t.Fatalf("verified files re-queued: id=%q err=%v", id, err)
	}
}

func TestConcurrencyLimitAndLiveResize(t *testing.T) {
	f := newFixture(t, 2)
	gate := make(chan struct{})
	f.srv.setGate(gate)
	id, err := f.eng.Submit(NewJob(Meta{Title: "many"}).Step("Files", Bytes, f.items("c", 8, 1000)...))
	if err != nil {
		t.Fatal(err)
	}
	f.eventually("2 transfers", func() bool { return f.srv.inflight.Load() == 2 })
	time.Sleep(30 * time.Millisecond)
	if f.srv.peak.Load() != 2 {
		t.Fatalf("peak %d with a limit of 2", f.srv.peak.Load())
	}
	f.eng.SetParallel(5)
	f.eventually("5 transfers", func() bool { return f.srv.inflight.Load() == 5 })
	f.srv.setGate(nil)
	close(gate)
	if err := f.wait(id); err != nil {
		t.Fatal(err)
	}
	if f.srv.peak.Load() != 5 {
		t.Fatalf("peak %d after resizing to 5", f.srv.peak.Load())
	}
}

func TestTransientErrorsAreRetried(t *testing.T) {
	f := newFixture(t, 2)
	it := f.item("flaky", 2000)
	f.srv.mu.Lock()
	f.srv.failures["/flaky"] = 2
	f.srv.mu.Unlock()
	id, _ := f.eng.Submit(NewJob(Meta{Title: "flaky"}).Step("Files", Bytes, Download(it)))
	if err := f.wait(id); err != nil {
		t.Fatalf("2 transient failures should be absorbed: %v", err)
	}
}

func TestPermanentFailureThenRetry(t *testing.T) {
	f := newFixture(t, 2)
	good, bad := f.item("good", 100), f.item("bad", 100)
	f.srv.mu.Lock()
	f.srv.missing["/bad"] = true
	f.srv.mu.Unlock()
	var barrierRuns atomic.Int32
	id, _ := f.eng.Submit(NewJob(Meta{Title: "mixed"}).
		Step("Files", Bytes, Download(good), Download(bad)).
		After("Needs files", Count, Func("needs files", func(context.Context) error { barrierRuns.Add(1); return nil })))
	err := f.wait(id)
	var je *JobError
	if !errors.As(err, &je) || je.Msg != "The server answered 404" {
		t.Fatalf("err = %v", err)
	}
	if f.srv.hits.Load() != 2 {
		t.Fatalf("404 must not be retried automatically (hits %d)", f.srv.hits.Load())
	}
	if barrierRuns.Load() != 0 {
		t.Fatal("barrier step ran although an earlier step failed")
	}
	h := f.eng.History()
	if h[0].State != Failed || !h[0].Retryable || h[0].Error == "" {
		t.Fatalf("history = %+v", h[0])
	}
	f.srv.mu.Lock()
	f.srv.missing["/bad"] = false
	f.srv.mu.Unlock()
	if err := f.eng.Retry(id); err != nil {
		t.Fatal(err)
	}
	if err := f.wait(id); err != nil {
		t.Fatal(err)
	}
	if barrierRuns.Load() != 1 || f.state(id) != Done {
		t.Fatalf("state after retry = %s", f.state(id))
	}
}

func TestPauseResumeUsesRange(t *testing.T) {
	f := newFixture(t, 1)
	it := f.item("big", 64_000)
	gate := make(chan struct{})
	f.srv.setGate(gate)
	id, _ := f.eng.Submit(NewJob(Meta{Title: "big"}).Step("Files", Bytes, Download(it)))
	f.eventually("half the file", func() bool {
		s := f.eng.Snapshot()
		return len(s.Active) == 1 && s.Active[0].DoneBytes == 32_000
	})
	if err := f.eng.Pause(id); err != nil {
		t.Fatal(err)
	}
	f.eventually("paused with nothing in flight", func() bool { return f.srv.inflight.Load() == 0 && f.state(id) == Paused })
	f.srv.setGate(nil)
	close(gate)
	hits := f.srv.hits.Load()
	if err := f.eng.Resume(id); err != nil {
		t.Fatal(err)
	}
	if err := f.wait(id); err != nil {
		t.Fatal(err)
	}
	if f.srv.hits.Load() != hits+1 {
		t.Fatal("resume needed more than one request")
	}
	if got, _ := os.ReadFile(it.Dest); len(got) != 64_000 {
		t.Fatalf("file has %d bytes", len(got))
	}
}

func TestCancelRemovesPartials(t *testing.T) {
	f := newFixture(t, 1)
	it := f.item("doomed", 10_000)
	gate := make(chan struct{})
	defer close(gate)
	f.srv.setGate(gate)
	id, _ := f.eng.Submit(NewJob(Meta{Title: "doomed"}).Step("Files", Bytes, Download(it)))
	f.eventually("transfer started", func() bool { return f.srv.inflight.Load() == 1 })
	if err := f.eng.Cancel(id); err != nil {
		t.Fatal(err)
	}
	if err := f.wait(id); !errors.Is(err, ErrCancelled) {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(it.Dest + ".part"); !os.IsNotExist(err) {
		t.Fatal(".part kept after cancel")
	}
	if f.eng.Retry(id) == nil {
		t.Fatal("cancelled job must not be retryable")
	}
}

func TestCloseRecordsInterruptedJobForRetryAfterRestart(t *testing.T) {
	f := newFixture(t, 1)
	it := f.item("long", 20_000)
	gate := make(chan struct{})
	f.srv.setGate(gate)
	id, _ := f.eng.Submit(NewJob(Meta{Title: "long"}).Step("Files", Bytes, Download(it)))
	f.eventually("transfer started", func() bool { return f.srv.inflight.Load() == 1 })
	if err := f.eng.Close(); err != nil {
		t.Fatal(err)
	}
	close(gate)
	f.srv.setGate(nil)
	f.open(1)
	h := f.eng.History()
	if len(h) != 1 || h[0].State != Failed || !h[0].Retryable || h[0].Error != "Stopped when the launcher closed" {
		t.Fatalf("history after restart = %+v", h)
	}
	if err := f.eng.Retry(id); err != nil {
		t.Fatal(err)
	}
	f.eventually("rebuilt job to finish", func() bool { return len(f.eng.Snapshot().Active) == 0 })
	if !f.dl.Ready(it) {
		t.Fatal("file not downloaded after retry")
	}
}

func TestHistoryPrunesAndDistrustsFile(t *testing.T) {
	f := newFixture(t, 1)
	_ = f.eng.Close()
	old := time.Now().Add(-25 * time.Hour).UnixMilli()
	recent := time.Now().Add(-time.Hour).UnixMilli()
	doc := `{"entries":[
	 {"view":{"id":"a","state":"done","finishedAt":` + strconv.FormatInt(old, 10) + `}},
	 {"view":{"id":"b","state":"failed","finishedAt":` + strconv.FormatInt(recent, 10) + `,"retryable":true},
	  "steps":[{"name":"x","unit":"bytes","items":[{"urls":["http://evil.example/x"],"dest":"` + filepath.ToSlash(filepath.Join(f.root, "x")) + `"}]}]},
	 {"view":{"id":"c","state":"failed","finishedAt":` + strconv.FormatInt(recent, 10) + `,"retryable":true},
	  "steps":[{"name":"x","unit":"bytes","items":[{"urls":["https://example.com/x"],"dest":"/etc/passwd"}]}]}]}`
	if err := os.WriteFile(filepath.Join(f.root, "downloads.json"), []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	f.open(1)
	h := f.eng.History()
	if len(h) != 2 {
		t.Fatalf("old entry not pruned: %+v", h)
	}
	for _, v := range h {
		if v.Retryable {
			t.Fatalf("entry %s with unsafe items is retryable", v.ID)
		}
		if f.eng.Retry(v.ID) == nil {
			t.Fatalf("unsafe entry %s was retried", v.ID)
		}
	}
}

func TestFriendly(t *testing.T) {
	cases := map[string]error{
		"The server answered 403":          &netx.StatusError{Code: 403},
		"The connection stalled":           netx.ErrIdleTimeout,
		"The file arrived damaged":         hashx.ErrMismatch,
		"Stopped when the launcher closed": errInterrupted,
	}
	for want, err := range cases {
		if got := Friendly(err); got != want {
			t.Errorf("Friendly(%v) = %q, want %q", err, got, want)
		}
	}
}
