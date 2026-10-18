package markdown

import (
	"encoding/json"
	"os"
	"testing"
)

// specExample is one example of the CommonMark spec (testdata/spec.json,
// CommonMark 0.31.2 as goldmark ships it).
type specExample struct {
	Markdown string `json:"markdown"`
	HTML     string `json:"html"`
	Example  int    `json:"example"`
	Section  string `json:"section"`
}

func loadSpec(t testing.TB) []specExample {
	t.Helper()
	b, err := os.ReadFile("testdata/spec.json")
	if err != nil {
		t.Fatal(err)
	}
	var ex []specExample
	if err := json.Unmarshal(b, &ex); err != nil {
		t.Fatal(err)
	}
	return ex
}

func TestSpec(t *testing.T) {
	for _, ex := range loadSpec(t) {
		if got := commonMark(ex.Markdown); got != ex.HTML {
			t.Errorf("example %d (%s)\n--- markdown\n%q\n--- want\n%q\n--- got\n%q", ex.Example, ex.Section, ex.Markdown, ex.HTML, got)
		}
	}
}
