//go:build darwin || linux

package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

func TestIsInteractiveTerminalRealPTY(t *testing.T) {
	ttyFile, cleanup := openTestPTY(t)
	defer cleanup()

	if !isInteractiveTerminal(ttyFile) {
		t.Fatalf("expected isInteractiveTerminal(%s) == true, got false", ttyFile.Name())
	}

	regularFile, err := os.CreateTemp(t.TempDir(), "non-tty-*")
	if err != nil {
		t.Fatalf("os.CreateTemp failed: %v", err)
	}
	defer func() { _ = regularFile.Close() }()

	if isInteractiveTerminal(regularFile) {
		t.Fatalf("expected isInteractiveTerminal(%s) == false, got true", regularFile.Name())
	}

	t.Setenv("POSTGRES_PASSWORD", "")
	var stdout, stderr, logBuf bytes.Buffer
	deps := defaultCLIDeps()
	deps.logOut = &logBuf

	root := newRootCommand(deps)
	root.SetArgs([]string{"create-user", "myuser", "--password-stdin"})
	root.SetIn(ttyFile)
	root.SetOut(&stdout)
	root.SetErr(&stderr)

	errCh := make(chan error, 1)
	go func() {
		errCh <- root.Execute()
	}()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected interactive terminal error for real PTY stdin, got nil")
		}
		if !strings.Contains(err.Error(), "piped or redirected") {
			t.Fatalf("error = %q, want message explaining piped or redirected stdin is required", err.Error())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("create-user --password-stdin blocked on real PTY stdin instead of failing immediately")
	}
}
