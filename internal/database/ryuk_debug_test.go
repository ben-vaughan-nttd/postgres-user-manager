package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestWithoutRyuk tests testcontainers with ryuk disabled
func TestWithoutRyuk(t *testing.T) {
	// Disable ryuk for this test
	os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	defer os.Unsetenv("TESTCONTAINERS_RYUK_DISABLED")

	ctx := context.Background()

	t.Log("Starting container test with ryuk disabled...")

	// Try a simple container
	req := testcontainers.ContainerRequest{
		Image: "hello-world",
		WaitingFor: wait.ForExit().WithExitTimeout(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})

	if err != nil {
		t.Fatalf("Failed to start container: %v", err)
	}

	defer func() {
		t.Log("Cleaning up container...")
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Error terminating container: %v", err)
		}
	}()

	t.Log("Container test completed successfully!")
}

// TestPostgreSQLWithoutRyuk tests PostgreSQL container with ryuk disabled
func TestPostgreSQLWithoutRyuk(t *testing.T) {
	// Disable ryuk for this test
	os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	defer os.Unsetenv("TESTCONTAINERS_RYUK_DISABLED")

	ctx := context.Background()

	t.Log("Starting PostgreSQL container test with ryuk disabled...")

	req := testcontainers.ContainerRequest{
		Image:        "postgres:15-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_DB":       "testdb",
			"POSTGRES_USER":     "testuser",
			"POSTGRES_PASSWORD": "testpass",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).
			WithStartupTimeout(2 * time.Minute),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})

	if err != nil {
		t.Fatalf("Failed to start PostgreSQL container: %v", err)
	}

	defer func() {
		t.Log("Cleaning up PostgreSQL container...")
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Error terminating container: %v", err)
		}
	}()

	// Get container details
	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("Failed to get host: %v", err)
	}
	t.Logf("PostgreSQL container host: %s", host)

	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatalf("Failed to get port: %v", err)
	}
	t.Logf("PostgreSQL container port: %s", port.Port())

	t.Log("PostgreSQL container test completed successfully!")
}
