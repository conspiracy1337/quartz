package procx

import "errors"

// ErrDeclined means the system's administrator prompt was dismissed, or the
// account can't approve it.
var ErrDeclined = errors.New("procx: administrator approval was declined")
