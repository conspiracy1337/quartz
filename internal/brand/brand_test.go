package brand

import "testing"

func TestClientID(t *testing.T) {
	MSAClientID = ""
	t.Setenv(EnvMSAClientID, "")
	if got := ClientID(); got != "" {
		t.Fatalf("unset: %q", got)
	}
	MSAClientID = "11111111-2222-3333-4444-555555555555"
	if got := ClientID(); got != MSAClientID {
		t.Fatalf("build value: %q", got)
	}
	t.Setenv(EnvMSAClientID, "AAAAAAAA-2222-3333-4444-555555555555")
	if got := ClientID(); got != "aaaaaaaa-2222-3333-4444-555555555555" {
		t.Fatalf("env override: %q", got)
	}
	t.Setenv(EnvMSAClientID, "not-a-guid&x=1")
	if got := ClientID(); got != MSAClientID {
		t.Fatalf("malformed env must be ignored: %q", got)
	}
	MSAClientID = ""
}

func TestCurseForgeKey(t *testing.T) {
	CurseForgeAPIKey = ""
	t.Setenv(EnvCurseForgeKey, "")
	if got := CurseForgeKey(); got != "" {
		t.Fatalf("unset: %q", got)
	}
	CurseForgeAPIKey = "$2a$10$abcdefghijklmnopqrstuv"
	if got := CurseForgeKey(); got != CurseForgeAPIKey {
		t.Fatalf("build value: %q", got)
	}
	t.Setenv(EnvCurseForgeKey, "$2a$10$fromtheenvironmentvalue")
	if got := CurseForgeKey(); got != "$2a$10$fromtheenvironmentvalue" {
		t.Fatalf("env override: %q", got)
	}
	for _, bad := range []string{"short", "has a space in the middle of it", "line\nbreak-injected-header-value"} {
		t.Setenv(EnvCurseForgeKey, bad)
		if got := CurseForgeKey(); got != CurseForgeAPIKey {
			t.Fatalf("malformed %q must be ignored: %q", bad, got)
		}
	}
	CurseForgeAPIKey = ""
}

func TestValidators(t *testing.T) {
	for _, c := range []struct {
		valid func(string) bool
		good  string
		bad   []string
	}{
		{ValidClientID, "11111111-2222-3333-4444-555555555555", []string{"", "11111111-2222-3333-4444-55555555555", "x=1 11111111-2222-3333-4444-555555555555"}},
		{ValidCurseForgeKey, "$2a$10$abcdefghijklmnopqrstuv", []string{"", "short", "has a space in the middle of it"}},
		{ValidDiscordID, "123456789012345678", []string{"", "1234", "12345678901234567a", "123456789012345678 -X main.x=1"}},
	} {
		if !c.valid(c.good) {
			t.Errorf("%q should be valid", c.good)
		}
		for _, b := range c.bad {
			if c.valid(b) {
				t.Errorf("%q should be invalid", b)
			}
		}
	}
}
