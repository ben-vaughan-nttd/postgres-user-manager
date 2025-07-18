package database

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/ben-vaughan-nttd/postgres-user-manager/internal/structs"
	"github.com/sirupsen/logrus"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// SimpleDatabaseSetup is an alternative setup that uses environment variables if available
type SimpleDatabaseSetup struct {
	Container testcontainers.Container
	Manager   *Manager
	ConnInfo  *structs.DatabaseConnection
	Logger    *logrus.Logger
	UseLocal  bool
}

// SetupSimpleTestDatabase creates a test database setup with fallback to local PostgreSQL
func SetupSimpleTestDatabase(t *testing.T) *SimpleDatabaseSetup {
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	// Check if we should use local PostgreSQL instead of containers
	if os.Getenv("USE_LOCAL_POSTGRES") == "true" {
		return setupLocalDatabase(t, logger)
	}

	// Try to use testcontainers, but with simpler configuration
	container, err := postgres.Run(ctx,
		"postgres:15-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(1).
				WithStartupTimeout(2*time.Minute)),
	)

	if err != nil {
		t.Logf("Failed to start container: %v", err)
		t.Skip("Skipping test - Docker/testcontainers not available")
		return nil
	}

	host, err := container.Host(ctx)
	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("Failed to get container host: %v", err)
	}

	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("Failed to get container port: %v", err)
	}

	connInfo := &structs.DatabaseConnection{
		Host:     host,
		Port:     port.Int(),
		Database: "testdb",
		Username: "testuser",
		Password: "testpass",
		SSLMode:  "disable",
		IAMAuth:  false,
	}

	manager, err := NewManager(connInfo, logger, false)
	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("Failed to create database manager: %v", err)
	}

	return &SimpleDatabaseSetup{
		Container: container,
		Manager:   manager,
		ConnInfo:  connInfo,
		Logger:    logger,
		UseLocal:  false,
	}
}

// setupLocalDatabase sets up a connection to a local PostgreSQL instance
func setupLocalDatabase(t *testing.T, logger *logrus.Logger) *SimpleDatabaseSetup {
	host := getEnvWithDefault("POSTGRES_HOST", "localhost")
	portStr := getEnvWithDefault("POSTGRES_PORT", "5432")
	user := getEnvWithDefault("POSTGRES_USER", "testuser")
	password := getEnvWithDefault("POSTGRES_PASSWORD", "testpass")
	database := getEnvWithDefault("POSTGRES_DB", "testdb")

	// Convert port string to int
	port := 5432
	if p, err := strconv.Atoi(portStr); err == nil {
		port = p
	}

	connInfo := &structs.DatabaseConnection{
		Host:     host,
		Port:     port,
		Database: database,
		Username: user,
		Password: password,
		SSLMode:  "disable",
		IAMAuth:  false,
	}

	manager, err := NewManager(connInfo, logger, false)
	if err != nil {
		t.Skipf("Failed to connect to local PostgreSQL: %v", err)
		return nil
	}

	return &SimpleDatabaseSetup{
		Container: nil,
		Manager:   manager,
		ConnInfo:  connInfo,
		Logger:    logger,
		UseLocal:  true,
	}
}

// Cleanup cleans up the test database setup
func (sds *SimpleDatabaseSetup) Cleanup(t *testing.T) {
	ctx := context.Background()

	if sds.Manager != nil {
		if err := sds.Manager.Close(); err != nil {
			t.Logf("Error closing database manager: %v", err)
		}
	}

	if sds.Container != nil && !sds.UseLocal {
		if err := sds.Container.Terminate(ctx); err != nil {
			t.Logf("Error terminating container: %v", err)
		}
	}
}

// ResetDatabase cleans up test data
func (sds *SimpleDatabaseSetup) ResetDatabase(t *testing.T) {
	testUsers := []string{"test_user", "test_user_2", "iam_user", "nologin_user", "limited_user"}
	testRoles := []string{"test_group", "test_role", "app_group", "read_only"}

	// Clean up users
	for _, user := range testUsers {
		if exists, err := sds.Manager.UserExists(user); err == nil && exists {
			if err := sds.Manager.DropUser(user); err != nil {
				t.Logf("Error dropping test user %s: %v", user, err)
			}
		}
	}

	// Clean up roles
	for _, role := range testRoles {
		if exists, err := sds.Manager.GroupExists(role); err == nil && exists {
			query := "DROP ROLE IF EXISTS " + sds.Manager.quoteIdentifier(role)
			if _, err := sds.Manager.db.Exec(query); err != nil {
				t.Logf("Error dropping test role %s: %v", role, err)
			}
		}
	}
}

// CreateTestDatabase creates a test database
func (sds *SimpleDatabaseSetup) CreateTestDatabase(t *testing.T, dbName string) {
	query := "CREATE DATABASE " + sds.Manager.quoteIdentifier(dbName)
	if _, err := sds.Manager.db.Exec(query); err != nil {
		t.Logf("Error creating test database %s (might already exist): %v", dbName, err)
	}
}

// DropTestDatabase drops a test database
func (sds *SimpleDatabaseSetup) DropTestDatabase(t *testing.T, dbName string) {
	// Terminate connections first
	query := "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1"
	sds.Manager.db.Exec(query, dbName)

	query = "DROP DATABASE IF EXISTS " + sds.Manager.quoteIdentifier(dbName)
	if _, err := sds.Manager.db.Exec(query); err != nil {
		t.Logf("Error dropping test database %s: %v", dbName, err)
	}
}

// getEnvWithDefault gets an environment variable or returns a default value
func getEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// TestDatabaseConnection is a simple connection test that doesn't require containers
func TestDatabaseConnection(t *testing.T) {
	if os.Getenv("USE_LOCAL_POSTGRES") != "true" {
		t.Skip("Skipping connection test - set USE_LOCAL_POSTGRES=true to enable")
	}

	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	connInfo := &structs.DatabaseConnection{
		Host:     getEnvWithDefault("POSTGRES_HOST", "localhost"),
		Port:     5432,
		Database: getEnvWithDefault("POSTGRES_DB", "testdb"),
		Username: getEnvWithDefault("POSTGRES_USER", "testuser"),
		Password: getEnvWithDefault("POSTGRES_PASSWORD", "testpass"),
		SSLMode:  "disable",
		IAMAuth:  false,
	}

	manager, err := NewManager(connInfo, logger, false)
	if err != nil {
		t.Fatalf("Failed to create database manager: %v", err)
	}
	defer manager.Close()

	// Test basic query
	var result int
	err = manager.db.QueryRow("SELECT 1").Scan(&result)
	if err != nil {
		t.Fatalf("Failed to execute test query: %v", err)
	}

	if result != 1 {
		t.Fatalf("Expected 1, got %d", result)
	}

	t.Log("Database connection test passed")
}
