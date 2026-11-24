//go:build windows

package cmd

import (
	"io"
	"os"

	"golang.org/x/sys/windows"
)

func isInteractiveTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok || f == nil {
		return false
	}

	var mode uint32
	err := windows.GetConsoleMode(windows.Handle(f.Fd()), &mode)
	return err == nil
}
