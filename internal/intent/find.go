package intent

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrNotFound means no instance or account matches.
	ErrNotFound = errors.New("not found")
	// ErrAmbiguous means a name matches more than one.
	ErrAmbiguous = errors.New("more than one match")
)

// Find picks the item whose id is q, or else the only one whose name is q
// (ignoring case). what names the kind of item in errors.
func Find[T any](items []T, q, what string, id, name func(T) string) (T, error) {
	var zero T
	for _, it := range items {
		if id(it) == q {
			return it, nil
		}
	}
	var hits []T
	for _, it := range items {
		if strings.EqualFold(name(it), q) {
			hits = append(hits, it)
		}
	}
	switch len(hits) {
	case 0:
		return zero, fmt.Errorf("%w: no %s is called %q", ErrNotFound, what, q)
	case 1:
		return hits[0], nil
	}
	ids := make([]string, len(hits))
	for i, h := range hits {
		ids[i] = id(h)
	}
	return zero, fmt.Errorf("%w: %d of your %ss are called %q, use an id: %s", ErrAmbiguous, len(hits), what, q, strings.Join(ids, ", "))
}
