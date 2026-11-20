package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ben-vaughan-nttd/postgres-user-manager/internal/permissions"
	"github.com/ben-vaughan-nttd/postgres-user-manager/internal/structs"
	"github.com/sirupsen/logrus"
)

func TestValidateConfigAcceptsValidPermissions(t *testing.T) {
	cfg := &structs.Config{
		Groups: []structs.GroupConfig{{
			Name:       "app_group",
			Privileges: []string{"CONNECT", "TEMP"},
			Databases:  []string{"myapp_db"},
			Permissions: []string{
				"GRANT USAGE ON SCHEMA app TO app_group",
				"GRANT SELECT ON ALL TABLES IN SCHEMA app TO app_group",
			},
		}},
		Users: []structs.UserConfig{{
			Username:   "App_User",
			Privileges: []string{"CONNECT"},
			Databases:  []string{"myapp_db"},
			Permissions: []string{
				`GRANT SELECT, INSERT ON TABLE app.orders TO "App_User"`,
				`GRANT USAGE ON SEQUENCE app.orders_id_seq TO "App_User"`,
			},
		}},
	}

	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
}

func TestValidateConfigRejectsBadPermissions(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *structs.Config
		wantErr error
		wantMsg string
	}{
		{
			name: "revoke in config",
			cfg: &structs.Config{Users: []structs.UserConfig{{
				Username:    "app_user",
				Permissions: []string{"REVOKE SELECT ON TABLE t FROM app_user"},
			}}},
			wantErr: permissions.ErrActionNotAllowed,
			wantMsg: "remove the grant",
		},
		{
			name: "public grantee",
			cfg: &structs.Config{Users: []structs.UserConfig{{
				Username:    "app_user",
				Permissions: []string{"GRANT SELECT ON TABLE t TO PUBLIC"},
			}}},
			wantErr: permissions.ErrUnsupportedOption,
		},
		{
			name: "grantee is another role",
			cfg: &structs.Config{Groups: []structs.GroupConfig{{
				Name:        "app_group",
				Permissions: []string{"GRANT SELECT ON TABLE t TO someone_else"},
			}}},
			wantErr: permissions.ErrGranteeMismatch,
		},
		{
			name: "role membership",
			cfg: &structs.Config{Users: []structs.UserConfig{{
				Username:    "app_user",
				Permissions: []string{"GRANT admins TO app_user"},
			}}},
			wantErr: permissions.ErrUnsupportedStatement,
		},
		{
			name: "chained statement in permissions",
			cfg: &structs.Config{Users: []structs.UserConfig{{
				Username:    "app_user",
				Permissions: []string{"GRANT SELECT ON TABLE t TO app_user; DROP TABLE t"},
			}}},
			wantErr: permissions.ErrMultipleStatements,
		},
		{
			name: "injection through legacy privilege",
			cfg: &structs.Config{Users: []structs.UserConfig{{
				Username:   "app_user",
				Privileges: []string{`CONNECT ON DATABASE x TO y; DROP TABLE users; --`},
				Databases:  []string{"db"},
			}}},
			wantErr: permissions.ErrMultipleStatements,
		},
		{
			name: "unsupported legacy privilege",
			cfg: &structs.Config{Groups: []structs.GroupConfig{{
				Name:       "app_group",
				Privileges: []string{"SELECT"},
				Databases:  []string{"db"},
			}}},
			wantErr: permissions.ErrUnsupportedPrivilege,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(tt.cfg)
			if err == nil {
				t.Fatal("ValidateConfig() succeeded, want error")
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidateConfig() error = %v, want errors.Is %v", err, tt.wantErr)
			}
			if tt.wantMsg != "" && !strings.Contains(err.Error(), tt.wantMsg) {
				t.Fatalf("error %q does not contain %q", err, tt.wantMsg)
			}
		})
	}
}

func TestValidateConfigReportsEveryProblem(t *testing.T) {
	cfg := &structs.Config{
		Groups: []structs.GroupConfig{{Name: "g", Permissions: []string{"GRANT SELECT ON TABLE t TO PUBLIC"}}},
		Users:  []structs.UserConfig{{Username: "u", Permissions: []string{"REVOKE SELECT ON TABLE t FROM u"}}},
	}
	err := ValidateConfig(cfg)
	if !errors.Is(err, permissions.ErrUnsupportedOption) || !errors.Is(err, permissions.ErrActionNotAllowed) {
		t.Fatalf("expected both problems in one error, got %v", err)
	}
	for _, want := range []string{"groups[0]", "users[0]"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not locate %s", err, want)
		}
	}
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadConfigRunsPreflight(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	manager := NewManager(logger)

	t.Run("valid permissions load", func(t *testing.T) {
		path := writeTempConfig(t, `{"users":[{"username":"app_user","enabled":true,"permissions":["GRANT SELECT ON TABLE app.t TO app_user"]}],"groups":[]}`)
		cfg, err := manager.LoadConfig(path)
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		if got := cfg.Users[0].Permissions; len(got) != 1 {
			t.Fatalf("Permissions = %v", got)
		}
	})

	t.Run("revoke fails", func(t *testing.T) {
		path := writeTempConfig(t, `{"users":[{"username":"app_user","enabled":true,"permissions":["REVOKE SELECT ON TABLE app.t FROM app_user"]}],"groups":[]}`)
		if _, err := manager.LoadConfig(path); !errors.Is(err, permissions.ErrActionNotAllowed) {
			t.Fatalf("LoadConfig() error = %v, want ErrActionNotAllowed", err)
		}
	})

	t.Run("public grant fails", func(t *testing.T) {
		path := writeTempConfig(t, `{"users":[],"groups":[{"name":"g","permissions":["GRANT SELECT ON TABLE app.t TO PUBLIC"]}]}`)
		if _, err := manager.LoadConfig(path); !errors.Is(err, permissions.ErrUnsupportedOption) {
			t.Fatalf("LoadConfig() error = %v, want ErrUnsupportedOption", err)
		}
	})
}

func TestExampleConfigPassesPreflight(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	cfg, err := NewManager(logger).LoadConfig(filepath.Join("..", "..", "config.example.json"))
	if err != nil {
		t.Fatalf("config.example.json failed to load: %v", err)
	}

	var withPermissions int
	for _, u := range cfg.Users {
		if len(u.Permissions) > 0 {
			withPermissions++
		}
	}
	for _, g := range cfg.Groups {
		if len(g.Permissions) > 0 {
			withPermissions++
		}
	}
	if withPermissions == 0 {
		t.Error("config.example.json should include a permissions example")
	}
}
