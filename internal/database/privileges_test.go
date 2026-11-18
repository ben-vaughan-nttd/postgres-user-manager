package database

import (
	"errors"
	"strings"
	"testing"

	"github.com/ben-vaughan-nttd/postgres-user-manager/internal/permissions"
	"github.com/ben-vaughan-nttd/postgres-user-manager/internal/structs"
)

const (
	testDatabase = "test_privileges_db"
)

func TestGrantPrivileges(t *testing.T) {
	setup := SetupFlexibleTestDatabase(t)
	defer setup.Cleanup(t)
	defer setup.ResetDatabase(t)

	// Create test database for privilege testing
	setup.CreateTestDatabase(t, testDatabase)
	defer setup.DropTestDatabase(t, testDatabase)

	// Create a test user
	userConfig := &structs.UserConfig{
		Username:   "test_user",
		Password:   "test_pass",
		AuthMethod: "password",
		CanLogin:   true,
		Enabled:    true,
	}

	err := setup.Manager.CreateUser(userConfig)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Grant privileges to user
	privileges := []string{"CONNECT", "CREATE"}
	databases := []string{testDatabase}

	err = setup.Manager.GrantPrivileges("test_user", privileges, databases)
	if err != nil {
		t.Fatalf("Failed to grant privileges: %v", err)
	}

	// Test should pass if no error occurred
}

func TestRevokePrivileges(t *testing.T) {
	setup := SetupFlexibleTestDatabase(t)
	defer setup.Cleanup(t)
	defer setup.ResetDatabase(t)

	// Create test database for privilege testing
	setup.CreateTestDatabase(t, testDatabase)
	defer setup.DropTestDatabase(t, testDatabase)

	// Create a test user
	userConfig := &structs.UserConfig{
		Username:   "test_user",
		Password:   "test_pass",
		AuthMethod: "password",
		CanLogin:   true,
		Enabled:    true,
	}

	err := setup.Manager.CreateUser(userConfig)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Grant privileges first
	privileges := []string{"CONNECT", "CREATE"}
	databases := []string{testDatabase}

	err = setup.Manager.GrantPrivileges("test_user", privileges, databases)
	if err != nil {
		t.Fatalf("Failed to grant privileges: %v", err)
	}

	// Now revoke privileges
	err = setup.Manager.RevokePrivileges("test_user", privileges, databases)
	if err != nil {
		t.Fatalf("Failed to revoke privileges: %v", err)
	}

	// Test should pass if no error occurred
}

func TestGrantPrivilegesToGroup(t *testing.T) {
	setup := SetupFlexibleTestDatabase(t)
	defer setup.Cleanup(t)
	defer setup.ResetDatabase(t)

	// Create test database for privilege testing
	setup.CreateTestDatabase(t, testDatabase)
	defer setup.DropTestDatabase(t, testDatabase)

	// Create a test group
	groupConfig := &structs.GroupConfig{
		Name:    "test_group",
		Inherit: true,
	}

	err := setup.Manager.CreateGroup(groupConfig)
	if err != nil {
		t.Fatalf("Failed to create test group: %v", err)
	}

	// Grant privileges to group
	privileges := []string{"CONNECT"}
	databases := []string{testDatabase}

	err = setup.Manager.GrantPrivileges("test_group", privileges, databases)
	if err != nil {
		t.Fatalf("Failed to grant privileges to group: %v", err)
	}

	// Test should pass if no error occurred
}

func TestSyncConfiguration(t *testing.T) {
	setup := SetupFlexibleTestDatabase(t)
	defer setup.Cleanup(t)
	defer setup.ResetDatabase(t)

	// Create test database for sync testing
	setup.CreateTestDatabase(t, testDatabase)
	defer setup.DropTestDatabase(t, testDatabase)

	config := createTestSyncConfig()

	// Sync the configuration
	result, err := setup.Manager.SyncConfiguration(config)
	if err != nil {
		t.Fatalf("Failed to sync configuration: %v", err)
	}

	// Verify sync results
	verifySyncResults(t, result)
	verifyGroupsExist(t, setup, config)
	verifyUsersExist(t, setup, config)
	verifyUserMemberships(t, setup)
}

func createTestSyncConfig() *structs.Config {
	return &structs.Config{
		Groups: []structs.GroupConfig{
			{
				Name:        "app_group",
				Privileges:  []string{"CONNECT"},
				Databases:   []string{testDatabase},
				Description: "Application group",
				Inherit:     true,
			},
			{
				Name:        "read_only",
				Privileges:  []string{"CONNECT"},
				Databases:   []string{testDatabase},
				Description: "Read-only group",
				Inherit:     true,
			},
		},
		Users: []structs.UserConfig{
			{
				Username:   "app_user",
				Password:   "app_pass",
				Groups:     []string{"app_group"},
				Privileges: []string{"CONNECT"},
				Databases:  []string{testDatabase},
				Enabled:    true,
				AuthMethod: "password",
				CanLogin:   true,
			},
			{
				Username:   "readonly_user",
				Groups:     []string{"read_only"},
				Privileges: []string{"CONNECT"},
				Databases:  []string{testDatabase},
				Enabled:    true,
				AuthMethod: "iam",
				CanLogin:   true,
			},
			{
				Username: "disabled_user",
				Password: "disabled_pass",
				Enabled:  false, // This user should be skipped
				CanLogin: true,
			},
		},
	}
}

func verifySyncResults(t *testing.T, result *structs.SyncResult) {
	if len(result.GroupsCreated) != 2 {
		t.Errorf("Expected 2 groups to be created, got %d", len(result.GroupsCreated))
	}

	expectedGroups := map[string]bool{"app_group": true, "read_only": true}
	for _, group := range result.GroupsCreated {
		if !expectedGroups[group] {
			t.Errorf("Unexpected group created: %s", group)
		}
	}

	if len(result.UsersCreated) != 2 {
		t.Errorf("Expected 2 users to be created, got %d", len(result.UsersCreated))
	}

	expectedUsers := map[string]bool{"app_user": true, "readonly_user": true}
	for _, user := range result.UsersCreated {
		if !expectedUsers[user] {
			t.Errorf("Unexpected user created: %s", user)
		}
	}
}

func verifyGroupsExist(t *testing.T, setup DatabaseTestSetup, config *structs.Config) {
	for _, group := range config.Groups {
		exists, err := setup.GetManager().GroupExists(group.Name)
		if err != nil {
			t.Fatalf("Error checking group existence: %v", err)
		}
		if !exists {
			t.Errorf("Group %s should exist after sync", group.Name)
		}
	}
}

func verifyUsersExist(t *testing.T, setup DatabaseTestSetup, config *structs.Config) {
	// Verify enabled users exist
	for _, user := range config.Users {
		if !user.Enabled {
			continue
		}
		exists, err := setup.GetManager().UserExists(user.Username)
		if err != nil {
			t.Fatalf("Error checking user existence: %v", err)
		}
		if !exists {
			t.Errorf("User %s should exist after sync", user.Username)
		}
	}

	// Verify disabled user does not exist
	exists, err := setup.GetManager().UserExists("disabled_user")
	if err != nil {
		t.Fatalf("Error checking disabled user existence: %v", err)
	}
	if exists {
		t.Error("Disabled user should not exist after sync")
	}
}

func verifyUserMemberships(t *testing.T, setup DatabaseTestSetup) {
	userInfo, err := setup.GetManager().GetUserInfo("app_user")
	if err != nil {
		t.Fatalf("Failed to get user info: %v", err)
	}

	found := false
	for _, group := range userInfo.Groups {
		if group == "app_group" {
			found = true
			break
		}
	}
	if !found {
		t.Error("app_user should be member of app_group")
	}
}

func TestSyncConfigurationWithErrors(t *testing.T) {
	setup := SetupFlexibleTestDatabase(t)
	defer setup.Cleanup(t)
	defer setup.ResetDatabase(t)

	// Create a configuration with an invalid group name to trigger an error
	config := &structs.Config{
		Groups: []structs.GroupConfig{
			{
				Name:        "valid_group",
				Privileges:  []string{"CONNECT"},
				Databases:   []string{"testdb"},
				Description: "Valid group",
				Inherit:     true,
			},
		},
		Users: []structs.UserConfig{
			{
				Username:   "test_user",
				Password:   "test_pass",
				Groups:     []string{"non_existent_group"}, // This will cause an error
				Privileges: []string{"CONNECT"},
				Databases:  []string{"testdb"},
				Enabled:    true,
				AuthMethod: "password",
				CanLogin:   true,
			},
		},
	}

	// Sync the configuration
	result, err := setup.Manager.SyncConfiguration(config)
	if err != nil {
		t.Fatalf("Failed to sync configuration: %v", err)
	}

	// Should have some errors due to the non-existent group
	if len(result.Errors) == 0 {
		t.Error("Expected some errors during sync due to non-existent group")
	}

	// But should still create the valid group
	if len(result.GroupsCreated) != 1 {
		t.Errorf("Expected 1 group to be created, got %d", len(result.GroupsCreated))
	}
}

func TestDryRunMode(t *testing.T) {
	setup := SetupFlexibleTestDatabase(t)
	defer setup.Cleanup(t)

	// Create a dry-run manager
	dryRunManager, err := NewManager(setup.ConnInfo, setup.Logger, true)
	if err != nil {
		t.Fatalf("Failed to create dry-run manager: %v", err)
	}
	defer dryRunManager.Close()

	// Try to create a user in dry-run mode
	userConfig := &structs.UserConfig{
		Username:   "dry_run_user",
		Password:   "test_pass",
		AuthMethod: "password",
		CanLogin:   true,
		Enabled:    true,
	}

	err = dryRunManager.CreateUser(userConfig)
	if err != nil {
		t.Fatalf("Dry-run CreateUser should not error: %v", err)
	}

	// Verify user was not actually created
	exists, err := setup.Manager.UserExists("dry_run_user")
	if err != nil {
		t.Fatalf("Error checking user existence: %v", err)
	}
	if exists {
		t.Fatal("User should not exist after dry-run operation")
	}

	// Try to create a group in dry-run mode
	groupConfig := &structs.GroupConfig{
		Name:    "dry_run_group",
		Inherit: true,
	}

	err = dryRunManager.CreateGroup(groupConfig)
	if err != nil {
		t.Fatalf("Dry-run CreateGroup should not error: %v", err)
	}

	// Verify group was not actually created
	exists, err = setup.Manager.GroupExists("dry_run_group")
	if err != nil {
		t.Fatalf("Error checking group existence: %v", err)
	}
	if exists {
		t.Fatal("Group should not exist after dry-run operation")
	}
}

func TestApplyPermissionsEndToEnd(t *testing.T) {
	setup := SetupFlexibleTestDatabase(t)
	defer setup.Cleanup(t)
	defer setup.ResetDatabase(t)

	mustExec := func(query string) {
		t.Helper()
		if _, err := setup.Manager.db.Exec(query); err != nil {
			t.Fatalf("exec %q: %v", query, err)
		}
	}
	hasPrivilege := func(query string, args ...any) bool {
		t.Helper()
		var ok bool
		if err := setup.Manager.db.QueryRow(query, args...).Scan(&ok); err != nil {
			t.Fatalf("query %q: %v", query, err)
		}
		return ok
	}

	// Objects are dropped before the roles (defers run last-in first-out) so
	// ResetDatabase can drop roles that were granted privileges.
	defer func() {
		if _, err := setup.Manager.db.Exec("DROP SCHEMA IF EXISTS perm_app CASCADE"); err != nil {
			t.Logf("drop schema perm_app: %v", err)
		}
	}()
	mustExec("CREATE SCHEMA perm_app")
	mustExec("CREATE TABLE perm_app.orders (id serial PRIMARY KEY, note text)")
	mustExec("CREATE TABLE perm_app.items (id int)")
	mustExec("CREATE SEQUENCE perm_app.extra_seq")

	if err := setup.Manager.CreateUser(&structs.UserConfig{
		Username:   "test_user",
		Password:   "test_pass",
		AuthMethod: "password",
		CanLogin:   true,
		Enabled:    true,
	}); err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Apply schema, table and sequence permissions.
	err := setup.Manager.ApplyPermissions("test_user", []string{
		"grant usage on schema perm_app to test_user",
		"GRANT SELECT, INSERT ON TABLE perm_app.orders TO test_user",
		"GRANT USAGE ON SEQUENCE perm_app.orders_id_seq TO test_user",
		"GRANT SELECT ON ALL TABLES IN SCHEMA perm_app TO test_user",
	})
	if err != nil {
		t.Fatalf("ApplyPermissions failed: %v", err)
	}

	checks := []struct {
		name string
		sql  string
		args []any
		want bool
	}{
		{"schema usage", "SELECT has_schema_privilege('test_user', 'perm_app', 'USAGE')", nil, true},
		{"orders select", "SELECT has_table_privilege('test_user', 'perm_app.orders', 'SELECT')", nil, true},
		{"orders insert", "SELECT has_table_privilege('test_user', 'perm_app.orders', 'INSERT')", nil, true},
		{"orders update not granted", "SELECT has_table_privilege('test_user', 'perm_app.orders', 'UPDATE')", nil, false},
		{"items select via all tables", "SELECT has_table_privilege('test_user', 'perm_app.items', 'SELECT')", nil, true},
		{"sequence usage", "SELECT has_sequence_privilege('test_user', 'perm_app.orders_id_seq', 'USAGE')", nil, true},
	}
	for _, c := range checks {
		if got := hasPrivilege(c.sql, c.args...); got != c.want {
			t.Errorf("%s = %v, want %v", c.name, got, c.want)
		}
	}

	// Applying the same permissions again is idempotent.
	if err := setup.Manager.ApplyPermissions("test_user", []string{"GRANT SELECT, INSERT ON TABLE perm_app.orders TO test_user"}); err != nil {
		t.Fatalf("re-applying permissions failed: %v", err)
	}

	// Revoke one privilege through Rule.Inverse().
	rule, err := permissions.Validate("GRANT INSERT ON TABLE perm_app.orders TO test_user", "test_user", permissions.ActionGrant)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	revokeSQL, err := rule.Inverse().SQL()
	if err != nil {
		t.Fatalf("Inverse().SQL() failed: %v", err)
	}
	mustExec(revokeSQL)

	if hasPrivilege("SELECT has_table_privilege('test_user', 'perm_app.orders', 'INSERT')") {
		t.Error("INSERT should be revoked")
	}
	if !hasPrivilege("SELECT has_table_privilege('test_user', 'perm_app.orders', 'SELECT')") {
		t.Error("SELECT should be unaffected by the revoke")
	}

	// A rejected statement leaves no change, even when earlier statements in the
	// same call are valid.
	err = setup.Manager.ApplyPermissions("test_user", []string{
		"GRANT USAGE ON SEQUENCE perm_app.extra_seq TO test_user",
		"GRANT SELECT ON TABLE perm_app.orders TO PUBLIC",
	})
	if !errors.Is(err, permissions.ErrUnsupportedOption) {
		t.Fatalf("expected ErrUnsupportedOption, got %v", err)
	}
	if hasPrivilege("SELECT has_sequence_privilege('test_user', 'perm_app.extra_seq', 'USAGE')") {
		t.Error("valid statement before a rejected one must not be applied")
	}
	if hasPrivilege("SELECT has_table_privilege('public', 'perm_app.orders', 'SELECT')") {
		t.Error("PUBLIC must not have been granted SELECT")
	}

	// REVOKE through the declarative path is rejected and leaves the grant in place.
	err = setup.Manager.ApplyPermissions("test_user", []string{"REVOKE SELECT ON TABLE perm_app.orders FROM test_user"})
	if !errors.Is(err, permissions.ErrActionNotAllowed) {
		t.Fatalf("expected ErrActionNotAllowed, got %v", err)
	}
	if !hasPrivilege("SELECT has_table_privilege('test_user', 'perm_app.orders', 'SELECT')") {
		t.Error("rejected REVOKE must not change privileges")
	}

	// Granting to another role is rejected.
	err = setup.Manager.ApplyPermissions("test_user", []string{"GRANT USAGE ON SCHEMA perm_app TO test_group"})
	if !errors.Is(err, permissions.ErrGranteeMismatch) {
		t.Fatalf("expected ErrGranteeMismatch, got %v", err)
	}

	// ALL canonicalizes to ALL (not an explicit list) and is interpreted by the
	// server: on PostgreSQL 15 it covers the seven table privileges and has no
	// MAINTAIN, which only exists on PostgreSQL 17+.
	defer func() {
		if _, err := setup.Manager.db.Exec("DROP SCHEMA IF EXISTS perm_all CASCADE"); err != nil {
			t.Logf("drop schema perm_all: %v", err)
		}
	}()
	mustExec("CREATE SCHEMA perm_all")
	mustExec("CREATE TABLE perm_all.single_t (id int)")
	mustExec("CREATE TABLE perm_all.t1 (id int)")
	mustExec("CREATE TABLE perm_all.t2 (id int)")

	pg15TablePrivileges := []string{"SELECT", "INSERT", "UPDATE", "DELETE", "TRUNCATE", "REFERENCES", "TRIGGER"}
	assertTablePrivileges := func(table string, want bool) {
		t.Helper()
		for _, priv := range pg15TablePrivileges {
			got := hasPrivilege("SELECT has_table_privilege('test_user', $1, $2)", table, priv)
			if got != want {
				t.Errorf("%s %s = %v, want %v", table, priv, got, want)
			}
		}
	}
	assertCanonicalAll := func(stmt string) permissions.Rule {
		t.Helper()
		rule, err := permissions.Validate(stmt, "test_user", permissions.ActionGrant)
		if err != nil {
			t.Fatalf("Validate(%q): %v", stmt, err)
		}
		canonical, err := rule.SQL()
		if err != nil {
			t.Fatalf("SQL(%q): %v", stmt, err)
		}
		if !strings.Contains(canonical, " ALL ") || strings.Contains(strings.ToUpper(canonical), "MAINTAIN") {
			t.Fatalf("canonical SQL %q should use ALL and not list MAINTAIN", canonical)
		}
		return rule
	}
	revokeInverse := func(rule permissions.Rule) {
		t.Helper()
		revokeSQL, err := rule.Inverse().SQL()
		if err != nil {
			t.Fatalf("Inverse().SQL(): %v", err)
		}
		if !strings.HasPrefix(revokeSQL, "REVOKE ALL ") {
			t.Fatalf("expected REVOKE ALL, got %q", revokeSQL)
		}
		mustExec(revokeSQL)
	}

	// GRANT ALL ON TABLE, then undo through Inverse().
	tableAll := "GRANT ALL ON TABLE perm_all.single_t TO test_user"
	tableRule := assertCanonicalAll(tableAll)
	assertTablePrivileges("perm_all.single_t", false)
	if err := setup.Manager.ApplyPermissions("test_user", []string{tableAll}); err != nil {
		t.Fatalf("ApplyPermissions(%q) failed: %v", tableAll, err)
	}
	assertTablePrivileges("perm_all.single_t", true)
	revokeInverse(tableRule)
	assertTablePrivileges("perm_all.single_t", false)

	// GRANT ALL ON ALL TABLES IN SCHEMA, then undo through Inverse().
	schemaAll := "GRANT ALL ON ALL TABLES IN SCHEMA perm_all TO test_user"
	schemaRule := assertCanonicalAll(schemaAll)
	if err := setup.Manager.ApplyPermissions("test_user", []string{schemaAll}); err != nil {
		t.Fatalf("ApplyPermissions(%q) failed: %v", schemaAll, err)
	}
	for _, table := range []string{"perm_all.single_t", "perm_all.t1", "perm_all.t2"} {
		assertTablePrivileges(table, true)
	}
	revokeInverse(schemaRule)
	for _, table := range []string{"perm_all.single_t", "perm_all.t1", "perm_all.t2"} {
		assertTablePrivileges(table, false)
	}
}

func TestGrantPrivilegesRejectsInjection(t *testing.T) {
	setup := SetupFlexibleTestDatabase(t)
	defer setup.Cleanup(t)
	defer setup.ResetDatabase(t)

	if err := setup.Manager.CreateUser(&structs.UserConfig{
		Username:   "test_user",
		Password:   "test_pass",
		AuthMethod: "password",
		CanLogin:   true,
		Enabled:    true,
	}); err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	err := setup.Manager.GrantPrivileges("test_user",
		[]string{"CONNECT ON DATABASE testdb TO test_user; CREATE ROLE injected_role; --"},
		[]string{"testdb"})
	if err == nil {
		t.Fatal("expected injection attempt to be rejected")
	}

	exists, err := setup.Manager.GroupExists("injected_role")
	if err != nil {
		t.Fatalf("GroupExists failed: %v", err)
	}
	if exists {
		t.Fatal("injected statement was executed")
	}
}

func TestSyncConfigurationAppliesPermissions(t *testing.T) {
	setup := SetupFlexibleTestDatabase(t)
	defer setup.Cleanup(t)
	defer setup.ResetDatabase(t)

	if _, err := setup.Manager.db.Exec("CREATE SCHEMA perm_sync"); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	defer func() {
		if _, err := setup.Manager.db.Exec("DROP SCHEMA IF EXISTS perm_sync CASCADE"); err != nil {
			t.Logf("drop schema perm_sync: %v", err)
		}
	}()

	result, err := setup.Manager.SyncConfiguration(&structs.Config{
		Groups: []structs.GroupConfig{{
			Name:        "test_group",
			Permissions: []string{"GRANT USAGE ON SCHEMA perm_sync TO test_group"},
			Inherit:     true,
		}},
		Users: []structs.UserConfig{{
			Username:    "test_user",
			Password:    "test_pass",
			AuthMethod:  "password",
			CanLogin:    true,
			Enabled:     true,
			Permissions: []string{"GRANT CREATE ON SCHEMA perm_sync TO test_user"},
		}},
	})
	if err != nil {
		t.Fatalf("SyncConfiguration failed: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected sync errors: %v", result.Errors)
	}

	var groupUsage, userCreate bool
	if err := setup.Manager.db.QueryRow("SELECT has_schema_privilege('test_group', 'perm_sync', 'USAGE')").Scan(&groupUsage); err != nil {
		t.Fatal(err)
	}
	if err := setup.Manager.db.QueryRow("SELECT has_schema_privilege('test_user', 'perm_sync', 'CREATE')").Scan(&userCreate); err != nil {
		t.Fatal(err)
	}
	if !groupUsage || !userCreate {
		t.Errorf("permissions not applied: group usage=%v user create=%v", groupUsage, userCreate)
	}
}
