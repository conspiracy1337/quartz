package modpack

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"quartz/internal/core/archive"
	"quartz/internal/core/hashx"
)

// A pack the Import screen previews is read in place over HTTP Range
// requests: zip keeps its directory at the end, so the index (or manifest)
// and the list of override files cost a few small requests however big the
// pack is. Nothing installs from those bytes: the whole archive downloads in
// the background, checked against the platform's hash, and the pack is read
// again from it before it installs (Complete).

const (
	// peekFrom is the size from which a pack is previewed in place; a
	// smaller one downloads whole as quickly.
	peekFrom = 8 << 20
	// rangeBlock is the unit remote reads are fetched and cached in.
	rangeBlock = 256 << 10
	// maxPeek caps what one preview may fetch before the whole file is
	// downloaded instead (an index can't sensibly be larger).
	maxPeek = 48 << 20
)

// errNoRange is returned when a server ignores Range requests.
var errNoRange = errors.New("modpack: the server can't send part of the pack")

// remoteFile is an io.ReaderAt over an https file of a known size, fetched
// in rangeBlock blocks (runs of missing blocks in one request) and cached.
type remoteFile struct {
	ctx     context.Context //nolint:containedctx // io.ReaderAt has no ctx; the reader lives for one call
	get     func(ctx context.Context, rawURL string, h http.Header) (*http.Response, error)
	url     string
	size    int64
	blocks  map[int64][]byte
	fetched int64
}

func (f *remoteFile) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= f.size {
		return 0, io.EOF
	}
	end := min(off+int64(len(p)), f.size)
	first, last := off/rangeBlock, (end-1)/rangeBlock
	for b := first; b <= last; {
		if _, ok := f.blocks[b]; ok {
			b++
			continue
		}
		run := b
		for run+1 <= last {
			if _, ok := f.blocks[run+1]; ok {
				break
			}
			run++
		}
		if err := f.fetch(b, run); err != nil {
			return 0, err
		}
		b = run + 1
	}
	n := 0
	for pos := off; pos < end; {
		blk := f.blocks[pos/rangeBlock]
		n += copy(p[n:end-off], blk[pos%rangeBlock:])
		pos = off + int64(n)
	}
	if end < off+int64(len(p)) {
		return n, io.EOF
	}
	return n, nil
}

// fetch loads blocks first..last in one Range request.
func (f *remoteFile) fetch(first, last int64) error {
	from, to := first*rangeBlock, min((last+1)*rangeBlock, f.size)-1
	want := to - from + 1
	if f.fetched+want > maxPeek {
		return errors.New("modpack: the pack's index is too large to read in place")
	}
	resp, err := f.get(f.ctx, f.url, http.Header{"Range": {fmt.Sprintf("bytes=%d-%d", from, to)}})
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusPartialContent || !rangeMatches(resp.Header.Get("Content-Range"), from, to, f.size) {
		return errNoRange
	}
	buf, err := io.ReadAll(io.LimitReader(resp.Body, want+1))
	if err != nil {
		return err
	}
	if int64(len(buf)) != want {
		return errNoRange
	}
	f.fetched += want
	for b := first; b <= last; b++ {
		lo := (b - first) * rangeBlock
		f.blocks[b] = buf[lo:min(lo+rangeBlock, want)]
	}
	return nil
}

// rangeMatches checks a Content-Range header names exactly the bytes asked
// for out of a file of the expected size.
func rangeMatches(h string, from, to, size int64) bool {
	spec, ok := strings.CutPrefix(h, "bytes ")
	if !ok {
		return false
	}
	span, total, ok := strings.Cut(spec, "/")
	a, b, ok2 := strings.Cut(span, "-")
	if !ok || !ok2 {
		return false
	}
	return num(a) == from && num(b) == to && num(total) == size
}

// num is a decimal number, or -1.
func num(s string) int64 {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return -1
	}
	return v
}

// prefetching is a previewed pack's archive downloading in the background.
type prefetching struct {
	done   chan struct{}
	cancel context.CancelFunc
	path   string
	err    error
}

// open reads the pack file at raw: in place when peek allows it (Import's
// preview of a large pack), else downloaded and read from disk.
func (r *Reader) open(ctx context.Context, raw string, size int64, sum hashx.Sum, peek bool) (*Pack, error) {
	if peek && r.d.Get != nil && size >= peekFrom && size <= maxPackFile {
		p, err := r.peek(ctx, raw, size)
		if err == nil {
			p.download = r.prefetch(raw, size, sum)
			return p, nil
		}
		r.d.Log.Debug("modpack: reading in place failed; downloading the whole pack", "err", err)
	}
	dest, err := r.fetch(ctx, raw, size, sum)
	if err != nil {
		return nil, err
	}
	return r.File(ctx, dest)
}

// peek reads a remote pack's index and override listing in place.
func (r *Reader) peek(ctx context.Context, raw string, size int64) (*Pack, error) {
	zr, err := archive.NewReader(&remoteFile{ctx: ctx, get: r.d.Get, url: raw, size: size, blocks: map[int64][]byte{}}, size)
	if err != nil {
		return nil, err
	}
	p, err := r.read(ctx, zr)
	if err != nil {
		return nil, err
	}
	p.listing = map[string][]archive.Entry{}
	for _, layer := range p.Layers {
		p.listing[layer] = zr.Entries(layer)
	}
	return p, nil
}

// prefetch downloads the whole archive in the background for Complete.
func (r *Reader) prefetch(raw string, size int64, sum hashx.Sum) *prefetching {
	ctx, cancel := context.WithCancel(context.Background())
	w := &prefetching{done: make(chan struct{}), cancel: cancel}
	go func() {
		defer close(w.done)
		w.path, w.err = r.fetch(ctx, raw, size, sum)
	}()
	return w
}

// Complete makes a previewed pack installable: it waits for its archive,
// checked against the platform's hash, and reads the pack again from it,
// keeping what the platform said about it. Packs read whole are left as
// they are.
func (r *Reader) Complete(ctx context.Context, p *Pack) error {
	w := p.download
	if w == nil {
		return nil
	}
	select {
	case <-w.done:
	case <-ctx.Done():
		return ctx.Err()
	}
	if w.err != nil {
		return w.err
	}
	q, err := r.File(ctx, w.path)
	if err != nil {
		return err
	}
	q.Origin, q.Name, q.Summary, q.IconURL, q.Version = p.Origin, p.Name, p.Summary, p.IconURL, p.Version
	*p = *q
	return nil
}

// discardPending stops a background download and deletes what it wrote.
func (r *Reader) discardPending(w *prefetching) {
	w.cancel()
	go func() {
		<-w.done
		if w.path != "" {
			_ = os.Remove(w.path)
		}
	}()
}
