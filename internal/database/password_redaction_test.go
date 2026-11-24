package database

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/ben-vaughan-nttd/postgres-user-manager/internal/structs"
	"github.com/sirupsen/logrus"
)

func TestRedactPasswordFromError(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(&bytes.Buffer{})
	mgr := &Manager{logger: logger}

	password := "  S3cr'et\"Pass!99\t "
	escaped := mgr.escapeString(password)

	t.Run("scrubs raw password without wrapping original error", func(t *testing.T) {
		orig := errors.New("pq: passwordcheck rejected " + password + " (repeat: " + password + ")")
		got := mgr.redactPasswordFromError(orig, password)
		if got == nil {
			t.Fatal("expected non-nil error")
		}
		if strings.Contains(got.Error(), password) || strings.Contains(got.Error(), "S3cr'et") {
			t.Fatalf("redacted error leaked password: %q", got.Error())
		}
		if !strings.Contains(got.Error(), "[REDACTED]") {
			t.Fatalf("expected [REDACTED] placeholder in error: %q", got.Error())
		}
		if errors.Is(got, orig) || errors.Unwrap(got) != nil {
			t.Fatal("scrubbed error must not wrap the unredacted original error")
		}
	})

	t.Run("scrubs SQL quote-escaped password without wrapping original error", func(t *testing.T) {
		orig := errors.New("pq: syntax error near '" + escaped + "'")
		got := mgr.redactPasswordFromError(orig, password)
		if got == nil {
			t.Fatal("expected non-nil error")
		}
		if strings.Contains(got.Error(), escaped) || strings.Contains(got.Error(), password) || strings.Contains(got.Error(), "S3cr") {
			t.Fatalf("redacted error leaked escaped password: %q", got.Error())
		}
		if !strings.Contains(got.Error(), "[REDACTED]") {
			t.Fatalf("expected [REDACTED] placeholder in error: %q", got.Error())
		}
		if errors.Is(got, orig) || errors.Unwrap(got) != nil {
			t.Fatal("scrubbed error must not wrap the unredacted original error")
		}
	})

	t.Run("preserves original error and wrapping when no secret is present", func(t *testing.T) {
		orig := errors.New("pq: permission denied to create role")
		got := mgr.redactPasswordFromError(orig, password)
		if !errors.Is(got, orig) {
			t.Fatalf("expected errors.Is(got, orig) == true, got %v", got)
		}
		if got.Error() != orig.Error() {
			t.Fatalf("got error %q, want %q", got.Error(), orig.Error())
		}
	})
}

func TestBuildCreateUserQueryRedaction(t *testing.T) {
	sentinel := "  S3cr'et\"Pass!99\t "
	forbiddenSubstrings := []string{
		sentinel,
		strings.TrimSpace(sentinel),
		"S3cr'et",
		"S3cr''et",
		"S3cr",
		"Pass!99",
	}

	logger := logrus.New()
	logger.SetOutput(&bytes.Buffer{})
	mgr := &Manager{logger: logger}

	t.Run("password auth redacted vs unredacted", func(t *testing.T) {
		user := &structs.UserConfig{
			Username:        "redact_user",
			Password:        sentinel,
			AuthMethod:      "password",
			CanLogin:        true,
			ConnectionLimit: 5,
		}

		redacted := mgr.buildCreateUserQuery(user, true)
		wantRedacted := `CREATE USER "redact_user" WITH PASSWORD '[REDACTED]' LOGIN CONNECTION LIMIT 5`
		if redacted != wantRedacted {
			t.Fatalf("redacted query = %q, want %q", redacted, wantRedacted)
		}
		for _, sub := range forbiddenSubstrings {
			if strings.Contains(redacted, sub) {
				t.Fatalf("redacted query leaked substring %q: %q", sub, redacted)
			}
		}

		live := mgr.buildCreateUserQuery(user, false)
		wantLive := `CREATE USER "redact_user" WITH PASSWORD '  S3cr''et"Pass!99` + "\t " + `' LOGIN CONNECTION LIMIT 5`
		if live != wantLive {
			t.Fatalf("live query = %q, want %q", live, wantLive)
		}
	})

	t.Run("empty password omits WITH PASSWORD clause", func(t *testing.T) {
		user := &structs.UserConfig{
			Username:   "nopass_user",
			Password:   "",
			AuthMethod: "password",
			CanLogin:   false,
		}

		want := `CREATE USER "nopass_user" NOLOGIN`
		if got := mgr.buildCreateUserQuery(user, true); got != want {
			t.Fatalf("redacted query = %q, want %q", got, want)
		}
		if got := mgr.buildCreateUserQuery(user, false); got != want {
			t.Fatalf("live query = %q, want %q", got, want)
		}
	})

	t.Run("iam auth omits WITH PASSWORD even if password field set", func(t *testing.T) {
		user := &structs.UserConfig{
			Username:   "iam_redact_user",
			Password:   sentinel,
			AuthMethod: "iam",
			CanLogin:   true,
		}

		want := `CREATE USER "iam_redact_user" LOGIN`
		for _, redact := range []bool{true, false} {
			got := mgr.buildCreateUserQuery(user, redact)
			if got != want {
				t.Fatalf("query(redact=%v) = %q, want %q", redact, got, want)
			}
			for _, sub := range forbiddenSubstrings {
				if strings.Contains(got, sub) {
					t.Fatalf("IAM query(redact=%v) leaked substring %q: %q", redact, sub, got)
				}
			}
		}
	})
}

func TestCreateUserDryRunAndLiveLogRedaction(t *testing.T) {
	setup := SetupFlexibleTestDatabase(t)
	defer setup.Cleanup(t)
	defer setup.ResetDatabase(t)

	sentinel := "  S3cr'et\"Pass!99\t "
	forbiddenSubstrings := []string{
		sentinel,
		strings.TrimSpace(sentinel),
		"S3cr'et",
		"S3cr''et",
		"S3cr",
		"Pass!99",
	}

	levels := []struct {
		name  string
		level logrus.Level
	}{
		{name: "info", level: logrus.InfoLevel},
		{name: "debug", level: logrus.DebugLevel},
	}

	for _, lv := range levels {
		t.Run("dry-run_"+lv.name, func(t *testing.T) {
			var logBuf bytes.Buffer
			logger := logrus.New()
			logger.SetLevel(lv.level)
			logger.SetOutput(&logBuf)

			dryMgr, err := NewManager(setup.ConnInfo, logger, true)
			if err != nil {
				t.Fatalf("NewManager(dryRun=true) failed: %v", err)
			}
			defer func() {
				if closeErr := dryMgr.Close(); closeErr != nil {
					t.Logf("failed to close dry-run manager: %v", closeErr)
				}
			}()

			username := "dryrun_redact_" + lv.name
			err = dryMgr.CreateUser(&structs.UserConfig{
				Username:   username,
				Password:   sentinel,
				AuthMethod: "password",
				CanLogin:   true,
				Enabled:    true,
			})
			if err != nil {
				t.Fatalf("CreateUser(dryRun=true) returned error: %v", err)
			}

			logs := logBuf.String()
			if !strings.Contains(logs, "WITH PASSWORD '[REDACTED]'") {
				t.Fatalf("expected dry-run log to contain redacted password clause, got:\n%s", logs)
			}
			for _, sub := range forbiddenSubstrings {
				if strings.Contains(logs, sub) {
					t.Fatalf("dry-run log at %s leaked substring %q:\n%s", lv.name, sub, logs)
				}
			}

			exists, err := setup.Manager.UserExists(username)
			if err != nil {
				t.Fatalf("UserExists(%q) failed: %v", username, err)
			}
			if exists {
				t.Fatalf("dry-run unexpectedly created user %q", username)
			}
		})

		t.Run("live_"+lv.name, func(t *testing.T) {
			var logBuf bytes.Buffer
			logger := logrus.New()
			logger.SetLevel(lv.level)
			logger.SetOutput(&logBuf)

			liveMgr, err := NewManager(setup.ConnInfo, logger, false)
			if err != nil {
				t.Fatalf("NewManager(dryRun=false) failed: %v", err)
			}
			defer func() {
				if closeErr := liveMgr.Close(); closeErr != nil {
					t.Logf("failed to close live manager: %v", closeErr)
				}
			}()

			username := "live_redact_" + lv.name
			defer func() {
				_ = setup.Manager.DropUser(username)
			}()

			err = liveMgr.CreateUser(&structs.UserConfig{
				Username:   username,
				Password:   sentinel,
				AuthMethod: "password",
				CanLogin:   true,
				Enabled:    true,
			})
			if err != nil {
				t.Fatalf("CreateUser(dryRun=false) returned error: %v", err)
			}

			logs := logBuf.String()
			for _, sub := range forbiddenSubstrings {
				if strings.Contains(logs, sub) {
					t.Fatalf("live log at %s leaked substring %q:\n%s", lv.name, sub, logs)
				}
			}
		})
	}
}
