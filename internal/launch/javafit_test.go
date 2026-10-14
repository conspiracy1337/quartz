package launch

import (
	"errors"
	"testing"

	"quartz/internal/instance"
)

func TestJavaFits(t *testing.T) {
	cases := []struct {
		have, need int
		loader     string
		fails      bool
		warns      bool
	}{
		{have: 21, need: 21},
		{have: 17, need: 21, fails: true}, // 1.20.5+ on Java 17
		{have: 8, need: 17, fails: true},
		{have: 21, need: 17},                              // newer is fine
		{have: 17, need: 8},                               // vanilla 1.12 on 17 runs
		{have: 17, need: 8, loader: "forge", warns: true}, // old Forge doesn't
		{have: 0, need: 21},                               // not probed: no verdict
	}
	for _, c := range cases {
		warn, err := javaFits(c.have, c.need, instance.Instance{Minecraft: "1.x", Loader: c.loader})
		if (err != nil) != c.fails || (err != nil && !errors.Is(err, ErrJavaTooOld)) {
			t.Errorf("%+v: err %v", c, err)
		}
		if (warn != "") != c.warns {
			t.Errorf("%+v: warn %q", c, warn)
		}
	}
}
