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
