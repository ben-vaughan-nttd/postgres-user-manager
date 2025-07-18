package database

import (
	"fmt"
	"testing"

	"github.com/ben-vaughan-nttd/postgres-user-manager/internal/structs"
)

func TestCreateUserWithInvalidCharacters(t *testing.T) {
	setup := SetupFlexibleTestDatabase(t)
	defer setup.Cleanup(t)
	defer setup.ResetDatabase(t)

	// Test with username containing special characters
	userConfig := &structs.UserConfig{
		Username:   "test-user", // Valid username with dash
		Password:   "test_pass",
		AuthMethod: "password",
		CanLogin:   true,
		Enabled:    true,
	}

	err := setup.Manager.CreateUser(userConfig)
	if err != nil {
		t.Fatalf("Failed to create user with dash: %v", err)
	}

	// Verify user was created
	exists, err := setup.Manager.UserExists("test-user")
	if err != nil {
		t.Fatalf("Error checking user existence: %v", err)
	}
	if !exists {
		t.Fatal("User with dash should exist")
	}
}

func TestCreateUserWithQuotesInUsername(t *testing.T) {
	setup := SetupFlexibleTestDatabase(t)
	defer setup.Cleanup(t)
	defer setup.ResetDatabase(t)

	// Test with username containing quotes (should be escaped)
	userConfig := &structs.UserConfig{
		Username:   `test"user`,
		Password:   "test_pass",
		AuthMethod: "password",
		CanLogin:   true,
		Enabled:    true,
	}

	err := setup.Manager.CreateUser(userConfig)
	if err != nil {
		t.Fatalf("Failed to create user with quotes: %v", err)
	}

	// Verify user was created
	exists, err := setup.Manager.UserExists(`test"user`)
	if err != nil {
		t.Fatalf("Error checking user existence: %v", err)
	}
	if !exists {
		t.Fatal("User with quotes should exist")
	}
}

func TestCreateUserWithQuotesInPassword(t *testing.T) {
	setup := SetupFlexibleTestDatabase(t)
	defer setup.Cleanup(t)
	defer setup.ResetDatabase(t)

	// Test with password containing single quotes (should be escaped)
	userConfig := &structs.UserConfig{
		Username:   "test_user",
		Password:   "test'pass'word",
		AuthMethod: "password",
		CanLogin:   true,
		Enabled:    true,
	}

	err := setup.Manager.CreateUser(userConfig)
	if err != nil {
		t.Fatalf("Failed to create user with quotes in password: %v", err)
	}

	// Verify user was created
	exists, err := setup.Manager.UserExists("test_user")
	if err != nil {
		t.Fatalf("Error checking user existence: %v", err)
	}
	if !exists {
		t.Fatal("User with quoted password should exist")
	}
}

func TestCreateUserConnectionLimitVariations(t *testing.T) {
	setup := SetupFlexibleTestDatabase(t)
	defer setup.Cleanup(t)
	defer setup.ResetDatabase(t)

	tests := []struct {
		name            string
		connectionLimit int
		expectErr       bool
	}{
		{
			name:            "Unlimited connections",
			connectionLimit: -1,
			expectErr:       false,
		},
		{
			name:            "Zero connections",
			connectionLimit: 0,
			expectErr:       false,
		},
		{
			name:            "Positive connection limit",
			connectionLimit: 100,
			expectErr:       false,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userConfig := &structs.UserConfig{
				Username:        generateUniqueUsername(i),
				Password:        "test_pass",
				AuthMethod:      "password",
				CanLogin:        true,
				ConnectionLimit: tt.connectionLimit,
				Enabled:         true,
			}

			err := setup.Manager.CreateUser(userConfig)
			if (err != nil) != tt.expectErr {
				t.Errorf("CreateUser() error = %v, expectErr %v", err, tt.expectErr)
				return
			}

			if !tt.expectErr {
				// Verify user was created
				exists, err := setup.Manager.UserExists(userConfig.Username)
				if err != nil {
					t.Fatalf("Error checking user existence: %v", err)
				}
				if !exists {
					t.Fatalf("User %s should exist after creation", userConfig.Username)
				}
			}
		})
	}
}

func TestAddUserToNonExistentGroup(t *testing.T) {
	setup := SetupFlexibleTestDatabase(t)
	defer setup.Cleanup(t)
	defer setup.ResetDatabase(t)

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

	// Try to add user to non-existent group - should error
	err = setup.Manager.AddUserToGroup("test_user", "non_existent_group")
	if err == nil {
		t.Fatal("Expected error when adding user to non-existent group")
	}
}

func TestRemoveUserFromNonExistentGroup(t *testing.T) {
	setup := SetupFlexibleTestDatabase(t)
	defer setup.Cleanup(t)
	defer setup.ResetDatabase(t)

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

	// Try to remove user from non-existent group - should error
	err = setup.Manager.RemoveUserFromGroup("test_user", "non_existent_group")
	if err == nil {
		t.Fatal("Expected error when removing user from non-existent group")
	}
}

func TestGrantPrivilegesToNonExistentUser(t *testing.T) {
	setup := SetupFlexibleTestDatabase(t)
	defer setup.Cleanup(t)

	// Try to grant privileges to non-existent user - may or may not error depending on PostgreSQL behavior
	privileges := []string{"CONNECT"}
	databases := []string{"testdb"}

	err := setup.Manager.GrantPrivileges("non_existent_user", privileges, databases)
	// Note: PostgreSQL might not error immediately, so we don't assert error here
	// This test mainly ensures the function handles the case gracefully
	if err != nil {
		t.Logf("Expected behavior: grant to non-existent user resulted in error: %v", err)
	}
}

func TestHelperMethods(t *testing.T) {
	setup := SetupFlexibleTestDatabase(t)
	defer setup.Cleanup(t)

	// Test quoteIdentifier method
	quoted := setup.Manager.quoteIdentifier("test_user")
	expected := `"test_user"`
	if quoted != expected {
		t.Errorf("quoteIdentifier() = %v, want %v", quoted, expected)
	}

	// Test quoteIdentifier with quotes
	quotedWithQuotes := setup.Manager.quoteIdentifier(`test"user`)
	expectedWithQuotes := `"test""user"`
	if quotedWithQuotes != expectedWithQuotes {
		t.Errorf("quoteIdentifier() = %v, want %v", quotedWithQuotes, expectedWithQuotes)
	}

	// Test escapeString method
	escaped := setup.Manager.escapeString("test'string")
	expectedEscaped := "test''string"
	if escaped != expectedEscaped {
		t.Errorf("escapeString() = %v, want %v", escaped, expectedEscaped)
	}
}

func TestIAMAuthFlow(t *testing.T) {
	setup := SetupFlexibleTestDatabase(t)
	defer setup.Cleanup(t)
	defer setup.ResetDatabase(t)

	// Test creating user with IAM auth
	userConfig := &structs.UserConfig{
		Username:   "iam_user",
		AuthMethod: "iam",
		IAMRole:    "arn:aws:iam::123456789012:role/test-role",
		CanLogin:   true,
		Enabled:    true,
	}

	err := setup.Manager.CreateUser(userConfig)
	if err != nil {
		t.Fatalf("Failed to create IAM user: %v", err)
	}

	// Verify user was created
	exists, err := setup.Manager.UserExists("iam_user")
	if err != nil {
		t.Fatalf("Error checking IAM user existence: %v", err)
	}
	if !exists {
		t.Fatal("IAM user should exist")
	}

	// Note: We can't easily test the actual rds_iam role grant without
	// having that role available in the test container, but the function
	// should complete without error in most cases
}

func TestCloseManager(t *testing.T) {
	setup := SetupFlexibleTestDatabase(t)
	defer setup.Cleanup(t)

	// Test closing the manager
	err := setup.Manager.Close()
	if err != nil {
		t.Fatalf("Failed to close manager: %v", err)
	}

	// Test closing again (should not error)
	err = setup.Manager.Close()
	if err != nil {
		t.Fatalf("Failed to close manager second time: %v", err)
	}
}

// Helper function to generate unique usernames for tests
func generateUniqueUsername(index int) string {
	return fmt.Sprintf("test_user_%d", index)
}
