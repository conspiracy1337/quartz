// Package logging sets up the launcher's structured log: JSON lines in a
// size-rotated file, with every string run through the shared Redactor.
package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"quartz/internal/core/fsx"
)

const (
	fileName = "launcher.log"
	maxSize  = 5 << 20 // rotate at 5 MB
	keep     = 3       // launcher.log.1 … launcher.log.3
)

// New returns a logger writing to dir/launcher.log, and a closer for shutdown.
func New(dir string, level slog.Level, r *Redactor) (*slog.Logger, io.Closer, error) {
	w, err := openRotating(filepath.Join(dir, fileName))
	if err != nil {
		return nil, nil, err
	}
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})
	return slog.New(&redactHandler{next: h, r: r}), w, nil
}

// redactHandler redacts the message and every string attribute.
type redactHandler struct {
	next slog.Handler
	r    *Redactor
}

func (h *redactHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.next.Enabled(ctx, l)
}

func (h *redactHandler) Handle(ctx context.Context, rec slog.Record) error {
	out := slog.NewRecord(rec.Time, rec.Level, h.r.Redact(rec.Message), rec.PC)
	rec.Attrs(func(a slog.Attr) bool {
		out.AddAttrs(h.redactAttr(a))
		return true
	})
	return h.next.Handle(ctx, out)
}

func (h *redactHandler) redactAttr(a slog.Attr) slog.Attr {
	v := a.Value.Resolve()
	switch v.Kind() {
	case slog.KindString:
		return slog.String(a.Key, h.r.Redact(v.String()))
	case slog.KindGroup:
		attrs := v.Group()
		out := make([]any, len(attrs))
		for i, g := range attrs {
			out[i] = h.redactAttr(g)
		}
		return slog.Group(a.Key, out...)
	case slog.KindAny:
		if err, ok := v.Any().(error); ok {
			return slog.String(a.Key, h.r.Redact(err.Error()))
		}
	}
	return a
}

func (h *redactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	red := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		red[i] = h.redactAttr(a)
	}
	return &redactHandler{next: h.next.WithAttrs(red), r: h.r}
}

func (h *redactHandler) WithGroup(name string) slog.Handler {
	return &redactHandler{next: h.next.WithGroup(name), r: h.r}
}

// rotatingFile is a minimal size-based rotating writer.
type rotatingFile struct {
	mu   sync.Mutex
	path string
	f    *os.File
	size int64
}

func openRotating(path string) (*rotatingFile, error) {
	rf := &rotatingFile{path: path}
	return rf, rf.open()
}

func (w *rotatingFile) open() error {
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, fsx.FilePerm)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	w.f, w.size = f, info.Size()
	return nil
}

func (w *rotatingFile) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.size+int64(len(p)) > maxSize {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := w.f.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *rotatingFile) rotate() error {
	if err := w.f.Close(); err != nil {
		return err
	}
	for i := keep - 1; i >= 1; i-- {
		_ = os.Rename(numbered(w.path, i), numbered(w.path, i+1))
	}
	_ = os.Rename(w.path, numbered(w.path, 1))
	return w.open()
}

func numbered(path string, i int) string {
	return path + "." + strconv.Itoa(i)
}

func (w *rotatingFile) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.f.Close()
}
