package database

import (
	"testing"

	"github.com/ben-vaughan-nttd/postgres-user-manager/internal/structs"
)

func TestNewManager(t *testing.T) {
	setup := SetupFlexibleTestDatabase(t)
	defer setup.Cleanup(t)

	// Test successful connection
	if setup.Manager == nil {
		t.Fatal("Expected non-nil manager")
	}

	// Test connection properties
	if setup.Manager.db == nil {
		t.Fatal("Expected non-nil database connection")
	}

	if setup.Manager.logger == nil {
		t.Fatal("Expected non-nil logger")
	}
}

func TestNewManagerWithInvalidConnection(t *testing.T) {
	setup := SetupFlexibleTestDatabase(t)
	defer setup.Cleanup(t)

	// Test with invalid connection details
	invalidConn := &structs.DatabaseConnection{
		Host:     "invalid-host",
		Port:     5432,
		Database: "invalid-db",
		Username: "invalid-user",
		Password: "invalid-pass",
		SSLMode:  "disable",
		IAMAuth:  false,
	}

	_, err := NewManager(invalidConn, setup.Logger, false)
	if err == nil {
		t.Fatal("Expected error with invalid connection details")
	}
}

func TestNewManagerDryRun(t *testing.T) {
	setup := SetupFlexibleTestDatabase(t)
	defer setup.Cleanup(t)

	// Create a dry-run manager
	dryRunManager, err := NewManager(setup.ConnInfo, setup.Logger, true)
	if err != nil {
		t.Fatalf("Failed to create dry-run manager: %v", err)
	}
	defer dryRunManager.Close()

	if !dryRunManager.dryRun {
		t.Fatal("Expected dry-run mode to be enabled")
	}
}

func TestUserExists(t *testing.T) {
	setup := SetupFlexibleTestDatabase(t)
	defer setup.Cleanup(t)
	defer setup.ResetDatabase(t)

	// Test with non-existent user
	exists, err := setup.Manager.UserExists("non_existent_user")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if exists {
		t.Fatal("Expected user to not exist")
	}

	// Create a test user first
	userConfig := &structs.UserConfig{
		Username:        "test_user",
		Password:        "test_pass",
		AuthMethod:      "password",
		CanLogin:        true,
		ConnectionLimit: 10,
		Enabled:         true,
	}

	err = setup.Manager.CreateUser(userConfig)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Test with existing user
	exists, err = setup.Manager.UserExists("test_user")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !exists {
		t.Fatal("Expected user to exist")
	}
}

func TestCreateUser(t *testing.T) {
	setup := SetupFlexibleTestDatabase(t)
	defer setup.Cleanup(t)
	defer setup.ResetDatabase(t)

	tests := []struct {
		name       string
		userConfig *structs.UserConfig
		expectErr  bool
	}{
		{
			name: "Create user with password auth",
			userConfig: &structs.UserConfig{
				Username:        "test_user",
				Password:        "test_pass",
				AuthMethod:      "password",
				CanLogin:        true,
				ConnectionLimit: 10,
				Enabled:         true,
			},
			expectErr: false,
		},
		{
			name: "Create user with IAM auth",
			userConfig: &structs.UserConfig{
				Username:   "iam_user",
				AuthMethod: "iam",
				CanLogin:   true,
				Enabled:    true,
			},
			expectErr: false,
		},
		{
			name: "Create user with no login",
			userConfig: &structs.UserConfig{
				Username:   "nologin_user",
				AuthMethod: "password",
				Password:   "test_pass",
				CanLogin:   false,
				Enabled:    true,
			},
			expectErr: false,
		},
		{
			name: "Create user with connection limit",
			userConfig: &structs.UserConfig{
				Username:        "limited_user",
				Password:        "test_pass",
				AuthMethod:      "password",
				CanLogin:        true,
				ConnectionLimit: 5,
				Enabled:         true,
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := setup.Manager.CreateUser(tt.userConfig)
			if (err != nil) != tt.expectErr {
				t.Errorf("CreateUser() error = %v, expectErr %v", err, tt.expectErr)
				return
			}

			if !tt.expectErr {
				// Verify user was created
				exists, err := setup.Manager.UserExists(tt.userConfig.Username)
				if err != nil {
					t.Fatalf("Error checking user existence: %v", err)
				}
				if !exists {
					t.Fatalf("User %s should exist after creation", tt.userConfig.Username)
				}
			}
		})
	}
}

func TestCreateUserDuplicate(t *testing.T) {
	setup := SetupFlexibleTestDatabase(t)
	defer setup.Cleanup(t)
	defer setup.ResetDatabase(t)

	userConfig := &structs.UserConfig{
		Username:   "test_user",
		Password:   "test_pass",
		AuthMethod: "password",
		CanLogin:   true,
		Enabled:    true,
	}

	// Create user first time
	err := setup.Manager.CreateUser(userConfig)
	if err != nil {
		t.Fatalf("Failed to create user first time: %v", err)
	}

	// Try to create same user again - should not error
	err = setup.Manager.CreateUser(userConfig)
	if err != nil {
		t.Fatalf("Creating duplicate user should not error: %v", err)
	}
}

func TestDropUser(t *testing.T) {
	setup := SetupFlexibleTestDatabase(t)
	defer setup.Cleanup(t)
	defer setup.ResetDatabase(t)

	// Create a test user first
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

	// Verify user exists
	exists, err := setup.Manager.UserExists("test_user")
	if err != nil {
		t.Fatalf("Error checking user existence: %v", err)
	}
	if !exists {
		t.Fatal("User should exist before dropping")
	}

	// Drop the user
	err = setup.Manager.DropUser("test_user")
	if err != nil {
		t.Fatalf("Failed to drop user: %v", err)
	}

	// Verify user no longer exists
	exists, err = setup.Manager.UserExists("test_user")
	if err != nil {
		t.Fatalf("Error checking user existence after drop: %v", err)
	}
	if exists {
		t.Fatal("User should not exist after dropping")
	}
}

func TestDropNonExistentUser(t *testing.T) {
	setup := SetupFlexibleTestDatabase(t)
	defer setup.Cleanup(t)

	// Try to drop a user that doesn't exist - should not error
	err := setup.Manager.DropUser("non_existent_user")
	if err != nil {
		t.Fatalf("Dropping non-existent user should not error: %v", err)
	}
}
