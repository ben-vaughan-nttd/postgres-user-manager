# Docker Setup Issues and Solutions

## Issue Encountered

The testcontainers setup is having issues with the current Docker environment (Colima). The containers are being created but immediately terminated with errors like:

```
Error response from daemon: No such container: [container-id]
```

## Solutions

### Option 1: Use Docker Desktop (Recommended)

The most reliable way to run these tests is with Docker Desktop:

1. Install [Docker Desktop](https://www.docker.com/products/docker-desktop/)
2. Start Docker Desktop
3. Run the tests: `go test ./internal/database -v`

### Option 2: Colima Configuration

If using Colima, ensure it's properly configured:

```bash
# Stop current Colima instance
colima stop

# Start with proper settings
colima start --cpu 2 --memory 4 --disk 20

# Verify Docker works
docker run hello-world
```

### Option 3: Manual PostgreSQL Testing

For development without testcontainers, you can:

1. Start a local PostgreSQL instance:
```bash
docker run --name postgres-test -e POSTGRES_PASSWORD=testpass -e POSTGRES_USER=testuser -e POSTGRES_DB=testdb -p 5432:5432 -d postgres:15-alpine
```

2. Run tests against the local instance using environment variables:
```bash
export USE_LOCAL_POSTGRES=true
export POSTGRES_HOST=localhost
export POSTGRES_PORT=5432
export POSTGRES_USER=testuser
export POSTGRES_PASSWORD=testpass
export POSTGRES_DB=testdb
go test ./internal/database -v
```

3. Clean up:
```bash
docker stop postgres-test
docker rm postgres-test
```

### Option 4: Skip Docker Tests

You can also run the non-Docker tests:

```bash
# Run only unit tests that don't require containers
go test ./internal/config -v
go test ./internal/structs -v
```

## Current Test Status

The integration tests have been created and should work properly once the Docker environment is correctly configured. The test files include:

- **testsetup.go**: Container setup utilities
- **database_test.go**: Core database functionality tests
- **groups_test.go**: Group/role management tests  
- **privileges_test.go**: Privilege management and sync tests
- **edge_cases_test.go**: Edge cases and error handling

## Alternative: Mock Tests

If Docker continues to be problematic, consider creating mock-based unit tests instead of integration tests. This would involve:

1. Creating interfaces for the database operations
2. Using mock implementations for testing
3. Testing the business logic without actual database connections

This approach would be faster and more reliable, though less comprehensive than true integration testing.
