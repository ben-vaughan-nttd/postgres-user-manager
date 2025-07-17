package database

import (
	"testing"

	"github.com/ben-vaughan-nttd/postgres-user-manager/internal/structs"
)

func TestGroupExists(t *testing.T) {
	setup := SetupFlexibleTestDatabase(t)
	defer setup.Cleanup(t)
	defer setup.ResetDatabase(t)

	// Test with non-existent group
	exists, err := setup.Manager.GroupExists("non_existent_group")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if exists {
		t.Fatal("Expected group to not exist")
	}

	// Create a test group first
	groupConfig := &structs.GroupConfig{
		Name:    "test_group",
		Inherit: true,
	}

	err = setup.Manager.CreateGroup(groupConfig)
	if err != nil {
		t.Fatalf("Failed to create test group: %v", err)
	}

	// Test with existing group
	exists, err = setup.Manager.GroupExists("test_group")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !exists {
		t.Fatal("Expected group to exist")
	}
}

func TestCreateGroup(t *testing.T) {
	setup := SetupFlexibleTestDatabase(t)
	defer setup.Cleanup(t)
	defer setup.ResetDatabase(t)

	tests := []struct {
		name        string
		groupConfig *structs.GroupConfig
		expectErr   bool
	}{
		{
			name: "Create group with inherit",
			groupConfig: &structs.GroupConfig{
				Name:        "test_group",
				Description: "Test group with inherit",
				Inherit:     true,
			},
			expectErr: false,
		},
		{
			name: "Create group without inherit",
			groupConfig: &structs.GroupConfig{
				Name:        "test_role",
				Description: "Test role without inherit",
				Inherit:     false,
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := setup.Manager.CreateGroup(tt.groupConfig)
			if (err != nil) != tt.expectErr {
				t.Errorf("CreateGroup() error = %v, expectErr %v", err, tt.expectErr)
				return
			}

			if !tt.expectErr {
				// Verify group was created
				exists, err := setup.Manager.GroupExists(tt.groupConfig.Name)
				if err != nil {
					t.Fatalf("Error checking group existence: %v", err)
				}
				if !exists {
					t.Fatalf("Group %s should exist after creation", tt.groupConfig.Name)
				}
			}
		})
	}
}

func TestCreateGroupDuplicate(t *testing.T) {
	setup := SetupFlexibleTestDatabase(t)
	defer setup.Cleanup(t)
	defer setup.ResetDatabase(t)

	groupConfig := &structs.GroupConfig{
		Name:    "test_group",
		Inherit: true,
	}

	// Create group first time
	err := setup.Manager.CreateGroup(groupConfig)
	if err != nil {
		t.Fatalf("Failed to create group first time: %v", err)
	}

	// Try to create same group again - should not error
	err = setup.Manager.CreateGroup(groupConfig)
	if err != nil {
		t.Fatalf("Creating duplicate group should not error: %v", err)
	}
}

func TestAddUserToGroup(t *testing.T) {
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

	// Create a test group
	groupConfig := &structs.GroupConfig{
		Name:    "test_group",
		Inherit: true,
	}

	err = setup.Manager.CreateGroup(groupConfig)
	if err != nil {
		t.Fatalf("Failed to create test group: %v", err)
	}

	// Add user to group
	err = setup.Manager.AddUserToGroup("test_user", "test_group")
	if err != nil {
		t.Fatalf("Failed to add user to group: %v", err)
	}

	// Verify user is in group by getting user info
	userInfo, err := setup.Manager.GetUserInfo("test_user")
	if err != nil {
		t.Fatalf("Failed to get user info: %v", err)
	}

	found := false
	for _, group := range userInfo.Groups {
		if group == "test_group" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("User should be member of the group")
	}
}

func TestRemoveUserFromGroup(t *testing.T) {
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

	// Create a test group
	groupConfig := &structs.GroupConfig{
		Name:    "test_group",
		Inherit: true,
	}

	err = setup.Manager.CreateGroup(groupConfig)
	if err != nil {
		t.Fatalf("Failed to create test group: %v", err)
	}

	// Add user to group first
	err = setup.Manager.AddUserToGroup("test_user", "test_group")
	if err != nil {
		t.Fatalf("Failed to add user to group: %v", err)
	}

	// Remove user from group
	err = setup.Manager.RemoveUserFromGroup("test_user", "test_group")
	if err != nil {
		t.Fatalf("Failed to remove user from group: %v", err)
	}

	// Verify user is no longer in group
	userInfo, err := setup.Manager.GetUserInfo("test_user")
	if err != nil {
		t.Fatalf("Failed to get user info: %v", err)
	}

	for _, group := range userInfo.Groups {
		if group == "test_group" {
			t.Fatal("User should not be member of the group after removal")
		}
	}
}

func TestGetUserInfo(t *testing.T) {
	setup := SetupFlexibleTestDatabase(t)
	defer setup.Cleanup(t)
	defer setup.ResetDatabase(t)

	// Test with non-existent user
	userInfo, err := setup.Manager.GetUserInfo("non_existent_user")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if userInfo.Exists {
		t.Fatal("Expected user to not exist")
	}
	if userInfo.Username != "non_existent_user" {
		t.Fatal("Expected username to match")
	}

	// Create a test user
	userConfig := &structs.UserConfig{
		Username:   "test_user",
		Password:   "test_pass",
		AuthMethod: "password",
		CanLogin:   true,
		Enabled:    true,
	}

	err = setup.Manager.CreateUser(userConfig)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Test with existing user
	userInfo, err = setup.Manager.GetUserInfo("test_user")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !userInfo.Exists {
		t.Fatal("Expected user to exist")
	}
	if userInfo.Username != "test_user" {
		t.Fatal("Expected username to match")
	}
	if userInfo.Groups == nil {
		t.Fatal("Expected groups slice to be initialized")
	}
}
