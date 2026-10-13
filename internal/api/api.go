// Package api holds the only types bound to the frontend. Facades are thin: they
// validate input, call a service and return DTOs. No business logic lives here.
package api

import (
	"context"
	"sync/atomic"
)

// Ctx carries the Wails runtime context, which exists only after startup.
type Ctx struct {
	v atomic.Pointer[context.Context]
}

func (c *Ctx) Set(ctx context.Context) { c.v.Store(&ctx) }

func (c *Ctx) Get() context.Context {
	if p := c.v.Load(); p != nil {
		return *p
	}
	return context.Background()
}

// asList keeps an empty result a list for the UI: Wails sends a nil slice as
// null, which screens would take for "still loading".
func asList[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// asListErr is asList for a (slice, error) result.
func asListErr[T any](s []T, err error) ([]T, error) { return asList(s), err }
