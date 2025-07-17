package database

import (
	"testing"
)

// TestFlexibleSetup validates that our flexible test setup works
func TestFlexibleSetup(t *testing.T) {
	setup := SetupFlexibleTestDatabase(t)
	defer setup.Cleanup(t)

	// Test that we can connect and perform basic operations
	if setup.Manager == nil {
		t.Fatal("Expected non-nil manager")
	}

	if setup.Manager.db == nil {
		t.Fatal("Expected non-nil database connection")
	}

	// Test that we can actually ping the database
	if err := setup.Manager.db.Ping(); err != nil {
		t.Fatalf("Failed to ping database: %v", err)
	}

	// Test basic database operation that requires a working connection
	exists, err := setup.Manager.UserExists("nonexistent_user")
	if err != nil {
		t.Fatalf("Failed to check user existence: %v", err)
	}
	if exists {
		t.Error("Expected nonexistent user to not exist")
	}

	t.Log("Database connection successful!")
}

// TestDockerEnvironmentDetection tests our Docker environment detection
func TestDockerEnvironmentDetection(t *testing.T) {
	env := detectDockerEnvironment()
	
	t.Logf("Detected Docker environment: Type=%s, SocketPath=%s", env.Type, env.SocketPath)
	
	// Ensure we get a valid environment type
	validTypes := map[string]bool{
		"colima":         true,
		"docker-desktop": true,
		"lima":           true,
		"podman":         true,
		"unknown":        true,
	}
	
	if !validTypes[env.Type] {
		t.Errorf("Invalid environment type detected: %s", env.Type)
	}
}
