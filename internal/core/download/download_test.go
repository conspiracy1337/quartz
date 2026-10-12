package download

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ulikunitz/xz/lzma"

	"quartz/internal/core/hashx"
	"quartz/internal/core/netx"
)

type fileServer struct {
	*httptest.Server
	mu      sync.Mutex
	files   map[string][]byte
	etag    string
	hits    atomic.Int32
	ranges  []string
	slow    chan struct{} // when set, the handler blocks after the first chunk
	noRange bool
}

func newFileServer(t *testing.T) *fileServer {
	fs := &fileServer{files: map[string][]byte{}, etag: `"v1"`}
	fs.Server = httptest.NewTLSServer(http.HandlerFunc(fs.serve))
	t.Cleanup(fs.Close)
	return fs
}

func (fs *fileServer) serve(w http.ResponseWriter, r *http.Request) {
	fs.hits.Add(1)
	fs.mu.Lock()
	body, ok := fs.files[r.URL.Path]
	fs.ranges = append(fs.ranges, r.Header.Get("Range"))
	etag, slow, noRange := fs.etag, fs.slow, fs.noRange
	fs.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	if slow != nil {
		w.Header().Set("ETag", etag)
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = w.Write(body[:len(body)/2])
		w.(http.Flusher).Flush()
		<-slow
		return
	}
	if noRange {
		_, _ = w.Write(body)
		return
	}
	w.Header().Set("ETag", etag)
	http.ServeContent(w, r, "", time.Unix(1_700_000_000, 0), bytes.NewReader(body))
}

type env struct {
	root string
	dl   *Downloader
	srv  *fileServer
}

func newEnv(t *testing.T) *env {
	t.Helper()
	srv := newFileServer(t)
	nc, err := netx.New(netx.Proxy{Mode: netx.ProxyNone}, netx.Options{TLS: &tls.Config{
		RootCAs: srv.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs, MinVersion: tls.VersionTLS12,
	}})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	ver, err := OpenVerified(filepath.Join(root, "verified.json"), root, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	return &env{root: root, dl: New(nc, ver), srv: srv}
}

func (e *env) item(name string, body []byte) Item {
	e.srv.mu.Lock()
	e.srv.files["/"+name] = body
	e.srv.mu.Unlock()
	h := hashx.New(hashx.SHA1)
	h.Write(body)
	return Item{URLs: []string{e.srv.URL + "/" + name}, Dest: filepath.Join(e.root, "store", name), Size: int64(len(body)), Sum: hashx.Of(hashx.SHA1, hashx.Hex(h))}
}

func payload(n int) []byte { return bytes.Repeat([]byte("minecraft!"), n/10+1)[:n] }

func readFile(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestFetchVerifyAndSkip(t *testing.T) {
	e := newEnv(t)
	it := e.item("a.jar", payload(100_000))
	var last int64
	if err := e.dl.Fetch(context.Background(), it, func(n int64) { last = n }); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(readFile(t, it.Dest), payload(100_000)) || last != it.Size {
		t.Fatalf("content or progress wrong (progress %d)", last)
	}
	if err := e.dl.Fetch(context.Background(), it, nil); err != nil || e.srv.hits.Load() != 1 {
		t.Fatalf("verified file refetched: hits=%d err=%v", e.srv.hits.Load(), err)
	}
	if _, err := os.Stat(it.part()); !os.IsNotExist(err) {
		t.Fatal(".part left behind")
	}
}

func TestResumeWithRange(t *testing.T) {
	e := newEnv(t)
	body := payload(50_000)
	it := e.item("b.jar", body)
	if err := os.MkdirAll(filepath.Dir(it.Dest), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(it.part(), body[:20_000], 0o600); err != nil {
		t.Fatal(err)
	}
	writeMeta(it, partMeta{URL: it.URLs[0], Validator: `"v1"`})
	var first int64 = -1
	err := e.dl.Fetch(context.Background(), it, func(n int64) {
		if first < 0 {
			first = n
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if e.srv.ranges[0] != "bytes=20000-" || first != 20_000 {
		t.Fatalf("range=%q first progress=%d", e.srv.ranges[0], first)
	}
	if !bytes.Equal(readFile(t, it.Dest), body) {
		t.Fatal("resumed file corrupt")
	}
}

func TestResumeRestartsWhenValidatorChanged(t *testing.T) {
	e := newEnv(t)
	body := payload(30_000)
	it := e.item("c.jar", body)
	_ = os.MkdirAll(filepath.Dir(it.Dest), 0o700)
	_ = os.WriteFile(it.part(), []byte("old content from an older file"), 0o600)
	writeMeta(it, partMeta{URL: it.URLs[0], Validator: `"v0"`})
	if err := e.dl.Fetch(context.Background(), it, nil); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(readFile(t, it.Dest), body) {
		t.Fatal("If-Range mismatch not handled with a full download")
	}
}

func TestCorruptPrefixIsRetryableOnce(t *testing.T) {
	e := newEnv(t)
	body := payload(30_000)
	it := e.item("d.jar", body)
	_ = os.MkdirAll(filepath.Dir(it.Dest), 0o700)
	_ = os.WriteFile(it.part(), bytes.Repeat([]byte("X"), 10_000), 0o600)
	writeMeta(it, partMeta{URL: it.URLs[0], Validator: `"v1"`})
	err := e.dl.Fetch(context.Background(), it, nil)
	if !IsHashMismatch(err) {
		t.Fatalf("err = %v", err)
	}
	if cls, _ := netx.Classify(err); cls != netx.Retryable {
		t.Fatalf("corrupt prefix should be retryable, got %v", cls)
	}
	if err := e.dl.Fetch(context.Background(), it, nil); err != nil {
		t.Fatalf("fresh retry failed: %v", err)
	}
}

func TestFreshHashMismatchIsPermanent(t *testing.T) {
	e := newEnv(t)
	it := e.item("e.jar", payload(1000))
	it.Sum = hashx.Of(hashx.SHA1, strings.Repeat("0", 40))
	err := e.dl.Fetch(context.Background(), it, nil)
	if cls, _ := netx.Classify(err); !IsHashMismatch(err) || cls != netx.Permanent {
		t.Fatalf("err=%v class=%v", err, cls)
	}
	if _, err := os.Stat(it.Dest); !os.IsNotExist(err) {
		t.Fatal("unverified file moved into place")
	}
}

func TestMirrorFallbackAndOversize(t *testing.T) {
	e := newEnv(t)
	it := e.item("f.jar", payload(500))
	it.URLs = append([]string{e.srv.URL + "/missing"}, it.URLs...)
	if err := e.dl.Fetch(context.Background(), it, nil); err != nil {
		t.Fatalf("mirror not tried: %v", err)
	}
	big := e.item("g.jar", payload(5000))
	big.Size, big.Sum = 100, hashx.Sum{}
	err := e.dl.Fetch(context.Background(), big, nil)
	if cls, _ := netx.Classify(err); cls != netx.Permanent {
		t.Fatalf("oversize body: %v (%v)", err, cls)
	}
}

func TestSameContentIsLinkedNotDownloaded(t *testing.T) {
	e := newEnv(t)
	it := e.item("h.jar", payload(2000))
	if err := e.dl.Fetch(context.Background(), it, nil); err != nil {
		t.Fatal(err)
	}
	twin := it
	twin.Dest = filepath.Join(e.root, "store", "other", "h-copy.jar")
	if err := e.dl.Fetch(context.Background(), twin, nil); err != nil {
		t.Fatal(err)
	}
	if e.srv.hits.Load() != 1 || !bytes.Equal(readFile(t, twin.Dest), payload(2000)) {
		t.Fatalf("twin not placed from store (hits %d)", e.srv.hits.Load())
	}
}

func TestPauseKeepsPartForResume(t *testing.T) {
	e := newEnv(t)
	body := payload(40_000)
	it := e.item("i.jar", body)
	release := make(chan struct{})
	e.srv.mu.Lock()
	e.srv.slow = release
	e.srv.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan error, 1)
	var seen atomic.Int64
	go func() { got <- e.dl.Fetch(ctx, it, func(n int64) { seen.Store(n) }) }()
	for seen.Load() < 20_000 {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	if err := <-got; err == nil {
		t.Fatal("cancelled fetch reported success")
	}
	close(release)
	if info, err := os.Stat(it.part()); err != nil || info.Size() != 20_000 {
		t.Fatalf("part not kept: %v", err)
	}
	e.srv.mu.Lock()
	e.srv.slow = nil
	e.srv.mu.Unlock()
	if err := e.dl.Fetch(context.Background(), it, nil); err != nil {
		t.Fatal(err)
	}
	if last := e.srv.ranges[len(e.srv.ranges)-1]; last != "bytes=20000-" {
		t.Fatalf("resume range = %q", last)
	}
	if !bytes.Equal(readFile(t, it.Dest), body) {
		t.Fatal("resumed content wrong")
	}
}

func TestServerWithoutRangeSupport(t *testing.T) {
	e := newEnv(t)
	body := payload(10_000)
	it := e.item("j.jar", body)
	e.srv.noRange = true
	_ = os.MkdirAll(filepath.Dir(it.Dest), 0o700)
	_ = os.WriteFile(it.part(), body[:4000], 0o600)
	writeMeta(it, partMeta{URL: it.URLs[0], Validator: `"v1"`})
	var minSeen int64 = 1 << 40
	if err := e.dl.Fetch(context.Background(), it, func(n int64) { minSeen = min(minSeen, n) }); err != nil {
		t.Fatal(err)
	}
	if minSeen != 0 || !bytes.Equal(readFile(t, it.Dest), body) {
		t.Fatalf("200 after Range must restart from zero (min progress %d)", minSeen)
	}
}

func TestLZMADecode(t *testing.T) {
	e := newEnv(t)
	raw := payload(20_000)
	var comp bytes.Buffer
	w, err := lzma.NewWriter(&comp)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write(raw)
	_ = w.Close()
	it := e.item("java.lzma", comp.Bytes())
	h := hashx.New(hashx.SHA1)
	h.Write(raw)
	it.Decode = DecodeLZMA
	it.Raw = &Check{Size: int64(len(raw)), Sum: hashx.Of(hashx.SHA1, hashx.Hex(h))}
	it.Exec = true
	if err := it.Validate(e.root); err != nil {
		t.Fatal(err)
	}
	if err := e.dl.Fetch(context.Background(), it, nil); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(readFile(t, it.Dest), raw) {
		t.Fatal("decoded content wrong")
	}
	if !e.dl.Ready(it) {
		t.Fatal("decoded file not recorded as verified")
	}
}

func TestValidate(t *testing.T) {
	root := t.TempDir()
	ok := Item{URLs: []string{"https://example.com/x"}, Dest: filepath.Join(root, "a", "x")}
	if err := ok.Validate(root); err != nil {
		t.Fatal(err)
	}
	bad := map[string]Item{
		"http":     {URLs: []string{"http://example.com/x"}, Dest: ok.Dest},
		"userinfo": {URLs: []string{"https://u:p@example.com/x"}, Dest: ok.Dest},
		"outside":  {URLs: ok.URLs, Dest: filepath.Join(root, "..", "x")},
		"relative": {URLs: ok.URLs, Dest: "x"},
		"no url":   {Dest: ok.Dest},
		"bad sum":  {URLs: ok.URLs, Dest: ok.Dest, Sum: hashx.Sum{Algo: hashx.SHA1, Hex: "zz"}},
		"lzma":     {URLs: ok.URLs, Dest: ok.Dest, Decode: DecodeLZMA},
	}
	for name, it := range bad {
		if err := it.Validate(root); !errors.Is(err, ErrInvalidItem) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

var _ io.Writer = (*counter)(nil)
