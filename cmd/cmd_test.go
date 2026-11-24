package cmd

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ben-vaughan-nttd/postgres-user-manager/internal/structs"
	_ "github.com/lib/pq"
	"github.com/sirupsen/logrus"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

type fakeUserManager struct {
	createdUsers []*structs.UserConfig
	createErr    error
	closed       bool
}

func (f *fakeUserManager) Close() error {
	f.closed = true
	return nil
}

func (f *fakeUserManager) CreateUser(user *structs.UserConfig) error {
	if f.createErr != nil {
		return f.createErr
	}
	copied := *user
	f.createdUsers = append(f.createdUsers, &copied)
	return nil
}

func (f *fakeUserManager) AddUserToGroup(_, _ string) error {
	return nil
}

func (f *fakeUserManager) GrantPrivileges(_ string, _, _ []string) error {
	return nil
}

func (f *fakeUserManager) DropUser(_ string) error {
	return nil
}

func (f *fakeUserManager) SyncConfiguration(_ *structs.Config) (*structs.SyncResult, error) {
	return &structs.SyncResult{}, nil
}

type failOnReadReader struct {
	called bool
	t      *testing.T
}

func (r *failOnReadReader) Read(_ []byte) (int, error) {
	r.called = true
	if r.t != nil {
		r.t.Errorf("Read should not have been called")
	}
	return 0, errors.New("unexpected Read invocation")
}

type partialThenErrReader struct {
	payload      []byte
	err          error
	combineCalls bool
	sent         bool
}

func (r *partialThenErrReader) Read(p []byte) (int, error) {
	if r.sent {
		return 0, r.err
	}
	r.sent = true
	n := copy(p, r.payload)
	if r.combineCalls {
		return n, r.err
	}
	return n, nil
}

type countingRepeatReader struct {
	b    byte
	read int64
}

func (r *countingRepeatReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = r.b
	}
	r.read += int64(len(p))
	return len(p), nil
}

func assertNoLeak(t *testing.T, forbidden []string, outputs ...string) {
	t.Helper()
	for _, out := range outputs {
		for _, secret := range forbidden {
			if secret != "" && strings.Contains(out, secret) {
				t.Fatalf("output leaked forbidden secret substring %q:\n%s", secret, out)
			}
		}
	}
}

func TestCreateUserRejectsRemovedPasswordFlags(t *testing.T) {
	sentinel := "Sup3rS3cr3tFlagVal99"
	forbidden := []string{sentinel, "Sup3rS3cr3t", "FlagVal99"}

	tests := []struct {
		name        string
		args        []string
		wantErrText string
	}{
		{
			name:        "long --password with separate arg",
			args:        []string{"create-user", "myuser", "--password", sentinel},
			wantErrText: "unknown flag: --password",
		},
		{
			name:        "long --password with equals",
			args:        []string{"create-user", "myuser", "--password=" + sentinel},
			wantErrText: "unknown flag: --password",
		},
		{
			name:        "short -p with separate arg",
			args:        []string{"create-user", "myuser", "-p", sentinel},
			wantErrText: "unknown shorthand flag: 'p'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("POSTGRES_PASSWORD", "")
			var stdout, stderr, logBuf bytes.Buffer
			managerCalled := false

			root := newRootCommand(cliDeps{
				isTerminal: func(io.Reader) bool { return false },
				newManager: func(_ *structs.DatabaseConnection, _ *logrus.Logger, _ bool) (userManager, error) {
					managerCalled = true
					return &fakeUserManager{}, nil
				},
				logOut: &logBuf,
			})
			root.SetArgs(tt.args)
			root.SetOut(&stdout)
			root.SetErr(&stderr)

			err := root.Execute()
			if err == nil {
				t.Fatalf("expected error for args %v, got nil", tt.args)
			}
			if managerCalled {
				t.Fatal("database manager should not be created on unknown flag")
			}
			if !strings.Contains(err.Error(), tt.wantErrText) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantErrText)
			}
			assertNoLeak(t, forbidden, err.Error(), stdout.String(), stderr.String(), logBuf.String())
		})
	}
}

func TestReadPasswordFromStdinValidInputs(t *testing.T) {
	maxLenPassword := strings.Repeat("a", maxPasswordBytes)

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "LF terminated",
			input: "simple-password\n",
			want:  "simple-password",
		},
		{
			name:  "CRLF terminated",
			input: "simple-password\r\n",
			want:  "simple-password",
		},
		{
			name:  "EOF without trailing newline",
			input: "simple-password",
			want:  "simple-password",
		},
		{
			name:  "leading and trailing spaces and tabs",
			input: "  \tS3cr'et\"Pass\\!@#\t  \n",
			want:  "  \tS3cr'et\"Pass\\!@#\t  ",
		},
		{
			name:  "quotes backslashes and punctuation with CRLF",
			input: "  S3cr'et\"Pass\\!@#$%^&*()_+-=[]{}|;:,.<>?/~`\t \r\n",
			want:  "  S3cr'et\"Pass\\!@#$%^&*()_+-=[]{}|;:,.<>?/~`\t ",
		},
		{
			name:  "double CR before LF preserves first CR",
			input: "abc\r\r\n",
			want:  "abc\r",
		},
		{
			name:  "trailing CR without LF is preserved",
			input: "abc\r",
			want:  "abc\r",
		},
		{
			name:  "whitespace-only password with LF",
			input: "   \n",
			want:  "   ",
		},
		{
			name:  "tab-only password with CRLF",
			input: "\t\r\n",
			want:  "\t",
		},
		{
			name:  "unicode characters preserved",
			input: "p@ssw\u00f6rd-\u5bc6\u7801-\U0001f511\n",
			want:  "p@ssw\u00f6rd-\u5bc6\u7801-\U0001f511",
		},
		{
			name:  "exact max length without trailing newline",
			input: maxLenPassword,
			want:  maxLenPassword,
		},
		{
			name:  "exact max length with LF",
			input: maxLenPassword + "\n",
			want:  maxLenPassword,
		},
		{
			name:  "exact max length with CRLF",
			input: maxLenPassword + "\r\n",
			want:  maxLenPassword,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readPasswordFromStdin(strings.NewReader(tt.input), func(io.Reader) bool { return false })
			if err != nil {
				t.Fatalf("readPasswordFromStdin() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("readPasswordFromStdin() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCreateUserPasswordStdinPipedAndOptionality(t *testing.T) {
	t.Run("piped password passed verbatim to manager", func(t *testing.T) {
		t.Setenv("POSTGRES_PASSWORD", "admin-test-pass")
		wantPassword := "  S3cr'et\"Pass\\!99\t "

		fakeMgr := &fakeUserManager{}
		var stdout, stderr, logBuf bytes.Buffer

		root := newRootCommand(cliDeps{
			isTerminal: func(io.Reader) bool { return false },
			newManager: func(_ *structs.DatabaseConnection, _ *logrus.Logger, _ bool) (userManager, error) {
				return fakeMgr, nil
			},
			logOut: &logBuf,
		})
		root.SetArgs([]string{"create-user", "myuser", "--password-stdin", "--verbose"})
		root.SetIn(strings.NewReader(wantPassword + "\r\n"))
		root.SetOut(&stdout)
		root.SetErr(&stderr)

		if err := root.Execute(); err != nil {
			t.Fatalf("Execute() unexpected error: %v", err)
		}
		if len(fakeMgr.createdUsers) != 1 {
			t.Fatalf("expected 1 created user, got %d", len(fakeMgr.createdUsers))
		}
		if fakeMgr.createdUsers[0].Password != wantPassword {
			t.Fatalf("created password = %q, want %q", fakeMgr.createdUsers[0].Password, wantPassword)
		}
		assertNoLeak(
			t,
			[]string{wantPassword, strings.TrimSpace(wantPassword), "S3cr'et", "S3cr", "Pass\\!99"},
			stdout.String(),
			stderr.String(),
			logBuf.String(),
		)
	})

	t.Run("omitting --password-stdin leaves password empty without reading stdin", func(t *testing.T) {
		t.Setenv("POSTGRES_PASSWORD", "admin-test-pass")

		fakeMgr := &fakeUserManager{}
		reader := &failOnReadReader{t: t}
		var stdout, stderr, logBuf bytes.Buffer

		root := newRootCommand(cliDeps{
			isTerminal: func(io.Reader) bool { return true }, // even on a TTY, omitting --password-stdin must succeed
			newManager: func(_ *structs.DatabaseConnection, _ *logrus.Logger, _ bool) (userManager, error) {
				return fakeMgr, nil
			},
			logOut: &logBuf,
		})
		root.SetArgs([]string{"create-user", "myuser"})
		root.SetIn(reader)
		root.SetOut(&stdout)
		root.SetErr(&stderr)

		if err := root.Execute(); err != nil {
			t.Fatalf("Execute() unexpected error: %v", err)
		}
		if reader.called {
			t.Fatal("stdin should not be read when --password-stdin is omitted")
		}
		if len(fakeMgr.createdUsers) != 1 {
			t.Fatalf("expected 1 created user, got %d", len(fakeMgr.createdUsers))
		}
		if fakeMgr.createdUsers[0].Password != "" {
			t.Fatalf("expected empty password, got %q", fakeMgr.createdUsers[0].Password)
		}
	})

	t.Run("real os.Pipe is not classified as TTY", func(t *testing.T) {
		pr, pw, err := os.Pipe()
		if err != nil {
			t.Fatalf("os.Pipe() failed: %v", err)
		}
		defer func() { _ = pr.Close() }()

		if isInteractiveTerminal(pr) {
			t.Fatal("os.Pipe read end must not be classified as interactive terminal")
		}

		want := "pipe-secret-123"
		go func() {
			_, _ = io.WriteString(pw, want+"\n")
			_ = pw.Close()
		}()

		got, err := readPasswordFromStdin(pr, isInteractiveTerminal)
		if err != nil {
			t.Fatalf("readPasswordFromStdin(pipe) failed: %v", err)
		}
		if got != want {
			t.Fatalf("readPasswordFromStdin(pipe) = %q, want %q", got, want)
		}
	})
}

func TestCreateUserPasswordStdinEmptyAndDevNull(t *testing.T) {
	emptyCases := []struct {
		name  string
		input string
	}{
		{name: "0-byte EOF", input: ""},
		{name: "LF only", input: "\n"},
		{name: "CRLF only", input: "\r\n"},
	}

	for _, tc := range emptyCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("POSTGRES_PASSWORD", "")
			managerCalled := false
			var stdout, stderr, logBuf bytes.Buffer

			root := newRootCommand(cliDeps{
				isTerminal: func(io.Reader) bool { return false },
				newManager: func(_ *structs.DatabaseConnection, _ *logrus.Logger, _ bool) (userManager, error) {
					managerCalled = true
					return &fakeUserManager{}, nil
				},
				logOut: &logBuf,
			})
			root.SetArgs([]string{"create-user", "myuser", "--password-stdin"})
			root.SetIn(strings.NewReader(tc.input))
			root.SetOut(&stdout)
			root.SetErr(&stderr)

			err := root.Execute()
			if err == nil {
				t.Fatalf("expected error for empty input %q, got nil", tc.input)
			}
			if managerCalled {
				t.Fatal("database manager should not be created on empty stdin")
			}
			if !strings.Contains(err.Error(), "empty") {
				t.Fatalf("error = %q, want substring %q", err.Error(), "empty")
			}
		})
	}

	t.Run("real devnull is not a TTY and fails with empty password", func(t *testing.T) {
		if runtime.GOOS != "windows" && os.DevNull != "/dev/null" {
			t.Fatalf("expected os.DevNull to be /dev/null on %s, got %q", runtime.GOOS, os.DevNull)
		}
		f, err := os.Open(os.DevNull)
		if err != nil {
			t.Fatalf("os.Open(%q) failed: %v", os.DevNull, err)
		}
		defer func() { _ = f.Close() }()

		if isInteractiveTerminal(f) {
			t.Fatalf("%s must not be classified as an interactive terminal", os.DevNull)
		}

		t.Setenv("POSTGRES_PASSWORD", "")
		managerCalled := false
		var stdout, stderr, logBuf bytes.Buffer

		root := newRootCommand(cliDeps{
			isTerminal: isInteractiveTerminal,
			newManager: func(_ *structs.DatabaseConnection, _ *logrus.Logger, _ bool) (userManager, error) {
				managerCalled = true
				return &fakeUserManager{}, nil
			},
			logOut: &logBuf,
		})
		root.SetArgs([]string{"create-user", "myuser", "--password-stdin"})
		root.SetIn(f)
		root.SetOut(&stdout)
		root.SetErr(&stderr)

		err = root.Execute()
		if err == nil {
			t.Fatal("expected empty-password error for /dev/null, got nil")
		}
		if managerCalled {
			t.Fatal("database manager should not be created for /dev/null input")
		}
		if !strings.Contains(err.Error(), "empty") {
			t.Fatalf("error = %q, want empty-input error (not TTY error)", err.Error())
		}
	})
}

func TestCreateUserPasswordStdinInteractiveTTYDoesNotRead(t *testing.T) {
	t.Setenv("POSTGRES_PASSWORD", "")
	reader := &failOnReadReader{t: t}
	managerCalled := false
	var stdout, stderr, logBuf bytes.Buffer

	root := newRootCommand(cliDeps{
		isTerminal: func(io.Reader) bool { return true },
		newManager: func(_ *structs.DatabaseConnection, _ *logrus.Logger, _ bool) (userManager, error) {
			managerCalled = true
			return &fakeUserManager{}, nil
		},
		logOut: &logBuf,
	})
	root.SetArgs([]string{"create-user", "myuser", "--password-stdin"})
	root.SetIn(reader)
	root.SetOut(&stdout)
	root.SetErr(&stderr)

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for interactive TTY stdin, got nil")
	}
	if reader.called {
		t.Fatal("Read must not be invoked when stdin is an interactive TTY")
	}
	if managerCalled {
		t.Fatal("database manager must not be created when stdin is an interactive TTY")
	}
	if !strings.Contains(err.Error(), "piped or redirected") {
		t.Fatalf("error = %q, want message explaining piped or redirected stdin is required", err.Error())
	}
}

func TestCreateUserPasswordStdinMultiLineRejectedWithoutEcho(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		forbidden []string
	}{
		{
			name:      "two LF lines",
			input:     "first_secret\nsecond_secret\n",
			forbidden: []string{"first_secret", "second_secret", "first", "second"},
		},
		{
			name:      "CRLF then second line",
			input:     "first_secret\r\nsecond_secret",
			forbidden: []string{"first_secret", "second_secret", "first", "second"},
		},
		{
			name:      "trailing blank line",
			input:     "first_secret\n\n",
			forbidden: []string{"first_secret", "first"},
		},
		{
			name:      "max length first line with second line",
			input:     strings.Repeat("a", maxPasswordBytes) + "\nsecond_secret",
			forbidden: []string{"second_secret", "second"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("POSTGRES_PASSWORD", "")
			managerCalled := false
			var stdout, stderr, logBuf bytes.Buffer

			root := newRootCommand(cliDeps{
				isTerminal: func(io.Reader) bool { return false },
				newManager: func(_ *structs.DatabaseConnection, _ *logrus.Logger, _ bool) (userManager, error) {
					managerCalled = true
					return &fakeUserManager{}, nil
				},
				logOut: &logBuf,
			})
			root.SetArgs([]string{"create-user", "myuser", "--password-stdin", "--verbose"})
			root.SetIn(strings.NewReader(tt.input))
			root.SetOut(&stdout)
			root.SetErr(&stderr)

			err := root.Execute()
			if err == nil {
				t.Fatalf("expected multi-line rejection error for %q, got nil", tt.name)
			}
			if managerCalled {
				t.Fatal("database manager should not be created on multi-line stdin")
			}
			if !strings.Contains(err.Error(), "single password line") {
				t.Fatalf("error = %q, want single-line message", err.Error())
			}
			assertNoLeak(t, tt.forbidden, err.Error(), stdout.String(), stderr.String(), logBuf.String())
		})
	}
}

func TestCreateUserPasswordStdinNULOversizedAndReadErrors(t *testing.T) {
	t.Run("NUL byte rejected without echo", func(t *testing.T) {
		t.Setenv("POSTGRES_PASSWORD", "")
		managerCalled := false
		var stdout, stderr, logBuf bytes.Buffer

		root := newRootCommand(cliDeps{
			isTerminal: func(io.Reader) bool { return false },
			newManager: func(_ *structs.DatabaseConnection, _ *logrus.Logger, _ bool) (userManager, error) {
				managerCalled = true
				return &fakeUserManager{}, nil
			},
			logOut: &logBuf,
		})
		root.SetArgs([]string{"create-user", "myuser", "--password-stdin", "--verbose"})
		root.SetIn(strings.NewReader("sec_SENTINEL_NUL\x00ret_TAIL_99\n"))
		root.SetOut(&stdout)
		root.SetErr(&stderr)

		err := root.Execute()
		if err == nil {
			t.Fatal("expected NUL-byte rejection error, got nil")
		}
		if managerCalled {
			t.Fatal("database manager should not be created on NUL byte input")
		}
		if !strings.Contains(err.Error(), "NUL") {
			t.Fatalf("error = %q, want NUL message", err.Error())
		}
		assertNoLeak(
			t,
			[]string{"sec_SENTINEL_NUL", "ret_TAIL_99", "SENTINEL_NUL", "TAIL_99"},
			err.Error(),
			stdout.String(),
			stderr.String(),
			logBuf.String(),
		)
	})

	t.Run("oversized input rejected and bounded without echo", func(t *testing.T) {
		t.Setenv("POSTGRES_PASSWORD", "")
		sentinel := "OVERSIZED_SECRET_PREFIX_77"
		oversized := sentinel + strings.Repeat("Z", maxPasswordBytes)
		managerCalled := false
		var stdout, stderr, logBuf bytes.Buffer

		root := newRootCommand(cliDeps{
			isTerminal: func(io.Reader) bool { return false },
			newManager: func(_ *structs.DatabaseConnection, _ *logrus.Logger, _ bool) (userManager, error) {
				managerCalled = true
				return &fakeUserManager{}, nil
			},
			logOut: &logBuf,
		})
		root.SetArgs([]string{"create-user", "myuser", "--password-stdin", "--verbose"})
		root.SetIn(strings.NewReader(oversized))
		root.SetOut(&stdout)
		root.SetErr(&stderr)

		err := root.Execute()
		if err == nil {
			t.Fatal("expected oversized input error, got nil")
		}
		if managerCalled {
			t.Fatal("database manager should not be created on oversized input")
		}
		if !strings.Contains(err.Error(), "exceeds maximum length") {
			t.Fatalf("error = %q, want maximum-length message", err.Error())
		}
		assertNoLeak(
			t,
			[]string{sentinel, "OVERSIZED_SECRET", "ZZZZZZZZ"},
			err.Error(),
			stdout.String(),
			stderr.String(),
			logBuf.String(),
		)

		// Verify maxPasswordBytes+1 with trailing LF fails with exceeds maximum length.
		_, err = readPasswordFromStdin(
			strings.NewReader(strings.Repeat("a", maxPasswordBytes+1)+"\n"),
			func(io.Reader) bool { return false },
		)
		if err == nil || !strings.Contains(err.Error(), "exceeds maximum length") {
			t.Fatalf("expected exceeds maximum length error for maxPasswordBytes+1 with LF, got: %v", err)
		}

		// Verify read is strictly bounded on an infinite stream.
		inf := &countingRepeatReader{b: 'x'}
		_, err = readPasswordFromStdin(inf, func(io.Reader) bool { return false })
		if err == nil {
			t.Fatal("expected oversized error on infinite stream")
		}
		if inf.read > maxPasswordBytes+3 {
			t.Fatalf("bounded read consumed %d bytes, want <= %d", inf.read, maxPasswordBytes+3)
		}
	})

	for _, combine := range []bool{false, true} {
		name := "reader error after partial sentinel bytes"
		if combine {
			name += " (same Read call)"
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv("POSTGRES_PASSWORD", "")
			sentinel := "PARTIAL_SECRET_SENTINEL_42"
			managerCalled := false
			var stdout, stderr, logBuf bytes.Buffer

			root := newRootCommand(cliDeps{
				isTerminal: func(io.Reader) bool { return false },
				newManager: func(_ *structs.DatabaseConnection, _ *logrus.Logger, _ bool) (userManager, error) {
					managerCalled = true
					return &fakeUserManager{}, nil
				},
				logOut: &logBuf,
			})
			root.SetArgs([]string{"create-user", "myuser", "--password-stdin", "--verbose"})
			root.SetIn(&partialThenErrReader{
				payload:      []byte(sentinel),
				err:          errors.New("simulated io failure"),
				combineCalls: combine,
			})
			root.SetOut(&stdout)
			root.SetErr(&stderr)

			err := root.Execute()
			if err == nil {
				t.Fatal("expected reader error, got nil")
			}
			if managerCalled {
				t.Fatal("database manager should not be created on stdin read error")
			}
			if !strings.Contains(err.Error(), "failed to read password from standard input") {
				t.Fatalf("error = %q, want stdin read failure prefix", err.Error())
			}
			if !strings.Contains(err.Error(), "simulated io failure") {
				t.Fatalf("error = %q, want wrapped cause", err.Error())
			}
			assertNoLeak(
				t,
				[]string{sentinel, "PARTIAL_SECRET", "SENTINEL_42"},
				err.Error(),
				stdout.String(),
				stderr.String(),
				logBuf.String(),
			)
		})
	}
}

func TestCreateUserPreflightOrderAndIAMConflict(t *testing.T) {
	t.Run("IAM with --password-stdin fails before reading stdin and before DB connection", func(t *testing.T) {
		t.Setenv("POSTGRES_PASSWORD", "")
		reader := &failOnReadReader{t: t}
		managerCalled := false
		var stdout, stderr, logBuf bytes.Buffer

		root := newRootCommand(cliDeps{
			isTerminal: func(io.Reader) bool { return true },
			newManager: func(_ *structs.DatabaseConnection, _ *logrus.Logger, _ bool) (userManager, error) {
				managerCalled = true
				return &fakeUserManager{}, nil
			},
			logOut: &logBuf,
		})
		root.SetArgs([]string{"create-user", "iam_user", "--auth-method", "iam", "--password-stdin"})
		root.SetIn(reader)
		root.SetOut(&stdout)
		root.SetErr(&stderr)

		err := root.Execute()
		if err == nil {
			t.Fatal("expected error combining --auth-method iam and --password-stdin, got nil")
		}
		if reader.called {
			t.Fatal("stdin must not be read when --auth-method iam conflicts with --password-stdin")
		}
		if managerCalled {
			t.Fatal("database manager must not be created on IAM conflict")
		}
		if !strings.Contains(err.Error(), "--password-stdin") || !strings.Contains(err.Error(), "iam") {
			t.Fatalf("error = %q, want clear IAM conflict message", err.Error())
		}
	})

	t.Run("invalid auth-method fails before reading stdin and before DB connection", func(t *testing.T) {
		t.Setenv("POSTGRES_PASSWORD", "")
		reader := &failOnReadReader{t: t}
		managerCalled := false
		var stdout, stderr, logBuf bytes.Buffer

		root := newRootCommand(cliDeps{
			isTerminal: func(io.Reader) bool { return true },
			newManager: func(_ *structs.DatabaseConnection, _ *logrus.Logger, _ bool) (userManager, error) {
				managerCalled = true
				return &fakeUserManager{}, nil
			},
			logOut: &logBuf,
		})
		root.SetArgs([]string{"create-user", "myuser", "--auth-method", "kerberos", "--password-stdin"})
		root.SetIn(reader)
		root.SetOut(&stdout)
		root.SetErr(&stderr)

		err := root.Execute()
		if err == nil {
			t.Fatal("expected invalid auth-method error, got nil")
		}
		if reader.called {
			t.Fatal("stdin must not be read when auth-method is invalid")
		}
		if managerCalled {
			t.Fatal("database manager must not be created when auth-method is invalid")
		}
		if !strings.Contains(err.Error(), "invalid auth-method") {
			t.Fatalf("error = %q, want invalid auth-method message", err.Error())
		}
	})

	t.Run("invalid privileges fail before reading stdin and before DB connection", func(t *testing.T) {
		t.Setenv("POSTGRES_PASSWORD", "")
		reader := &failOnReadReader{t: t}
		managerCalled := false
		var stdout, stderr, logBuf bytes.Buffer

		root := newRootCommand(cliDeps{
			isTerminal: func(io.Reader) bool { return true },
			newManager: func(_ *structs.DatabaseConnection, _ *logrus.Logger, _ bool) (userManager, error) {
				managerCalled = true
				return &fakeUserManager{}, nil
			},
			logOut: &logBuf,
		})
		root.SetArgs([]string{
			"create-user", "myuser",
			"--password-stdin",
			"--privileges", "SELECT",
			"--databases", "mydb",
		})
		root.SetIn(reader)
		root.SetOut(&stdout)
		root.SetErr(&stderr)

		err := root.Execute()
		if err == nil {
			t.Fatal("expected invalid privileges error, got nil")
		}
		if reader.called {
			t.Fatal("stdin must not be read when privileges are invalid")
		}
		if managerCalled {
			t.Fatal("database manager must not be created when privileges are invalid")
		}
		if !strings.Contains(err.Error(), "invalid privileges") {
			t.Fatalf("error = %q, want invalid privileges message", err.Error())
		}
	})

	t.Run("password auth with --iam-role still logs warning", func(t *testing.T) {
		t.Setenv("POSTGRES_PASSWORD", "admin-test-pass")
		fakeMgr := &fakeUserManager{}
		var stdout, stderr, logBuf bytes.Buffer

		root := newRootCommand(cliDeps{
			isTerminal: func(io.Reader) bool { return false },
			newManager: func(_ *structs.DatabaseConnection, _ *logrus.Logger, _ bool) (userManager, error) {
				return fakeMgr, nil
			},
			logOut: &logBuf,
		})
		root.SetArgs([]string{
			"create-user", "myuser",
			"--auth-method", "password",
			"--iam-role", "arn:aws:iam::123456789012:role/TestRole",
		})
		root.SetOut(&stdout)
		root.SetErr(&stderr)

		if err := root.Execute(); err != nil {
			t.Fatalf("Execute() unexpected error: %v", err)
		}
		if !strings.Contains(logBuf.String(), "IAM role specified for password authentication user") {
			t.Fatalf("expected IAM role warning in logs, got:\n%s", logBuf.String())
		}
	})

	t.Run("CreateUser failure never leaks password in error or logs", func(t *testing.T) {
		t.Setenv("POSTGRES_PASSWORD", "admin-test-pass")
		sentinel := "  S3cr'et\"Pass!99\t "
		fakeMgr := &fakeUserManager{createErr: errors.New("pq: permission denied")}
		var stdout, stderr, logBuf bytes.Buffer

		root := newRootCommand(cliDeps{
			isTerminal: func(io.Reader) bool { return false },
			newManager: func(_ *structs.DatabaseConnection, _ *logrus.Logger, _ bool) (userManager, error) {
				return fakeMgr, nil
			},
			logOut: &logBuf,
		})
		root.SetArgs([]string{"create-user", "myuser", "--password-stdin", "--verbose"})
		root.SetIn(strings.NewReader(sentinel + "\n"))
		root.SetOut(&stdout)
		root.SetErr(&stderr)

		err := root.Execute()
		if err == nil {
			t.Fatal("expected CreateUser error, got nil")
		}
		assertNoLeak(
			t,
			[]string{sentinel, strings.TrimSpace(sentinel), "S3cr'et", "S3cr", "Pass!99"},
			err.Error(),
			stdout.String(),
			stderr.String(),
			logBuf.String(),
		)
	})
}

func TestCommandFlagStateIsolationAcrossInvocations(t *testing.T) {
	t.Setenv("POSTGRES_PASSWORD", "admin-test-pass")

	var firstDryRun, secondDryRun bool
	firstMgr := &fakeUserManager{}
	root1 := newRootCommand(cliDeps{
		isTerminal: func(io.Reader) bool { return false },
		newManager: func(_ *structs.DatabaseConnection, _ *logrus.Logger, dryRun bool) (userManager, error) {
			firstDryRun = dryRun
			return firstMgr, nil
		},
		logOut: &bytes.Buffer{},
	})
	root1.SetArgs([]string{"create-user", "user1", "--dry-run", "--verbose", "--password-stdin"})
	root1.SetIn(strings.NewReader("first-secret\n"))
	root1.SetOut(&bytes.Buffer{})
	root1.SetErr(&bytes.Buffer{})

	if err := root1.Execute(); err != nil {
		t.Fatalf("first invocation failed: %v", err)
	}
	if !firstDryRun {
		t.Fatal("expected first invocation dryRun=true")
	}

	secondMgr := &fakeUserManager{}
	reader := &failOnReadReader{t: t}
	root2 := newRootCommand(cliDeps{
		isTerminal: func(io.Reader) bool { return true },
		newManager: func(_ *structs.DatabaseConnection, _ *logrus.Logger, dryRun bool) (userManager, error) {
			secondDryRun = dryRun
			return secondMgr, nil
		},
		logOut: &bytes.Buffer{},
	})
	root2.SetArgs([]string{"create-user", "user2"})
	root2.SetIn(reader)
	root2.SetOut(&bytes.Buffer{})
	root2.SetErr(&bytes.Buffer{})

	if err := root2.Execute(); err != nil {
		t.Fatalf("second invocation failed: %v", err)
	}
	if secondDryRun {
		t.Fatal("dryRun flag leaked from first invocation into second invocation")
	}
	if reader.called {
		t.Fatal("password-stdin flag leaked from first invocation into second invocation")
	}
	if len(secondMgr.createdUsers) != 1 || secondMgr.createdUsers[0].Password != "" {
		t.Fatalf("unexpected second invocation user config: %+v", secondMgr.createdUsers)
	}
}

func configureTestcontainersRyukIfNeeded(t *testing.T) {
	t.Helper()
	if os.Getenv("TESTCONTAINERS_RYUK_DISABLED") == "true" {
		return
	}
	dockerHost := os.Getenv("DOCKER_HOST")
	homeDir, _ := os.UserHomeDir()
	colimaSocket := filepath.Join(homeDir, ".colima", "default", "docker.sock")
	limaSocket := filepath.Join(homeDir, ".lima", "default", "docker.sock")
	if strings.Contains(dockerHost, ".colima") ||
		strings.Contains(dockerHost, "colima") ||
		strings.Contains(dockerHost, ".lima") ||
		strings.Contains(dockerHost, "podman") ||
		fileExists(colimaSocket) ||
		fileExists(limaSocket) {
		t.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func postgresURL(host string, port int, dbName, user, password string) string {
	u := &url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(user, password),
		Host:     net.JoinHostPort(host, strconv.Itoa(port)),
		Path:     dbName,
		RawQuery: "sslmode=disable",
	}
	return u.String()
}

func TestCreateUserPasswordStdinEndToEndPostgreSQL(t *testing.T) {
	configureTestcontainersRyukIfNeeded(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	pgContainer, err := postgres.Run(ctx,
		"postgres:15-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(25*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("failed to start PostgreSQL container: %v", err)
	}
	defer func() {
		termCtx, termCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer termCancel()
		if termErr := pgContainer.Terminate(termCtx); termErr != nil {
			t.Logf("failed to terminate PostgreSQL container: %v", termErr)
		}
	}()

	host, err := pgContainer.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get container host: %v", err)
	}
	if host == "localhost" {
		host = "127.0.0.1"
	}

	mappedPort, err := pgContainer.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatalf("failed to get mapped port: %v", err)
	}
	port, err := strconv.Atoi(mappedPort.Port())
	if err != nil {
		t.Fatalf("failed to parse mapped port %q: %v", mappedPort.Port(), err)
	}

	// Wait briefly until PostgreSQL accepts connections reliably.
	adminDSN := postgresURL(host, port, "testdb", "testuser", "testpass")
	var adminDB *sql.DB
	for attempt := 0; attempt < 5; attempt++ {
		adminDB, err = sql.Open("postgres", adminDSN)
		if err == nil {
			if pingErr := adminDB.Ping(); pingErr == nil {
				break
			} else {
				_ = adminDB.Close()
				err = pingErr
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("failed to connect to test PostgreSQL container: %v", err)
	}
	defer func() { _ = adminDB.Close() }()

	t.Setenv("POSTGRES_HOST", host)
	t.Setenv("POSTGRES_PORT", strconv.Itoa(port))
	t.Setenv("POSTGRES_DB", "testdb")
	t.Setenv("POSTGRES_USER", "testuser")
	t.Setenv("POSTGRES_PASSWORD", "testpass")
	t.Setenv("POSTGRES_SSLMODE", "disable")
	t.Setenv("POSTGRES_IAM_AUTH", "false")

	sentinelPassword := "  S3cr'et\"Pass\\!99\t "
	forbiddenSubstrings := []string{
		sentinelPassword,
		strings.TrimSpace(sentinelPassword),
		"S3cr'et",
		"S3cr''et",
		"S3cr",
		"Pass\\!99",
	}

	t.Run("dry-run redacts password at info and debug levels", func(t *testing.T) {
		for _, verbose := range []bool{false, true} {
			name := "info"
			args := []string{"create-user", "e2e_dryrun_user", "--password-stdin", "--dry-run"}
			if verbose {
				name = "debug"
				args = append(args, "--verbose")
			}

			t.Run(name, func(t *testing.T) {
				var stdout, stderr, logBuf bytes.Buffer
				deps := defaultCLIDeps()
				deps.logOut = &logBuf

				root := newRootCommand(deps)
				root.SetArgs(args)
				root.SetIn(strings.NewReader(sentinelPassword + "\n"))
				root.SetOut(&stdout)
				root.SetErr(&stderr)

				if err := root.Execute(); err != nil {
					t.Fatalf("CLI dry-run failed: %v\nlogs:\n%s", err, logBuf.String())
				}

				if !strings.Contains(logBuf.String(), "WITH PASSWORD '[REDACTED]'") {
					t.Fatalf("expected [REDACTED] in dry-run logs, got:\n%s", logBuf.String())
				}
				assertNoLeak(t, forbiddenSubstrings, stdout.String(), stderr.String(), logBuf.String())

				var exists int
				err := adminDB.QueryRow("SELECT 1 FROM pg_roles WHERE rolname = $1", "e2e_dryrun_user").Scan(&exists)
				if !errors.Is(err, sql.ErrNoRows) {
					t.Fatalf("dry-run should not have created user e2e_dryrun_user (err=%v)", err)
				}
			})
		}
	})

	t.Run("live create-user authenticates with exact password and rejects trimmed password", func(t *testing.T) {
		var stdout, stderr, logBuf bytes.Buffer
		deps := defaultCLIDeps()
		deps.logOut = &logBuf

		root := newRootCommand(deps)
		root.SetArgs([]string{
			"create-user", "e2e_stdin_user",
			"--password-stdin",
			"--privileges", "CONNECT",
			"--databases", "testdb",
			"--verbose",
		})
		root.SetIn(strings.NewReader(sentinelPassword + "\r\n"))
		root.SetOut(&stdout)
		root.SetErr(&stderr)

		if err := root.Execute(); err != nil {
			t.Fatalf("CLI create-user failed: %v\nstderr:\n%s\nlogs:\n%s", err, stderr.String(), logBuf.String())
		}
		assertNoLeak(t, forbiddenSubstrings, stdout.String(), stderr.String(), logBuf.String())

		// Exact password must authenticate.
		exactDB, err := sql.Open("postgres", postgresURL(host, port, "testdb", "e2e_stdin_user", sentinelPassword))
		if err != nil {
			t.Fatalf("sql.Open(exact) failed: %v", err)
		}
		defer func() { _ = exactDB.Close() }()

		if err := exactDB.Ping(); err != nil {
			t.Fatalf("expected PostgreSQL authentication with exact password to succeed, got: %v", err)
		}

		// Trimmed password must fail authentication.
		trimmedDB, err := sql.Open("postgres", postgresURL(host, port, "testdb", "e2e_stdin_user", strings.TrimSpace(sentinelPassword)))
		if err != nil {
			t.Fatalf("sql.Open(trimmed) failed: %v", err)
		}
		defer func() { _ = trimmedDB.Close() }()

		if err := trimmedDB.Ping(); err == nil {
			t.Fatal("expected PostgreSQL authentication with trimmed password to fail, but Ping succeeded")
		}
	})
}
