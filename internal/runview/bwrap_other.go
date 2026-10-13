//go:build !linux

package runview

// bwrapSupport: mount namespaces are Linux only.
func bwrapSupport() support { return support{} }
