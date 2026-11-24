//go:build linux

package cmd

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

func openTestPTY(t *testing.T) (*os.File, func()) {
	t.Helper()

	masterFd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		if errors.Is(err, unix.EACCES) || errors.Is(err, unix.ENOENT) || errors.Is(err, unix.EPERM) {
			t.Skipf("skipping PTY test: cannot open /dev/ptmx: %v", err)
		}
		t.Fatalf("failed to open /dev/ptmx: %v", err)
	}
	masterFile := os.NewFile(uintptr(masterFd), "/dev/ptmx")

	if err := unix.IoctlSetPointerInt(masterFd, unix.TIOCSPTLCK, 0); err != nil {
		_ = masterFile.Close()
		t.Fatalf("TIOCSPTLCK failed: %v", err)
	}

	ptsNum, err := unix.IoctlGetInt(masterFd, unix.TIOCGPTN)
	if err != nil {
		_ = masterFile.Close()
		t.Fatalf("TIOCGPTN failed: %v", err)
	}

	slavePath := fmt.Sprintf("/dev/pts/%d", ptsNum)
	slaveFd, err := unix.Open(slavePath, unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		_ = masterFile.Close()
		if errors.Is(err, unix.EACCES) || errors.Is(err, unix.ENOENT) || errors.Is(err, unix.EPERM) {
			t.Skipf("skipping PTY test: cannot open %s: %v", slavePath, err)
		}
		t.Fatalf("failed to open slave PTY %s: %v", slavePath, err)
	}
	slaveFile := os.NewFile(uintptr(slaveFd), slavePath)

	cleanup := func() {
		_ = slaveFile.Close()
		_ = masterFile.Close()
	}
	return slaveFile, cleanup
}
