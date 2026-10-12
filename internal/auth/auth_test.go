package auth

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"quartz/internal/auth/msa"
	"quartz/internal/core/netx"
	"quartz/internal/mcapi"
)

func TestDescribeAndClassify(t *testing.T) {
	cases := []struct {
		err       error
		reason    string
		reauth    bool
		transient bool
	}{
		{msa.ErrNotConfigured, "not-configured", false, false},
		{fmt.Errorf("x: %w", msa.ErrReauth), "reauth", true, false},
		{msa.ErrDenied, "cancelled", false, false},
		{context.Canceled, "cancelled", false, false},
		{msa.ErrExpired, "expired", false, false},
		{mcapi.ErrNotOwned, "not-owned", true, false},
		{mcapi.ErrNoProfile, "no-profile", true, false},
		{&netx.StatusError{Code: 429}, "rate-limited", false, true},
		{&netx.StatusError{Code: 503}, "network", false, true},
		{errors.New("connection reset"), "network", false, true},
		{&netx.StatusError{Code: 400}, "failed", false, false},
	}
	for _, c := range cases {
		if p := Describe(c.err); p.Reason != c.reason || p.Message == "" {
			t.Errorf("%v: %+v", c.err, p)
		}
		if NeedsReauth(c.err) != c.reauth || Transient(c.err) != c.transient {
			t.Errorf("%v: reauth=%v transient=%v", c.err, NeedsReauth(c.err), Transient(c.err))
		}
	}
}
