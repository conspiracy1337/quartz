package instance

import (
	"path/filepath"
	"testing"
)

// The command-settings origin gate: commands signed by this install's key (as
// SetSettings stamps them) are honoured; anything else — unsigned, signed for
// another instance id, or signed by a different install — is dropped before it
// can run.
func TestCommandOriginGate(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "instances"), nil)
	if err != nil {
		t.Fatal(err)
	}
	cmds := func() CommandSettings {
		return CommandSettings{PreLaunch: "echo hi", Wrapper: "prime-run", PostExit: "cleanup"}
	}
	withCmds := func(id string, c CommandSettings) Instance {
		return Instance{ID: id, Settings: Settings{Commands: c}}
	}
	live := func(c CommandSettings) bool { return c.PreLaunch != "" || c.Wrapper != "" || c.PostExit != "" }

	// Correctly signed for this id → kept.
	c := cmds()
	c.Sig = s.signCommands("inst1", c)
	if got := s.verified(withCmds("inst1", c)).Settings.Commands; !live(got) {
		t.Error("commands set in this launcher were dropped")
	}

	// Same signed blob reused under a different id → dropped (id is bound in).
	if got := s.verified(withCmds("inst2", c)).Settings.Commands; live(got) {
		t.Error("commands signed for another instance id were honoured")
	}

	// Commands present with no signature (hand-added) → dropped.
	if got := s.verified(withCmds("inst1", cmds())).Settings.Commands; live(got) {
		t.Error("an unsigned command was honoured")
	}

	// Signed by a different install (different key) → dropped.
	other, err := Open(filepath.Join(t.TempDir(), "instances"), nil)
	if err != nil {
		t.Fatal(err)
	}
	f := cmds()
	f.Sig = other.signCommands("inst1", f)
	if got := s.verified(withCmds("inst1", f)).Settings.Commands; live(got) {
		t.Error("commands signed by another install were honoured")
	}

	// A tampered command with an otherwise-valid-looking sig → dropped.
	tampered := withCmds("inst1", c) // c.Sig is for cmds(); now change a command
	tampered.Settings.Commands.Wrapper = "evil.exe"
	if got := s.verified(tampered).Settings.Commands; got.Wrapper == "evil.exe" {
		t.Error("a tampered command slipped past the signature")
	}

	// No commands at all → untouched (no false positives, no key churn).
	if got := s.verified(Instance{ID: "inst1"}).Settings.Commands; live(got) {
		t.Error("empty commands were somehow populated")
	}
}
