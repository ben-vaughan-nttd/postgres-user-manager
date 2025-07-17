package database

import (
	"testing"

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
