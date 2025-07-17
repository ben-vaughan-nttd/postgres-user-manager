package database

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestGenericContainerDebug tests with a generic container approach
func TestGenericContainerDebug(t *testing.T) {
	ctx := context.Background()

	t.Log("Starting generic container debug test...")

	// Try a generic container setup instead of using the postgres module
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
		t.Fatalf("Failed to start generic container: %v", err)
	}

	defer func() {
		t.Log("Cleaning up generic container...")
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Error terminating container: %v", err)
		}
	}()

	t.Log("Generic container started successfully!")

	// Get container details
	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("Failed to get host: %v", err)
	}
	t.Logf("Container host: %s", host)

	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatalf("Failed to get port: %v", err)
	}
	t.Logf("Container port: %s", port.Port())

	t.Log("Generic container test completed successfully!")
}

// TestSimpleContainerDebug tests with the simplest possible container
func TestSimpleContainerDebug(t *testing.T) {
	ctx := context.Background()

	t.Log("Starting simple container debug test...")

	// Try the simplest possible container that just exits successfully
	req := testcontainers.ContainerRequest{
		Image: "hello-world",
		WaitingFor: wait.ForExit().WithExitTimeout(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})

	if err != nil {
		t.Fatalf("Failed to start simple container: %v", err)
	}

	defer func() {
		t.Log("Cleaning up simple container...")
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Error terminating container: %v", err)
		}
	}()

	t.Log("Simple container test completed successfully!")
}
