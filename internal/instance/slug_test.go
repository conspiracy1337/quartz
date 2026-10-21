package instance

import (
	"strings"
	"testing"
)

func TestIDSlug(t *testing.T) {
	for name, want := range map[string]string{
		"Fabulously Optimized 6.2":   "fabulously-optimized-6-2",
		"Ünïcødé 日本語 🎮":              "unicode",
		"Straße nach Łódź":           "strasse-nach-lodz",
		"Æsir Œuvre Þór":             "aesir-oeuvre-thor",
		"日本語":                        "instance",
		"🎮🎮":                         "instance",
		`a:b?c*<>|"`:                 "a-b-c",
		"CON":                        "con",
		"Trailing dot. ":             "trailing-dot",
		strings.Repeat("abcde ", 20): "abcde-abcde-abcde-abcde-abcde-abcde-abcd", // cut at 40
	} {
		if got := idSlug(name); got != want {
			t.Errorf("idSlug(%q) = %q, want %q", name, got, want)
		}
	}
}
