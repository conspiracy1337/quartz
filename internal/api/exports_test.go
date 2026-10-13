package api

import (
	"errors"
	"testing"

	"quartz/internal/export"
)

func TestExportsValidate(t *testing.T) {
	x := NewExports(&Ctx{}, ExportDeps{Busy: func(string) bool { return true }})
	if _, err := x.Export("../etc", export.Plan{Format: export.Zip}); !errors.Is(err, ErrBadInstanceID) {
		t.Fatalf("bad id: %v", err)
	}
	if _, err := x.Export("survival-abc1", export.Plan{Format: export.List, List: "xlsx"}); !errors.Is(err, export.ErrPlan) {
		t.Fatalf("bad plan: %v", err)
	}
	if _, err := x.Export("survival-abc1", export.Plan{Format: export.Mrpack}); !errors.Is(err, ErrBusy) {
		t.Fatalf("running: %v", err)
	}
}
