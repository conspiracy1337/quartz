package procx

// Same reports whether pid still is the process Ident described: alive and
// started at the same moment, so a reused pid never counts as the game the
// launcher started. Ident 0 (the OS couldn't tell) trusts the pid alone, as
// does a live process whose start can't be read now.
func Same(pid int, ident uint64) bool {
	if !Alive(pid) {
		return false
	}
	if ident == 0 {
		return true
	}
	now, ok := Ident(pid)
	return !ok || now == ident
}
