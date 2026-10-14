package download

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/ulikunitz/xz/lzma"

	"quartz/internal/core/flightx"
	"quartz/internal/core/fsx"
	"quartz/internal/core/hashx"
	"quartz/internal/core/linkx"
	"quartz/internal/core/netx"
)

// Doer sends HTTP requests; *netx.Client implements it.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Progress receives how many bytes of the item are on disk so far. The value
// can go down when a transfer has to restart from zero.
type Progress func(done int64)

// Downloader fetches items. It is safe for concurrent use.
type Downloader struct {
	doer  Doer
	ver   *Verified
	group flightx.Group[struct{}]
	bufs  sync.Pool
}

// New returns a Downloader using doer and the verified-file index.
func New(doer Doer, ver *Verified) *Downloader {
	return &Downloader{doer: doer, ver: ver, bufs: sync.Pool{New: func() any { b := make([]byte, 64<<10); return &b }}}
}

// Ready reports, cheaply (index lookup plus one stat), whether it is already in
// place and verified.
func (d *Downloader) Ready(it Item) bool {
	size, sum := it.final()
	return d.ver.Has(it.Dest, size, sum)
}

// Verify checks a file the launcher holds but can't download again against
// its sum, hashing it only when it changed since the last check.
func (d *Downloader) Verify(it Item) error {
	if d.Ready(it) {
		return nil
	}
	size, sum := it.final()
	if sum.IsZero() {
		return fmt.Errorf("download: no checksum to verify %s against", filepath.Base(it.Dest))
	}
	info, err := os.Stat(it.Dest)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || (size > 0 && info.Size() != size) {
		return fmt.Errorf("%w (%s)", hashx.ErrMismatch, filepath.Base(it.Dest))
	}
	got, err := hashx.File(it.Dest, sum.Algo)
	if err != nil {
		return err
	}
	if err := sum.Check(got); err != nil {
		return err
	}
	d.ver.Add(it.Dest, sum)
	return nil
}

// Forget makes the next Fetch of it verify the file again (used by Repair).
func (d *Downloader) Forget(it Item) { d.ver.Forget(it.Dest) }

// Discard removes any partial download of it (used by Cancel).
func Discard(it Item) {
	_ = os.Remove(it.part())
	_ = os.Remove(it.meta())
}

// Fetch makes it.Dest hold the item's content, reusing a verified file, a
// same-content file elsewhere, or a resumable partial download when possible.
// Concurrent fetches of the same destination share one transfer, which
// stops only when every caller has given up (the .part file stays).
func (d *Downloader) Fetch(ctx context.Context, it Item, progress Progress) error {
	if progress == nil {
		progress = func(int64) {}
	}
	if d.Ready(it) {
		progress(it.Total())
		return nil
	}
	_, err := d.group.Do(ctx, it.Dest, func(ctx context.Context) (struct{}, error) { return struct{}{}, d.fetch(ctx, it, progress) })
	if err == nil {
		progress(it.Total()) // a caller that joined another's transfer saw no progress yet
	}
	return err
}

func (d *Downloader) fetch(ctx context.Context, it Item, progress Progress) error {
	size, sum := it.final()
	if ok, err := d.adoptExisting(it, size, sum); ok || err != nil {
		if ok {
			progress(it.Total())
		}
		return err
	}
	if err := os.MkdirAll(filepath.Dir(it.Dest), 0o700); err != nil {
		return err
	}
	var err error
	for i, url := range it.URLs {
		err = d.transfer(ctx, it, url, progress)
		if err == nil || ctx.Err() != nil {
			return err
		}
		if cls, _ := netx.Classify(err); cls != netx.Permanent || i == len(it.URLs)-1 {
			return err
		}
	}
	return err
}

// adoptExisting accepts a correct file already at Dest, or places a verified
// copy of the same content from elsewhere in the store by hardlink or copy.
func (d *Downloader) adoptExisting(it Item, size int64, sum hashx.Sum) (bool, error) {
	if info, err := os.Stat(it.Dest); err == nil && info.Mode().IsRegular() && (size == 0 || info.Size() == size) {
		if sum.IsZero() {
			d.ver.Add(it.Dest, sum)
			return true, nil
		}
		if got, err := hashx.File(it.Dest, sum.Algo); err == nil && got == sum.Hex {
			d.ver.Add(it.Dest, sum)
			return true, nil
		}
	}
	src, ok := d.ver.FindBySum(sum, size)
	if !ok || src == it.Dest {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(it.Dest), 0o700); err != nil {
		return false, err
	}
	perm := os.FileMode(fsx.FilePerm)
	if it.Exec {
		perm = 0o700
	}
	if _, err := linkx.Place(src, it.Dest, linkx.Options{Perm: perm}); err != nil {
		return false, nil //nolint:nilerr // falling back to a real download is the right recovery
	}
	d.ver.Add(it.Dest, sum)
	return true, nil
}

// partMeta remembers which URL a .part file came from and the validator that
// makes resuming it safe (If-Range).
type partMeta struct {
	URL       string `json:"url"`
	Validator string `json:"validator"`
}

func (d *Downloader) transfer(ctx context.Context, it Item, url string, progress Progress) error {
	h := hashx.New(it.Sum.Algo)
	offset, validator := resumePoint(it, url, h)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if offset > 0 {
		req.Header.Set("Range", "bytes="+strconv.FormatInt(offset, 10)+"-")
		req.Header.Set("If-Range", validator)
	}
	resp, err := d.doer.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	flags := os.O_CREATE | os.O_WRONLY
	switch {
	case resp.StatusCode == http.StatusPartialContent && offset > 0 && rangeStartsAt(resp.Header.Get("Content-Range"), offset):
		flags |= os.O_APPEND
	case resp.StatusCode == http.StatusOK:
		offset = 0
		h.Reset()
		flags |= os.O_TRUNC
	default:
		Discard(it)
		return fmt.Errorf("download: unexpected %d from %s", resp.StatusCode, url)
	}
	progress(offset)
	writeMeta(it, partMeta{URL: url, Validator: strongValidator(resp.Header)})

	f, err := os.OpenFile(it.part(), flags, fsx.FilePerm) //nolint:gosec // Dest was validated by Item.Validate
	if err != nil {
		return err
	}
	limit := int64(maxUnknownSize) - offset
	if it.Size > 0 {
		limit = it.Size - offset
	}
	bp := d.bufs.Get().(*[]byte)
	n, err := io.CopyBuffer(io.MultiWriter(f, h, &counter{base: offset, report: progress}), io.LimitReader(resp.Body, limit+1), *bp)
	d.bufs.Put(bp)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err // the .part file stays for a later resume
	}
	if n > limit {
		Discard(it)
		return &netx.PermanentError{Err: fmt.Errorf("download: %s is larger than expected", url)}
	}
	if it.Size > 0 && offset+n != it.Size {
		return io.ErrUnexpectedEOF
	}
	if err := it.Sum.Check(hashx.Hex(h)); err != nil {
		Discard(it)
		if offset > 0 {
			return err // a stale prefix may be to blame; one fresh attempt is worth it
		}
		return &netx.PermanentError{Err: err}
	}
	return d.finish(it)
}

// resumePoint hashes an existing .part prefix into h and returns its length and
// the validator for If-Range, or 0 when the transfer must start fresh.
func resumePoint(it Item, url string, h hash.Hash) (int64, string) {
	m, ok := readMeta(it)
	if !ok || m.URL != url || m.Validator == "" {
		return 0, ""
	}
	f, err := os.Open(it.part())
	if err != nil {
		return 0, ""
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || info.Size() == 0 || (it.Size > 0 && info.Size() >= it.Size) {
		return 0, ""
	}
	if _, err := io.Copy(h, f); err != nil {
		h.Reset()
		return 0, ""
	}
	return info.Size(), m.Validator
}

// finish decodes if needed, moves the verified file into place and records it.
func (d *Downloader) finish(it Item) error {
	size, sum := it.final()
	if it.Decode == DecodeLZMA {
		if err := decodeLZMA(it.part(), it.Dest, size, sum); err != nil {
			Discard(it)
			return &netx.PermanentError{Err: err}
		}
		_ = os.Remove(it.part())
	} else if err := os.Rename(it.part(), it.Dest); err != nil {
		return err
	}
	_ = os.Remove(it.meta())
	if it.Exec {
		if err := os.Chmod(it.Dest, 0o700); err != nil { //nolint:gosec // runtime executables must be runnable by the owner
			return err
		}
	}
	d.ver.Add(it.Dest, sum)
	return nil
}

func decodeLZMA(src, dst string, size int64, sum hashx.Sum) error {
	in, err := os.Open(src) //nolint:gosec // our own .part file
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	r, err := lzma.NewReader(in)
	if err != nil {
		return err
	}
	tmp := dst + ".decode"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fsx.FilePerm) //nolint:gosec // derived from a validated Dest
	if err != nil {
		return err
	}
	h := hashx.New(sum.Algo)
	n, err := io.Copy(io.MultiWriter(out, h), io.LimitReader(r, size+1))
	if err == nil {
		err = out.Sync()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && n != size {
		err = fmt.Errorf("download: decoded size %d, want %d", n, size)
	}
	if err == nil {
		err = sum.Check(hashx.Hex(h))
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

// strongValidator returns a validator usable with If-Range: a strong ETag, or
// Last-Modified. Weak ETags must not be used for ranges (RFC 9110 §13.1.5).
func strongValidator(h http.Header) string {
	if etag := h.Get("ETag"); etag != "" && !strings.HasPrefix(etag, "W/") {
		return etag
	}
	return h.Get("Last-Modified")
}

func rangeStartsAt(contentRange string, offset int64) bool {
	return strings.HasPrefix(contentRange, "bytes "+strconv.FormatInt(offset, 10)+"-")
}

func readMeta(it Item) (partMeta, bool) {
	data, err := fsx.ReadCapped(it.meta(), 8<<10)
	if err != nil {
		return partMeta{}, false
	}
	var m partMeta
	return m, json.Unmarshal(data, &m) == nil
}

func writeMeta(it Item, m partMeta) {
	if m.Validator == "" {
		_ = os.Remove(it.meta())
		return
	}
	if data, err := json.Marshal(m); err == nil {
		_ = fsx.AtomicWrite(it.meta(), data, fsx.FilePerm)
	}
}

// counter reports the running byte total of one transfer.
type counter struct {
	base   int64
	n      int64
	report Progress
}

func (c *counter) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	c.report(c.base + c.n)
	return len(p), nil
}

// IsHashMismatch reports whether err came from a failed checksum.
func IsHashMismatch(err error) bool { return errors.Is(err, hashx.ErrMismatch) }
