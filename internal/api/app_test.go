package api

import "testing"

func TestCheckHTTPS(t *testing.T) {
	ok := []string{"https://www.minecraft.net/en-us/articles", "https://modrinth.com/mod/sodium"}
	bad := []string{"http://x.com", "file:///etc/passwd", "javascript:alert(1)", "https://", "https://u:p@x.com", "modrinth://x", ""}
	for _, u := range ok {
		if checkHTTPS(u) != nil {
			t.Errorf("%q rejected", u)
		}
	}
	for _, u := range bad {
		if checkHTTPS(u) == nil {
			t.Errorf("%q accepted", u)
		}
	}
}
