package instance

// ImportedFrom names the instance of another launcher an instance was
// brought over from, so importing again can tell it is already here.
type ImportedFrom struct {
	Launcher string `json:"launcher"`
	Path     string `json:"path"`
}
