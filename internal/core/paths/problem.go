package paths

import (
	"os"
	"path/filepath"
	"strings"

	"quartz/internal/core/fsx"
)

// Problems with where the data folder is.
const (
	// ProblemBang: a "!" in the path breaks Java, which reads "!" as the
	// end of a jar's name in its URLs (Prism warns about the same).
	ProblemBang = "bang"
	// ProblemTemp: the system may empty a temporary folder at any time, and
	// a launcher run from inside an archive keeps its data there.
	ProblemTemp = "temp"
)

// tempDir is the system's temporary folder (a variable for tests, which
// themselves run in it).
var tempDir = os.TempDir

// Problem names what is wrong with root as the data folder, or "".
func Problem(root string) string {
	switch {
	case strings.Contains(root, "!"):
		return ProblemBang
	case inTemp(root):
		return ProblemTemp
	}
	return ""
}

func inTemp(root string) bool {
	if t := tempDir(); t != "" && fsx.Within(filepath.Clean(root), filepath.Clean(t)) {
		return true
	}
	// Archives opened in Explorer or WinRAR, and app-container temp folders.
	lower := strings.ToLower(filepath.ToSlash(root))
	return strings.Contains(lower, "rar$") || strings.Contains(lower, "/tempstate/")
}
