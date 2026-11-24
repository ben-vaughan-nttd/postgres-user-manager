//go:build !linux && !darwin && !dragonfly && !freebsd && !netbsd && !openbsd && !windows

package cmd

import "io"

func isInteractiveTerminal(_ io.Reader) bool {
	return false
}
