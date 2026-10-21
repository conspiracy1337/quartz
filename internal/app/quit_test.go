package app

import "testing"

// Closing asks first about a running game, then a hosted server, then
// unfinished downloads; with none of them it closes.
func TestCloseBlocker(t *testing.T) {
	for _, c := range []struct {
		games, servers, downloads int
		subject, text             string
		busy                      bool
	}{
		{0, 0, 0, "", "", false},
		{1, 1, 3, "1 game", "is still running", true},
		{2, 0, 0, "2 games", "are still running", true},
		{0, 1, 2, "Your server", "is still running", true},
		{0, 0, 1, "1 download", "isn't finished yet", true},
		{0, 0, 4, "4 downloads", "aren't finished yet", true},
	} {
		subject, text, busy := closeBlocker(c.games, c.servers, c.downloads)
		if subject != c.subject || text != c.text || busy != c.busy {
			t.Errorf("closeBlocker(%d, %d, %d) = %q, %q, %v; want %q, %q, %v",
				c.games, c.servers, c.downloads, subject, text, busy, c.subject, c.text, c.busy)
		}
	}
}
