package flightx

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestOneCallerLeavingDoesNotFailTheOthers(t *testing.T) {
	var g Group[int]
	var runs atomic.Int32
	release := make(chan struct{})
	work := func(ctx context.Context) (int, error) {
		runs.Add(1)
		select {
		case <-release:
			return 42, nil
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	first, cancelFirst := context.WithCancel(context.Background())
	firstErr := make(chan error, 1)
	go func() { _, err := g.Do(first, "k", work); firstErr <- err }()
	waitFor(t, func() bool { return runs.Load() == 1 })
	second := make(chan int, 1)
	go func() { v, _ := g.Do(context.Background(), "k", work); second <- v }()
	waitFor(t, func() bool { g.mu.Lock(); defer g.mu.Unlock(); return g.m["k"] != nil && g.m["k"].waiters == 2 })

	cancelFirst()
	if err := <-firstErr; !errors.Is(err, context.Canceled) {
		t.Fatalf("first caller: %v", err)
	}
	close(release)
	if v := <-second; v != 42 {
		t.Fatalf("second caller got %d; the shared work was cancelled with the first", v)
	}
	if runs.Load() != 1 {
		t.Fatalf("work ran %d times", runs.Load())
	}
}

func TestWorkStopsWhenEveryCallerLeaves(t *testing.T) {
	var g Group[int]
	stopped := make(chan struct{})
	finish := make(chan struct{})
	var runs atomic.Int32
	work := func(ctx context.Context) (int, error) {
		if runs.Add(1) > 1 {
			return 7, nil
		}
		<-ctx.Done()
		close(stopped)
		<-finish // still winding down
		return 0, ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := g.Do(ctx, "k", work); done <- err }()
	waitFor(t, func() bool { return runs.Load() == 1 })
	cancel()
	<-done
	<-stopped

	// A new call must not join the abandoned run, nor overlap it.
	got := make(chan int, 1)
	go func() { v, _ := g.Do(context.Background(), "k", work); got <- v }()
	time.Sleep(20 * time.Millisecond)
	if runs.Load() != 1 {
		t.Fatal("a new run started while the abandoned one was still running")
	}
	close(finish)
	if v := <-got; v != 7 || runs.Load() != 2 {
		t.Fatalf("after the abandoned run: %d (runs %d)", v, runs.Load())
	}
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	for range 500 {
		if ok() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("timed out")
}
