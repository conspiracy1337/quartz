//go:build integration

package integration

import (
	"testing"

	"quartz/internal/core/tasks"
	"quartz/internal/java"
)

func TestInstallJavaFromEveryVendor(t *testing.T) {
	for _, vendor := range []string{"temurin", "zulu", "mojang"} {
		t.Run(vendor, func(t *testing.T) {
			h := newHarness(t)
			h.vendor = vendor
			plan, err := h.java.Plan(h.ctx(), 21, "")
			if err != nil {
				t.Fatal(err)
			}
			if plan.Vendor != vendor {
				t.Fatalf("planned %s, wanted %s", plan.Vendor, vendor)
			}
			h.run(plan.AddTo(tasks.NewJob(tasks.Meta{Title: "Java 21"})))
			rt, again, err := h.java.Resolve(h.ctx(), java.Request{Major: 21})
			if err != nil || again != nil {
				t.Fatalf("installed runtime not found: %v %v", err, again)
			}
			probed, err := h.java.Probe(h.ctx(), rt.Path)
			if err != nil || probed.Major != 21 {
				t.Fatalf("installed runtime does not run: %+v %v", probed, err)
			}
			t.Logf("%s: %s (%s) at %s", vendor, probed.Version, probed.Vendor, rt.Path)
		})
	}
}
