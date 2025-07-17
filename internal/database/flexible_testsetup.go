package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/ben-vaughan-nttd/postgres-user-manager/internal/structs"
	"github.com/sirupsen/logrus"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	dockerSocketName   = "docker.sock"
	defaultDockerSocket = "/var/run/docker.sock"
)

// DatabaseTestSetup is a common interface for all test database setups
type DatabaseTestSetup interface {
	GetManager() *Manager
	Cleanup(*testing.T)
	ResetDatabase(*testing.T)
}

// FlexibleTestDatabaseSetup provides a test setup that adapts to different Docker environments
type FlexibleTestDatabaseSetup struct {
	Container testcontainers.Container
	Manager   *Manager
	ConnInfo  *structs.DatabaseConnection
	Logger    *logrus.Logger
}

// SetupFlexibleTestDatabase creates a PostgreSQL test database with automatic Docker environment detection
func SetupFlexibleTestDatabase(t *testing.T) *FlexibleTestDatabaseSetup {
	// Configure testcontainers for the current environment
	configureTestcontainersEnvironment(t)

	// Create a context with timeout to prevent indefinite hanging
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create logger with reduced verbosity for tests
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	// Use the postgres module with environment-specific configuration
	postgresContainer, err := postgres.Run(ctx,
		"postgres:15-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(20*time.Second)), // Reduced from 2 minutes to 20 seconds
	)
	if err != nil {
		t.Fatalf("Failed to start PostgreSQL container: %v", err)
	}

	// Reduced delay - only wait 500ms instead of 1s to minimize resource contention risk
	time.Sleep(500 * time.Millisecond)

	// Get connection details
	host, err := postgresContainer.Host(ctx)
	if err != nil {
		t.Fatalf("Failed to get container host: %v", err)
	}

	port, err := postgresContainer.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatalf("Failed to get container port: %v", err)
	}

	// Force IPv4 if host is localhost/127.0.0.1 to avoid IPv6 issues
	if host == "localhost" {
		host = "127.0.0.1"
	}

	t.Logf("Connecting to PostgreSQL at %s:%d", host, port.Int())

	// Create connection info
	connInfo := &structs.DatabaseConnection{
		Host:     host,
		Port:     port.Int(),
		Database: "testdb",
		Username: "testuser",
		Password: "testpass",
		SSLMode:  "disable",
		IAMAuth:  false,
	}

	// Create database manager with retry logic
	var manager *Manager
	var dbErr error
	maxRetries := 3  // Reduced from 5 to 3 to minimize hanging risk
	retryDelay := 1 * time.Second  // Reduced from 2s to 1s
	for i := 0; i < maxRetries; i++ {
		manager, dbErr = NewManager(connInfo, logger, false)
		if dbErr == nil {
			// Test the connection with a ping
			if pingErr := manager.db.Ping(); pingErr == nil {
				t.Logf("Database connection successful on attempt %d", i+1)
				break
			} else {
				dbErr = pingErr
				t.Logf("Database ping failed on attempt %d: %v", i+1, pingErr)
			}
		} else {
			t.Logf("Database connection attempt %d failed: %v", i+1, dbErr)
		}

		if i < maxRetries-1 {
			time.Sleep(retryDelay)
		}
	}

	if dbErr != nil {
		postgresContainer.Terminate(ctx)
		t.Fatalf("Failed to create database manager after %d attempts: %v", maxRetries, dbErr)
	}

	// Create the rds_iam role for IAM tests (simulate AWS RDS environment)
	if err := createRDSIAMRoleFlexible(manager); err != nil {
		t.Logf("Warning: Failed to create rds_iam role (this is expected for non-AWS environments): %v", err)
	}

	return &FlexibleTestDatabaseSetup{
		Container: postgresContainer,
		Manager:   manager,
		ConnInfo:  connInfo,
		Logger:    logger,
	}
}

// configureTestcontainersEnvironment detects the Docker environment and applies appropriate configuration
func configureTestcontainersEnvironment(t *testing.T) {
	// Check if ryuk is already disabled
	if os.Getenv("TESTCONTAINERS_RYUK_DISABLED") == "true" {
		t.Logf("Ryuk already disabled via environment variable")
		return
	}

	// Detect Docker environment and configure accordingly
	dockerConfig := detectDockerEnvironment()

	switch dockerConfig.Type {
	case "colima":
		t.Logf("Detected Colima Docker environment at %s", dockerConfig.SocketPath)
		// Disable ryuk for Colima due to socket path issues
		os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")

	case "docker-desktop":
		t.Logf("Detected Docker Desktop environment")
		// Docker Desktop usually works fine with ryuk, but we can disable it for consistency
		if shouldDisableRyukForDockerDesktop() {
			os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
		}

	case "lima":
		t.Logf("Detected Lima Docker environment")
		// Lima may have similar issues to Colima
		os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")

	case "podman":
		t.Logf("Detected Podman environment")
		// Podman may have compatibility issues with ryuk
		os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")

	case "unknown":
		t.Logf("Unknown Docker environment, attempting to detect ryuk compatibility")
		if !isRyukCompatible() {
			t.Logf("Ryuk appears incompatible, disabling")
			os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
		}

	default:
		t.Logf("Using default testcontainers configuration")
	}
}

// DockerEnvironment represents the detected Docker configuration
type DockerEnvironment struct {
	Type       string // colima, docker-desktop, lima, podman, unknown
	SocketPath string
}

// detectDockerEnvironment attempts to identify the Docker environment being used
func detectDockerEnvironment() DockerEnvironment {
	// Check for common Docker socket paths and environment indicators
	dockerHost := os.Getenv("DOCKER_HOST")

	// Check for Colima
	if dockerHost != "" {
		if filepath.Base(dockerHost) == dockerSocketName &&
			(containsPath(dockerHost, ".colima") || containsPath(dockerHost, "colima")) {
			return DockerEnvironment{Type: "colima", SocketPath: dockerHost}
		}
	}

	// Check for Lima
	if dockerHost != "" && containsPath(dockerHost, ".lima") {
		return DockerEnvironment{Type: "lima", SocketPath: dockerHost}
	}

	// Check for Podman
	if dockerHost != "" && containsPath(dockerHost, "podman") {
		return DockerEnvironment{Type: "podman", SocketPath: dockerHost}
	}

	// Check filesystem for Docker environments
	homeDir, _ := os.UserHomeDir()

	// Check for Colima socket
	colimaSocket := filepath.Join(homeDir, ".colima", "default", dockerSocketName)
	if fileExists(colimaSocket) {
		return DockerEnvironment{Type: "colima", SocketPath: colimaSocket}
	}

	// Check for Lima socket
	limaSocket := filepath.Join(homeDir, ".lima", "default", dockerSocketName)
	if fileExists(limaSocket) {
		return DockerEnvironment{Type: "lima", SocketPath: limaSocket}
	}

	// Check for Docker Desktop (standard locations)
	if runtime.GOOS == "darwin" {
		if fileExists(defaultDockerSocket) {
			return DockerEnvironment{Type: "docker-desktop", SocketPath: defaultDockerSocket}
		}
	}

	return DockerEnvironment{Type: "unknown", SocketPath: ""}
}

// shouldDisableRyukForDockerDesktop determines if ryuk should be disabled even for Docker Desktop
func shouldDisableRyukForDockerDesktop() bool {
	// Check if there's a preference to disable ryuk globally
	if os.Getenv("TESTCONTAINERS_PREFER_NO_RYUK") == "true" {
		return true
	}

	// For CI environments, we might want to disable ryuk for faster cleanup
	if os.Getenv("CI") == "true" {
		return true
	}

	return false
}

// isRyukCompatible performs a basic check to see if ryuk is likely to work
func isRyukCompatible() bool {
	// This is a simplified check - in practice, you might want to do more sophisticated detection
	// For now, we'll assume unknown environments might have issues
	return false
}

// containsPath checks if a path contains a specific substring with recursion limit
func containsPath(path, substring string) bool {
	// Add recursion limit to prevent infinite loops
	return containsPathWithLimit(path, substring, 10)
}

// containsPathWithLimit checks if a path contains a specific substring with depth limit
func containsPathWithLimit(path, substring string, limit int) bool {
	if limit <= 0 {
		return false
	}
	
	if filepath.Base(path) == substring {
		return true
	}
	
	dir := filepath.Dir(path)
	if dir == path || dir == "." || dir == "/" {
		// We've reached the root, stop recursion
		return false
	}
	
	return containsPathWithLimit(dir, substring, limit-1)
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Cleanup terminates the test container and closes connections
func (ftds *FlexibleTestDatabaseSetup) Cleanup(t *testing.T) {
	// Use a context with timeout for cleanup to prevent hanging
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if ftds.Manager != nil {
		if err := ftds.Manager.Close(); err != nil {
			t.Logf("Error closing database manager: %v", err)
		}
	}

	if ftds.Container != nil {
		if err := ftds.Container.Terminate(ctx); err != nil {
			t.Logf("Error terminating container: %v", err)
		}
	}

	// Clean up environment variable if we set it
	if os.Getenv("TESTCONTAINERS_RYUK_DISABLED") == "true" {
		// Only unset if we're not in a persistent environment where it should stay
		if os.Getenv("TESTCONTAINERS_PERSIST_RYUK_DISABLED") != "true" {
			os.Unsetenv("TESTCONTAINERS_RYUK_DISABLED")
		}
	}
}

// ResetDatabase cleans up any test data from the database
func (ftds *FlexibleTestDatabaseSetup) ResetDatabase(t *testing.T) {
	ftds.dropTestUsers(t)
	ftds.dropTestRoles(t)
}

// dropTestUsers removes test users from the database
func (ftds *FlexibleTestDatabaseSetup) dropTestUsers(t *testing.T) {
	testUsers := []string{"test_user", "test_user_2", "iam_user", "nologin_user", "limited_user"}

	for _, user := range testUsers {
		exists, err := ftds.Manager.UserExists(user)
		if err != nil {
			t.Logf("Error checking if user %s exists: %v", user, err)
			continue
		}
		if exists {
			if err := ftds.Manager.DropUser(user); err != nil {
				t.Logf("Error dropping test user %s: %v", user, err)
			}
		}
	}
}

// dropTestRoles removes test roles from the database
func (ftds *FlexibleTestDatabaseSetup) dropTestRoles(t *testing.T) {
	testRoles := []string{"test_group", "test_role", "app_group", "read_only"}

	for _, role := range testRoles {
		exists, err := ftds.Manager.GroupExists(role)
		if err != nil {
			t.Logf("Error checking if role %s exists: %v", role, err)
			continue
		}
		if exists {
			// Drop role using direct SQL since we don't have a DropGroup method
			query := fmt.Sprintf("DROP ROLE IF EXISTS %s", ftds.Manager.quoteIdentifier(role))
			if _, err := ftds.Manager.db.Exec(query); err != nil {
				t.Logf("Error dropping test role %s: %v", role, err)
			}
		}
	}
}

// CreateTestDatabase creates a test database for privilege testing
func (ftds *FlexibleTestDatabaseSetup) CreateTestDatabase(t *testing.T, dbName string) {
	query := fmt.Sprintf("CREATE DATABASE %s", ftds.Manager.quoteIdentifier(dbName))
	if _, err := ftds.Manager.db.Exec(query); err != nil {
		t.Logf("Error creating test database %s (might already exist): %v", dbName, err)
	}
}

// DropTestDatabase drops a test database
func (ftds *FlexibleTestDatabaseSetup) DropTestDatabase(t *testing.T, dbName string) {
	// Terminate connections to the database first
	query := fmt.Sprintf("SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s'", dbName)
	ftds.Manager.db.Exec(query)

	query = fmt.Sprintf("DROP DATABASE IF EXISTS %s", ftds.Manager.quoteIdentifier(dbName))
	if _, err := ftds.Manager.db.Exec(query); err != nil {
		t.Logf("Error dropping test database %s: %v", dbName, err)
	}
}

// GetManager returns the database manager (implements DatabaseTestSetup interface)
func (ftds *FlexibleTestDatabaseSetup) GetManager() *Manager {
	return ftds.Manager
}

// createRDSIAMRoleFlexible creates the rds_iam role for testing IAM functionality in flexible setup
func createRDSIAMRoleFlexible(manager *Manager) error {
	query := "CREATE ROLE rds_iam"
	_, err := manager.db.Exec(query)
	if err != nil && err.Error() != `pq: role "rds_iam" already exists` {
		return err
	}
	return nil
}
