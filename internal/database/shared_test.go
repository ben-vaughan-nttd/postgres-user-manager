package database

import (
	"testing"

	"github.com/ben-vaughan-nttd/postgres-user-manager/internal/structs"
)

func TestSharedContainerApproach(t *testing.T) {
	setup := SetupSharedTestDatabase(t)
	defer setup.Cleanup(t)

	// Test basic functionality
	userConfig := &structs.UserConfig{
		Username:   "shared_test_user",
		Password:   "test_pass",
		AuthMethod: "password",
		CanLogin:   true,
		Enabled:    true,
	}

	// Create user
	err := setup.Manager.CreateUser(userConfig)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Verify user exists
	exists, err := setup.Manager.UserExists("shared_test_user")
	if err != nil {
		t.Fatalf("Failed to check if user exists: %v", err)
	}

	if !exists {
		t.Error("User should exist after creation")
	}

	t.Log("Shared container approach test passed!")
}

func TestSharedContainerIsolation(t *testing.T) {
	setup1 := SetupSharedTestDatabase(t)
	defer setup1.Cleanup(t)

	setup2 := SetupSharedTestDatabase(t)
	defer setup2.Cleanup(t)

	// Verify we have different database names (isolation)
	if setup1.ConnInfo.Database == setup2.ConnInfo.Database {
		t.Fatalf("Expected different database names for isolation, got both: %s", setup1.ConnInfo.Database)
	}

	// Create different users in each database to test basic functionality and isolation
	userConfig1 := &structs.UserConfig{
		Username:   "isolation_user_1",
		Password:   "test_pass",
		AuthMethod: "password",
		CanLogin:   true,
		Enabled:    true,
	}

	userConfig2 := &structs.UserConfig{
		Username:   "isolation_user_2",
		Password:   "test_pass",
		AuthMethod: "password",
		CanLogin:   true,
		Enabled:    true,
	}

	// Create users in their respective database contexts
	err := setup1.Manager.CreateUser(userConfig1)
	if err != nil {
		t.Fatalf("Failed to create user in first database context: %v", err)
	}

	err = setup2.Manager.CreateUser(userConfig2)
	if err != nil {
		t.Fatalf("Failed to create user in second database context: %v", err)
	}

	// Both users should exist since PostgreSQL users are server-global, 
	// but they were created in different database contexts
	exists1, err := setup1.Manager.UserExists("isolation_user_1")
	if err != nil || !exists1 {
		t.Errorf("User isolation_user_1 should exist from context 1")
	}

	exists2, err := setup2.Manager.UserExists("isolation_user_2")
	if err != nil || !exists2 {
		t.Errorf("User isolation_user_2 should exist from context 2")
	}

	t.Logf("Database isolation test passed! Database1: %s, Database2: %s", 
		setup1.ConnInfo.Database, setup2.ConnInfo.Database)
}

func TestSharedContainerWithIAM(t *testing.T) {
	setup := SetupSharedTestDatabase(t)
	defer setup.Cleanup(t)

	// Test IAM user creation (should work now with rds_iam role)
	userConfig := &structs.UserConfig{
		Username:   "iam_test_user",
		AuthMethod: "iam",
		CanLogin:   true,
		Enabled:    true,
	}

	// Create IAM user
	err := setup.Manager.CreateUser(userConfig)
	if err != nil {
		t.Fatalf("Failed to create IAM user: %v", err)
	}

	// Verify user exists
	exists, err := setup.Manager.UserExists("iam_test_user")
	if err != nil {
		t.Fatalf("Failed to check if IAM user exists: %v", err)
	}

	if !exists {
		t.Error("IAM user should exist after creation")
	}

	t.Log("IAM user creation test passed!")
}
