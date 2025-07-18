package database

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestContainerDebug is a minimal test to debug testcontainers issues
func TestContainerDebug(t *testing.T) {
	ctx := context.Background()

	t.Log("Starting testcontainers debug test...")

	// Try the most basic postgres container setup
	container, err := postgres.Run(ctx,
		"postgres:15-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		// Increase timeout and be more explicit about wait strategy
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(3*time.Minute),
		),
		// Add some debugging options
		testcontainers.WithLogConsumers(&testLogConsumer{t: t}),
	)

	if err != nil {
		t.Fatalf("Failed to start container: %v", err)
	}

	defer func() {
		t.Log("Cleaning up container...")
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Error terminating container: %v", err)
		}
	}()

	t.Log("Container started successfully!")

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

	// Try to connect
	t.Log("Container details retrieved successfully!")
}

// testLogConsumer helps us see what's happening with the container
type testLogConsumer struct {
	t *testing.T
}

func (lc *testLogConsumer) Accept(log testcontainers.Log) {
	lc.t.Logf("Container log: %s", string(log.Content))
}
