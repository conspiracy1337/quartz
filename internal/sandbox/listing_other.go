//go:build !windows

package sandbox

// Main runs the Windows administrator helper; there is none here.
func Main() {}

// ApproveListing is Windows' one-time approval; nothing needs it here.
func (s *Service) ApproveListing() error { return nil }
