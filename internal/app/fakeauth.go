//go:build fakeauth

package app

import (
	"quartz/internal/auth"
	"quartz/internal/auth/authtest"
	"quartz/internal/core/netx"
	"quartz/internal/mcapi"
)

// A development build (-tags fakeauth) signs in against a local fake of
// Microsoft, Xbox Live and Minecraft's services instead of the real ones, so
// the sign-in screens can be tried without an approved client ID. The device
// code flow completes after a few polls; the player is "Notch", wearing
// Steve and owning the Migrator cape (real textures).
func init() {
	newAuth = func(h *netx.Client, _ string) *auth.Service {
		srv := authtest.New()
		srv.PendingPolls = 3
		srv.Profile.Skins = []mcapi.Skin{{ID: "steve", State: "ACTIVE", Variant: "CLASSIC", URL: "http://textures.minecraft.net/texture/31f477eb1a7beee631c2ca64d06f8f68fa93a3386d04452ab27f43acdf1b60cb"}}
		return srv.Service(h, "00000000-0000-4000-8000-000000000000")
	}
}
