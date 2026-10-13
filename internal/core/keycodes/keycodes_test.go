package keycodes

import "testing"

func TestRoundTrips(t *testing.T) {
	for name, k := range byName {
		if k.Legacy != 0 {
			back, ok := ByLegacy(k.Legacy)
			if !ok || back.Name != name {
				t.Errorf("%s: legacy %d maps to %q", name, k.Legacy, back.Name)
			}
		}
		if k.Browser != "" {
			back, ok := ByBrowser(k.Browser)
			if !ok || back.Name != name {
				t.Errorf("%s: browser %q maps to %q", name, k.Browser, back.Name)
			}
		}
		if k.Label == "" {
			t.Errorf("%s has no label", name)
		}
	}
}

func TestKnownValues(t *testing.T) {
	for _, c := range []struct {
		in     string
		name   string
		legacy string
	}{
		{"key.keyboard.left.control", "key.keyboard.left.control", "29"},
		{"29", "key.keyboard.left.control", "29"},
		{"-100", "key.mouse.left", "-100"},
		{"key.mouse.right", "key.mouse.right", "-99"},
		{"key.mouse.4", "key.mouse.4", "-97"},
		{"17", "key.keyboard.w", "17"},
		{"key.keyboard.f5", "key.keyboard.f5", "63"},
		{"0", Unknown, "0"},
		{"key.keyboard.keypad.0", "key.keyboard.keypad.0", "82"},
	} {
		k, ok := Parse(c.in)
		if !ok || k.Name != c.name {
			t.Fatalf("Parse(%q) = %q %v", c.in, k.Name, ok)
		}
		if got, ok := Format(k, true); !ok || got != c.legacy {
			t.Fatalf("Format(%q, legacy) = %q %v", c.name, got, ok)
		}
	}
	if _, ok := Format(byName["key.keyboard.world.1"], true); ok {
		t.Fatal("world.1 has no LWJGL2 code")
	}
	if _, ok := Parse("key.keyboard.nope"); ok {
		t.Fatal("unknown name parsed")
	}
	if k, _ := ByBrowser("Mouse1"); k.Name != "key.mouse.middle" {
		t.Fatal(k.Name)
	}
}
