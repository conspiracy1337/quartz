package wintoast

import (
	"strings"
	"testing"
)

func TestPowershellScriptCarriesNoCode(t *testing.T) {
	hostile := "'@\n$(Remove-Item C:\\)\n\"@ ' `"
	s := powershellScript(hostile, hostile)
	if strings.Contains(s, "Remove-Item") || strings.Contains(s, "$(") || strings.Count(s, "'") != 4 {
		t.Fatalf("notification text reached the script as code:\n%s", s)
	}
	if powershellScript("", "<toast/>") == powershellScript("App", "<toast/>") {
		t.Fatal("empty app ID not defaulted")
	}
}
