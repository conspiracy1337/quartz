//go:build fakeauth

package app

import (
	"quartz/internal/auth"
	"quartz/internal/auth/authtest"
	"quartz/internal/core/netx"
)

// A development build (-tags fakeauth) signs in against a local fake of
// Microsoft, Xbox Live and Minecraft's services instead of the real ones, so
// the sign-in screens can be tried without an approved client ID. The device
// code flow completes after a few polls; the player is "Notch".
func init() {
	newAuth = func(h *netx.Client, _ string) *auth.Service {
		srv := authtest.New()
		srv.PendingPolls = 3
		return srv.Service(h, "00000000-0000-4000-8000-000000000000")
	}
}
