package database

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ben-vaughan-nttd/postgres-user-manager/internal/structs"
	"github.com/sirupsen/logrus"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// SharedTestContainer manages a single PostgreSQL container shared across multiple tests
type SharedTestContainer struct {
	Container testcontainers.Container
	ConnInfo  *structs.DatabaseConnection
	Logger    *logrus.Logger
	mutex     sync.Mutex
	refCount  int
}

var (
	sharedContainer *SharedTestContainer
	containerMutex  sync.Mutex
)

// SharedTestDatabaseSetup provides a test setup that uses a shared container
type SharedTestDatabaseSetup struct {
	Manager  *Manager
	ConnInfo *structs.DatabaseConnection
	Logger   *logrus.Logger
	dbName   string
}

// SetupSharedTestDatabase creates or reuses a shared PostgreSQL test database
func SetupSharedTestDatabase(t *testing.T) *SharedTestDatabaseSetup {
	containerMutex.Lock()
	defer containerMutex.Unlock()

	// Create shared container if it doesn't exist
	if sharedContainer == nil {
		container, err := createSharedContainer(t)
		if err != nil {
			t.Fatalf("Failed to create shared container: %v", err)
		}
		sharedContainer = container
	}

	// Increment reference count
	sharedContainer.mutex.Lock()
	sharedContainer.refCount++
	sharedContainer.mutex.Unlock()

	// Create a unique database for this test to ensure isolation
	dbName := generateTestDBName(t)
	connInfo := &structs.DatabaseConnection{
		Host:     sharedContainer.ConnInfo.Host,
		Port:     sharedContainer.ConnInfo.Port,
		Database: dbName,
		Username: sharedContainer.ConnInfo.Username,
		Password: sharedContainer.ConnInfo.Password,
		SSLMode:  "disable",
		IAMAuth:  false,
	}

	// Create the test database
	if err := createTestDatabase(sharedContainer, dbName); err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Create database manager
	manager, err := NewManager(connInfo, sharedContainer.Logger, false)
	if err != nil {
		t.Fatalf("Failed to create database manager: %v", err)
	}

	// Create the rds_iam role for IAM tests (simulate AWS RDS environment)
	if err := createRDSIAMRole(manager); err != nil {
		t.Logf("Warning: Failed to create rds_iam role (this is expected for non-AWS environments): %v", err)
	}

	return &SharedTestDatabaseSetup{
		Manager:  manager,
		ConnInfo: connInfo,
		Logger:   sharedContainer.Logger,
		dbName:   dbName,
	}
}

// createSharedContainer creates a new shared PostgreSQL container
func createSharedContainer(t *testing.T) (*SharedTestContainer, error) {
	// Configure testcontainers for the current environment
	configureTestcontainersEnvironment(t)

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create logger with reduced verbosity
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	t.Log("Creating shared PostgreSQL container...")

	// Use the postgres module
	postgresContainer, err := postgres.Run(ctx,
		"postgres:15-alpine",
		postgres.WithDatabase("postgres"), // Use default postgres database
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second)),
	)
	if err != nil {
		return nil, err
	}

	// Get connection details
	host, err := postgresContainer.Host(ctx)
	if err != nil {
		postgresContainer.Terminate(ctx)
		return nil, err
	}

	port, err := postgresContainer.MappedPort(ctx, "5432")
	if err != nil {
		postgresContainer.Terminate(ctx)
		return nil, err
	}

	// Force IPv4 if host is localhost
	if host == "localhost" {
		host = "127.0.0.1"
	}

	t.Logf("Shared container ready at %s:%d", host, port.Int())

	connInfo := &structs.DatabaseConnection{
		Host:     host,
		Port:     port.Int(),
		Database: "postgres",
		Username: "testuser",
		Password: "testpass",
		SSLMode:  "disable",
		IAMAuth:  false,
	}

	// Wait a bit and test the connection with retry logic
	time.Sleep(500 * time.Millisecond)
	
	// Test connection with retry
	maxRetries := 3
	retryDelay := 1 * time.Second
	for i := 0; i < maxRetries; i++ {
		tempManager, err := NewManager(connInfo, logger, false)
		if err == nil {
			if pingErr := tempManager.db.Ping(); pingErr == nil {
				tempManager.Close()
				t.Logf("Shared container connection verified on attempt %d", i+1)
				break
			} else {
				tempManager.Close()
				t.Logf("Shared container ping failed on attempt %d: %v", i+1, pingErr)
			}
		} else {
			t.Logf("Shared container connection attempt %d failed: %v", i+1, err)
		}

		if i < maxRetries-1 {
			time.Sleep(retryDelay)
		} else {
			postgresContainer.Terminate(ctx)
			return nil, err
		}
	}

	return &SharedTestContainer{
		Container: postgresContainer,
		ConnInfo:  connInfo,
		Logger:    logger,
		refCount:  0,
	}, nil
}

// generateTestDBName creates a unique database name for the test
func generateTestDBName(t *testing.T) string {
	// Use simple name with timestamp and test name hash to ensure uniqueness
	testHash := sanitizeDBName(t.Name())
	if len(testHash) > 20 {
		testHash = testHash[:20]
	}
	return "testdb_" + testHash + "_" + timeStamp()
}

// timeStamp returns a simple timestamp string with nanoseconds for uniqueness
func timeStamp() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// sanitizeDBName creates a valid PostgreSQL database name from test name
func sanitizeDBName(name string) string {
	// Replace all non-alphanumeric characters with underscores
	reg := regexp.MustCompile(`[^a-zA-Z0-9]`)
	return strings.ToLower(reg.ReplaceAllString(name, "_"))
}

// createTestDatabase creates a new database for the test
func createTestDatabase(container *SharedTestContainer, dbName string) error {
	// Create a temporary manager to create the database
	tempManager, err := NewManager(container.ConnInfo, container.Logger, false)
	if err != nil {
		return err
	}
	defer tempManager.Close()

	// Create the database (dbName should be safe since we generate it)
	query := "CREATE DATABASE " + dbName
	_, err = tempManager.db.Exec(query)
	return err
}

// createRDSIAMRole creates the rds_iam role for testing IAM functionality
func createRDSIAMRole(manager *Manager) error {
	query := "CREATE ROLE rds_iam"
	_, err := manager.db.Exec(query)
	if err != nil && err.Error() != `pq: role "rds_iam" already exists` {
		return err
	}
	return nil
}

// Cleanup cleans up the test database and decrements reference count
func (stds *SharedTestDatabaseSetup) Cleanup(t *testing.T) {
	// Clean up test data first
	stds.ResetDatabase(t)

	// Close the manager
	if stds.Manager != nil {
		if err := stds.Manager.Close(); err != nil {
			t.Logf("Error closing database manager: %v", err)
		}
	}

	// Drop the test database
	if err := dropTestDatabase(sharedContainer, stds.dbName); err != nil {
		t.Logf("Error dropping test database: %v", err)
	}

	// Decrement reference count and clean up container if needed
	containerMutex.Lock()
	defer containerMutex.Unlock()

	if sharedContainer != nil {
		sharedContainer.mutex.Lock()
		sharedContainer.refCount--
		refCount := sharedContainer.refCount
		sharedContainer.mutex.Unlock()

		// If no more tests are using the container, clean it up
		if refCount <= 0 {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := sharedContainer.Container.Terminate(ctx); err != nil {
				t.Logf("Error terminating shared container: %v", err)
			}
			sharedContainer = nil
			t.Log("Shared container terminated")
		}
	}
}

// dropTestDatabase drops the test database
func dropTestDatabase(container *SharedTestContainer, dbName string) error {
	// Create a temporary manager to drop the database
	tempManager, err := NewManager(container.ConnInfo, container.Logger, false)
	if err != nil {
		return err
	}
	defer tempManager.Close()

	// Drop the database
	query := "DROP DATABASE IF EXISTS " + dbName
	_, err = tempManager.db.Exec(query)
	return err
}

// ResetDatabase cleans up any test data from the database
func (stds *SharedTestDatabaseSetup) ResetDatabase(t *testing.T) {
	stds.dropTestUsers(t)
	stds.dropTestRoles(t)
}

// dropTestUsers removes test users from the database
func (stds *SharedTestDatabaseSetup) dropTestUsers(t *testing.T) {
	testUsers := []string{
		"test_user", "test_user_2", "iam_user", "nologin_user", "limited_user",
		"invalid_user", "quoted_user", "password_user", "unlimited_user",
		"zero_user", "positive_user", "group_user", "priv_user",
	}

	for _, user := range testUsers {
		exists, err := stds.Manager.UserExists(user)
		if err != nil {
			t.Logf("Error checking if user %s exists: %v", user, err)
			continue
		}
		if exists {
			if err := stds.Manager.DropUser(user); err != nil {
				t.Logf("Error dropping test user %s: %v", user, err)
			}
		}
	}
}

// dropTestRoles removes test roles from the database
func (stds *SharedTestDatabaseSetup) dropTestRoles(t *testing.T) {
	testRoles := []string{
		"test_group", "test_role", "app_group", "read_only",
		"admin_group", "user_group", "temp_group",
	}

	for _, role := range testRoles {
		exists, err := stds.Manager.GroupExists(role)
		if err != nil {
			t.Logf("Error checking if role %s exists: %v", role, err)
			continue
		}
		if exists {
			// Manually drop the role since there's no DropGroup method
			query := "DROP ROLE IF EXISTS " + role
			if _, err := stds.Manager.db.Exec(query); err != nil {
				t.Logf("Error dropping test role %s: %v", role, err)
			}
		}
	}
}

// GetManager returns the database manager
func (stds *SharedTestDatabaseSetup) GetManager() *Manager {
	return stds.Manager
}
