package database

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ben-vaughan-nttd/postgres-user-manager/internal/structs"
	"github.com/sirupsen/logrus"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestDatabaseSetup holds the shared test database setup
type TestDatabaseSetup struct {
	Container testcontainers.Container
	Manager   *Manager
	ConnInfo  *structs.DatabaseConnection
	Logger    *logrus.Logger
}

// SetupTestDatabase creates a shared PostgreSQL container for tests
func SetupTestDatabase(t *testing.T) *TestDatabaseSetup {
	ctx := context.Background()

	// Create logger with reduced verbosity for tests
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	// Create PostgreSQL container
	postgresContainer, err := postgres.Run(ctx,
		"postgres:15-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(5*time.Minute)),
	)
	if err != nil {
		t.Fatalf("Failed to start PostgreSQL container: %v", err)
	}

	// Get connection details
	host, err := postgresContainer.Host(ctx)
	if err != nil {
		t.Fatalf("Failed to get container host: %v", err)
	}

	port, err := postgresContainer.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatalf("Failed to get container port: %v", err)
	}

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

	// Create database manager
	manager, err := NewManager(connInfo, logger, false)
	if err != nil {
		postgresContainer.Terminate(ctx)
		t.Fatalf("Failed to create database manager: %v", err)
	}

	return &TestDatabaseSetup{
		Container: postgresContainer,
		Manager:   manager,
		ConnInfo:  connInfo,
		Logger:    logger,
	}
}

// GetManager returns the database manager (implements DatabaseTestSetup interface)
func (tds *TestDatabaseSetup) GetManager() *Manager {
	return tds.Manager
}

// Cleanup terminates the test container and closes connections
func (tds *TestDatabaseSetup) Cleanup(t *testing.T) {
	ctx := context.Background()
	
	if tds.Manager != nil {
		if err := tds.Manager.Close(); err != nil {
			t.Logf("Error closing database manager: %v", err)
		}
	}
	
	if tds.Container != nil {
		if err := tds.Container.Terminate(ctx); err != nil {
			t.Logf("Error terminating container: %v", err)
		}
	}
}

// ResetDatabase cleans up any test data from the database
func (tds *TestDatabaseSetup) ResetDatabase(t *testing.T) {
	tds.dropTestUsers(t)
	tds.dropTestRoles(t)
}

// dropTestUsers removes test users from the database
func (tds *TestDatabaseSetup) dropTestUsers(t *testing.T) {
	testUsers := []string{"test_user", "test_user_2", "iam_user", "nologin_user", "limited_user"}
	
	for _, user := range testUsers {
		exists, err := tds.Manager.UserExists(user)
		if err != nil {
			t.Logf("Error checking if user %s exists: %v", user, err)
			continue
		}
		if exists {
			if err := tds.Manager.DropUser(user); err != nil {
				t.Logf("Error dropping test user %s: %v", user, err)
			}
		}
	}
}

// dropTestRoles removes test roles from the database
func (tds *TestDatabaseSetup) dropTestRoles(t *testing.T) {
	testRoles := []string{"test_group", "test_role", "app_group", "read_only"}

	for _, role := range testRoles {
		exists, err := tds.Manager.GroupExists(role)
		if err != nil {
			t.Logf("Error checking if role %s exists: %v", role, err)
			continue
		}
		if exists {
			// Drop role using direct SQL since we don't have a DropGroup method
			query := fmt.Sprintf("DROP ROLE IF EXISTS %s", tds.Manager.quoteIdentifier(role))
			if _, err := tds.Manager.db.Exec(query); err != nil {
				t.Logf("Error dropping test role %s: %v", role, err)
			}
		}
	}
}

// CreateTestDatabase creates a test database for privilege testing
func (tds *TestDatabaseSetup) CreateTestDatabase(t *testing.T, dbName string) {
	query := fmt.Sprintf("CREATE DATABASE %s", tds.Manager.quoteIdentifier(dbName))
	if _, err := tds.Manager.db.Exec(query); err != nil {
		t.Logf("Error creating test database %s (might already exist): %v", dbName, err)
	}
}

// DropTestDatabase drops a test database
func (tds *TestDatabaseSetup) DropTestDatabase(t *testing.T, dbName string) {
	// Terminate connections to the database first
	query := fmt.Sprintf("SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s'", dbName)
	tds.Manager.db.Exec(query)
	
	query = fmt.Sprintf("DROP DATABASE IF EXISTS %s", tds.Manager.quoteIdentifier(dbName))
	if _, err := tds.Manager.db.Exec(query); err != nil {
		t.Logf("Error dropping test database %s: %v", dbName, err)
	}
}
