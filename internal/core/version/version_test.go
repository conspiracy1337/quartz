package version

import "testing"

func TestCompare(t *testing.T) {
	ordered := []string{"0.16.9", "0.16.10", "0.17.0-beta.1", "0.17.0-beta.2", "0.17.0", "21.1.0-beta", "21.1.0", "21.1.77"}
	for i := 1; i < len(ordered); i++ {
		if Compare(ordered[i-1], ordered[i]) >= 0 {
			t.Errorf("%s should sort below %s", ordered[i-1], ordered[i])
		}
	}
}

func TestSatisfies(t *testing.T) {
	cases := []struct {
		st     Style
		r, v   string
		in, ok bool
	}{
		// Sodium's breaks on Iris: a current Iris is fine.
		{Fabric, "<1.6.0", "1.7.0+mc1.20.1", false, true},
		{Fabric, "<1.6.0", "1.5.2+mc1.20.1", true, true},
		{Fabric, "*", "${version}", true, true},
		{Fabric, "", "anything", true, true},
		{Fabric, ">=0.5 <0.6", "0.5.8", true, true},
		{Fabric, ">=0.5 <0.6", "0.6.0-beta.1", true, true}, // a 0.6 beta precedes 0.6
		{Fabric, ">=0.5 <0.6", "0.6.0", false, true},
		{Fabric, "~1.20", "1.20.4", true, true},
		{Fabric, "~1.20", "1.21", false, true},
		{Fabric, "~1.20.1", "1.20.6", true, true},
		{Fabric, "^2.1", "2.9.9", true, true},
		{Fabric, "^2.1", "3.0", false, true},
		{Fabric, "1.20.x", "1.20.1", true, true},
		{Fabric, "1.20.x", "1.21", false, true},
		{Fabric, "1.20.1", "1.20.1", true, true},
		{Fabric, "=1.20.1", "1.20.2", false, true},
		{Fabric, "1.19.2||1.20.1", "1.20.1", true, true},
		{Fabric, "1.19.2||1.20.1", "1.21", false, true},
		// Things it can't judge.
		{Fabric, ">=1.0", "mc1.20.1-0.5.3", false, false},
		{Fabric, ">=1.0", "${version}", false, false},
		{Fabric, ">=1.0", "24w14a", false, false},
		{Fabric, ">=weird", "1.0", false, false},
		{Fabric, "1.19.2||nonsense", "1.20.1", false, false},
		{Fabric, "1.19.2||nonsense", "1.19.2", true, true},
		// Maven.
		{Maven, "[1.20.1,1.21)", "1.20.4", true, true},
		{Maven, "[1.20.1,1.21)", "1.21", false, true},
		{Maven, "(,1.5]", "1.5", true, true},
		{Maven, "(,1.5)", "1.5", false, true},
		{Maven, "[2.0,)", "2.0", true, true},
		{Maven, "(2.0,)", "2.0", false, true},
		{Maven, "[1.0]", "1.0.0", true, true},
		{Maven, "[1,2),[3,4)", "3.5", true, true},
		{Maven, "[1,2),[3,4)", "2.5", false, true},
		{Maven, "1.0", "0.1", true, true}, // a bare version only recommends
		{Maven, "[2.0,)", "1.20.1-2.3.0", false, false},
		{Maven, "[2.0,", "2.1", false, false},
	}
	for _, c := range cases {
		in, ok := Satisfies(c.st, c.r, c.v)
		if in != c.in || ok != c.ok {
			t.Errorf("Satisfies(%v, %q, %q) = %v, %v; want %v, %v", c.st, c.r, c.v, in, ok, c.in, c.ok)
		}
	}
}

func TestDescribe(t *testing.T) {
	cases := []struct {
		st   Style
		r    string
		want string
	}{
		{Fabric, ">=0.5.0", "≥ 0.5.0"},
		{Fabric, ">=0.5 <0.6", "≥ 0.5 < 0.6"},
		{Fabric, "~1.20", "1.20.x"},
		{Fabric, "~1.20.1", "1.20.x"},
		{Fabric, "1.19.2||1.20.1", "1.19.2 or 1.20.1"},
		{Fabric, "*", ""},
		{Maven, "[1.20.1,1.21)", "≥ 1.20.1, < 1.21"},
		{Maven, "[47,)", "≥ 47"},
		{Maven, "[1.0]", "1.0"},
		{Maven, "1.0", ""},
	}
	for _, c := range cases {
		if got := Describe(c.st, c.r); got != c.want {
			t.Errorf("Describe(%v, %q) = %q, want %q", c.st, c.r, got, c.want)
		}
	}
}
