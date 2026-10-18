package api

import (
	"testing"
	"time"

	"quartz/internal/accounts"
)

func TestViewStatus(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	ms := func(expires time.Duration, reauth bool) accounts.Account {
		return accounts.Account{Kind: accounts.Microsoft, ExpiresAt: now.Add(expires), Reauth: reauth, Problem: "The password was changed."}
	}
	cases := map[string]struct {
		a              accounts.Account
		status, detail string
	}{
		"valid":   {ms(time.Hour, false), "valid", ""},
		"expired": {ms(-time.Minute, false), "expired", ""},
		"at end":  {ms(0, false), "expired", ""},
		"signin":  {ms(time.Hour, true), "signin", "The password was changed."},
		"offline": {accounts.Account{Kind: accounts.Offline}, "offline", "Local account, for testing"},
	}
	for name, c := range cases {
		if v := view(c.a, "", now); v.Status != c.status || v.Detail != c.detail {
			t.Errorf("%s: got %q %q, want %q %q", name, v.Status, v.Detail, c.status, c.detail)
		}
	}
}
