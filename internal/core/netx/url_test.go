package netx

import "testing"

func TestSafeHTTPS(t *testing.T) {
	ok := []string{"https://www.minecraft.net/en-us/articles", "https://modrinth.com/mod/sodium"}
	bad := []string{"http://x.com", "file:///etc/passwd", "javascript:alert(1)", "https://", "https://u:p@x.com", "modrinth://x", ""}
	for _, u := range ok {
		if _, safe := SafeHTTPS(u); !safe {
			t.Errorf("%q rejected", u)
		}
	}
	for _, u := range bad {
		if _, safe := SafeHTTPS(u); safe {
			t.Errorf("%q accepted", u)
		}
	}
}
