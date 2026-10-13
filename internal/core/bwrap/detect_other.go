//go:build !linux

package bwrap

func detect() Support { return Support{Reason: "bubblewrap is Linux-only."} }
