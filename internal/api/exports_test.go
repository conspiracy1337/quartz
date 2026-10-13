package api

import (
	"errors"
	"testing"

	"quartz/internal/exporting"
)

func TestExportsValidate(t *testing.T) {
	x := NewExports(&Ctx{}, ExportDeps{Busy: func(string) bool { return true }})
	if _, err := x.Export("../etc", exporting.Plan{Format: exporting.Zip}); !errors.Is(err, ErrBadInstanceID) {
		t.Fatalf("bad id: %v", err)
	}
	if _, err := x.Export("survival-abc1", exporting.Plan{Format: exporting.List, List: "xlsx"}); !errors.Is(err, exporting.ErrPlan) {
		t.Fatalf("bad plan: %v", err)
	}
	if _, err := x.Export("survival-abc1", exporting.Plan{Format: exporting.Mrpack}); !errors.Is(err, ErrBusy) {
		t.Fatalf("running: %v", err)
	}
}
