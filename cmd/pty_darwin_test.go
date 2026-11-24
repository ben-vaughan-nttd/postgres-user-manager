//go:build darwin

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

	if err := unix.IoctlSetInt(masterFd, unix.TIOCPTYGRANT, 0); err != nil {
		_ = masterFile.Close()
		t.Fatalf("TIOCPTYGRANT failed: %v", err)
	}
	if err := unix.IoctlSetInt(masterFd, unix.TIOCPTYUNLK, 0); err != nil {
		_ = masterFile.Close()
		t.Fatalf("TIOCPTYUNLK failed: %v", err)
	}

	var st unix.Stat_t
	if err := unix.Fstat(masterFd, &st); err != nil {
		_ = masterFile.Close()
		t.Fatalf("fstat(/dev/ptmx) failed: %v", err)
	}

	// On Darwin, the master PTY's minor device number identifies /dev/ttysNNN.
	slavePath := fmt.Sprintf("/dev/ttys%03d", unix.Minor(uint64(st.Rdev)))
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
