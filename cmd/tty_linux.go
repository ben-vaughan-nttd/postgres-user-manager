//go:build linux

package cmd

import (
	"io"
	"math"
	"os"

	"golang.org/x/sys/unix"
)

func isInteractiveTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok || f == nil {
		return false
	}

	fd := f.Fd()
	if fd > math.MaxInt {
		return false
	}

	_, err := unix.IoctlGetTermios(int(fd), unix.TCGETS)
	return err == nil
}
