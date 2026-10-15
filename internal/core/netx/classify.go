package netx

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"time"
)

// Class says whether a failed request is worth repeating.
type Class int

const (
	// Retryable: transient network trouble, timeouts, 408, 425, 5xx.
	Retryable Class = iota
	// RateLimited: the host asked us to slow down (429, or 503 with Retry-After).
	RateLimited
	// Permanent: repeating cannot help (other 4xx, bad TLS, unsafe redirect, cancel).
	Permanent
)

// PermanentError marks any error as not worth retrying (for example a file
// that fails its hash check twice).
type PermanentError struct{ Err error }

func (e *PermanentError) Error() string { return e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }

// Classify returns the class of err and, for RateLimited, how long to wait
// (0 means the caller picks its own backoff).
func Classify(err error) (Class, time.Duration) {
	var (
		se   *StatusError
		pe   *PermanentError
		cert *tls.CertificateVerificationError
		unk  x509.UnknownAuthorityError
		host x509.HostnameError
	)
	switch {
	case err == nil:
		return Permanent, 0
	case errors.As(err, &pe),
		errors.Is(err, context.Canceled),
		errors.Is(err, ErrInsecureRedirect),
		errors.Is(err, ErrTooManyRedirects),
		errors.Is(err, ErrRedirectRefused),
		errors.Is(err, ErrNotPublic),
		errors.As(err, &cert), errors.As(err, &unk), errors.As(err, &host):
		return Permanent, 0
	case errors.As(err, &se):
		switch {
		case se.Code == http.StatusTooManyRequests,
			se.Code == http.StatusServiceUnavailable && se.RetryAfter > 0:
			return RateLimited, se.RetryAfter
		case se.Code == http.StatusRequestTimeout, se.Code == http.StatusTooEarly, se.Code >= 500:
			return Retryable, 0
		default:
			return Permanent, 0
		}
	default:
		// Resets, refused connections, DNS hiccups, stalls and unexpected EOFs.
		return Retryable, 0
	}
}
