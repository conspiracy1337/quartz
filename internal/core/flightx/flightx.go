// Package flightx merges concurrent calls for the same key, like singleflight,
// but the shared work runs on its own context. One caller giving up never
// fails the others; the work is cancelled only when every caller has gone.
package flightx

import (
	"context"
	"sync"
)

// Group is safe for concurrent use. The zero value is ready.
type Group[T any] struct {
	mu sync.Mutex
	m  map[string]*call[T]
}

type call[T any] struct {
	done      chan struct{}
	val       T
	err       error
	waiters   int
	abandoned bool // every caller left; the work is winding down
	cancel    context.CancelFunc
}

// Do runs fn once for all concurrent callers of key and returns its result.
// fn gets a context that keeps the first caller's values but ends only when
// every caller has left. A caller whose ctx ends returns ctx.Err() at once.
// Work that every caller abandoned is never joined: a new call waits for it
// to wind down and then starts afresh, so two runs never overlap.
func (g *Group[T]) Do(ctx context.Context, key string, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	for {
		g.mu.Lock()
		if g.m == nil {
			g.m = map[string]*call[T]{}
		}
		c := g.m[key]
		if c != nil && c.abandoned {
			g.mu.Unlock()
			select {
			case <-c.done:
				continue
			case <-ctx.Done():
				return zero, ctx.Err()
			}
		}
		if c == nil {
			c = g.start(ctx, key, fn)
		}
		c.waiters++
		g.mu.Unlock()

		select {
		case <-c.done:
			return c.val, c.err
		case <-ctx.Done():
			g.mu.Lock()
			if c.waiters--; c.waiters == 0 {
				c.abandoned = true
				c.cancel()
			}
			g.mu.Unlock()
			return zero, ctx.Err()
		}
	}
}

// start launches fn for key; g.mu must be held.
func (g *Group[T]) start(ctx context.Context, key string, fn func(context.Context) (T, error)) *call[T] {
	wctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	c := &call[T]{done: make(chan struct{}), cancel: cancel}
	g.m[key] = c
	go func() {
		defer cancel()
		c.val, c.err = fn(wctx)
		g.mu.Lock()
		delete(g.m, key)
		g.mu.Unlock()
		close(c.done)
	}()
	return c
}
