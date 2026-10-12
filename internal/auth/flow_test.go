package auth_test

import (
	"context"
	"strings"
	"testing"

	"quartz/internal/auth"
	"quartz/internal/auth/authtest"
	"quartz/internal/core/netx"
)

// fakeServices answers every step of a sign-in.
func fakeServices(t *testing.T, xErr int64) *auth.Service {
	t.Helper()
	srv := authtest.New()
	srv.XErr = xErr
	t.Cleanup(srv.Close)
	h, err := netx.New(netx.Proxy{Mode: netx.ProxyNone}, netx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return srv.Service(h, "11111111-2222-3333-4444-555555555555")
}

func TestRefreshRunsEveryStep(t *testing.T) {
	s := fakeServices(t, 0)
	var steps []string
	m, err := s.MSA.Refresh(context.Background(), "fake-refresh-0")
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.Complete(context.Background(), m, func(st string) { steps = append(steps, st) })
	if err != nil || res.MSA.Refresh == "" || res.MC.XUID != authtest.XUID || res.Profile.Name != "Notch" {
		t.Fatalf("%+v %v", res, err)
	}
	if strings.Join(steps, ",") != auth.StepXbox+","+auth.StepMinecraft+","+auth.StepProfile {
		t.Fatalf("steps %v", steps)
	}
}

func TestXboxRefusalNeedsReauth(t *testing.T) {
	_, err := fakeServices(t, 2148916238).Refresh(context.Background(), "fake-refresh-0")
	if !auth.NeedsReauth(err) || auth.Transient(err) {
		t.Fatalf("child account: %v", err)
	}
	if p := auth.Describe(err); p.Reason != "child-account" || p.FixURL == "" {
		t.Fatalf("describe: %+v", p)
	}
}

