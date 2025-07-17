# Integration Tests for PostgreSQL User Manager

This directory contains comprehensive integration tests for the database functionality of the PostgreSQL User Manager tool. The tests use testcontainers to spin up real PostgreSQL instances for testing.

## Prerequisites

1. **Docker**: The tests require Docker to be installed and running, as testcontainers uses Docker to create PostgreSQL instances.
2. **Go**: Go 1.24.3 or later
3. **Test Dependencies**: Run `go mod tidy` to install required dependencies

## Test Structure

The integration tests are organized into several files:

### Core Test Files

- **`testsetup.go`**: Common test setup and utilities
  - `TestDatabaseSetup`: Manages shared PostgreSQL container setup
  - Helper functions for database cleanup and test data management

- **`database_test.go`**: Core database functionality tests
  - Manager creation and connection testing
  - User creation, deletion, and existence checks
  - Basic CRUD operations

- **`groups_test.go`**: Group/role management tests
  - Group creation and existence checks
  - User-to-group membership operations
  - User information retrieval

- **`privileges_test.go`**: Privilege management and synchronization tests
  - Granting and revoking privileges
  - Configuration synchronization
  - Dry-run mode testing

- **`edge_cases_test.go`**: Edge cases and error scenarios
  - Special characters in usernames and passwords
  - Connection limit variations
  - Error handling scenarios
  - Helper method testing

## Running the Tests

### Run All Database Integration Tests

```bash
# From the project root
go test ./internal/database -v

# Or with more verbose output and race detection
go test ./internal/database -v -race
```

### Run Specific Test Files

```bash
# Run only core database tests
go test ./internal/database -v -run TestCreateUser

# Run only group-related tests
go test ./internal/database -v -run TestGroup

# Run only privilege tests
go test ./internal/database -v -run TestGrant
```

### Run Tests with Docker Compose (if applicable)

If you have a docker-compose setup:

```bash
# Make sure Docker is running
docker --version

# Run the tests
go test ./internal/database -v
```

## Test Features

### Isolated Test Environment
- Each test uses a fresh PostgreSQL container via testcontainers
- Tests are isolated and can run in parallel
- Automatic cleanup of test data and containers

### Comprehensive Coverage
- **User Management**: Create, delete, check existence
- **Group Management**: Create groups, manage memberships
- **Privilege Management**: Grant/revoke privileges on databases
- **Configuration Sync**: Test full configuration synchronization
- **Edge Cases**: Special characters, error scenarios, dry-run mode
- **IAM Authentication**: Test IAM auth flow (without actual AWS dependencies)

### Test Database Setup
- Uses PostgreSQL 15 Alpine container
- Creates isolated test databases for privilege testing
- Automatic cleanup of test users, groups, and databases
- Support for both regular and dry-run testing modes

## Test Configuration

The tests use the following default configuration:
- **Database**: `testdb`
- **Username**: `testuser`  
- **Password**: `testpass`
- **Host**: Container-assigned host
- **Port**: Container-assigned port
- **SSL Mode**: `disable` (for testing)

## Important Notes

### AWS IAM Features
The tests include IAM authentication scenarios but do not require actual AWS credentials or services. IAM-related functionality is tested for:
- User creation with IAM auth method
- Role assignment patterns
- Configuration without actual AWS token generation

### Performance Considerations
- Tests may take longer on first run due to Docker image downloads
- Subsequent runs are faster as images are cached
- Each test creates a new container for complete isolation

### Debugging Failed Tests
If tests fail:

1. **Check Docker**: Ensure Docker is running and accessible
2. **Container Logs**: testcontainers provides container logs in case of failures
3. **Network Issues**: Ensure no firewall blocking container communication
4. **Resource Limits**: Ensure sufficient system resources for containers

### Cleanup
Tests automatically clean up:
- PostgreSQL containers after each test
- Database connections
- Test users and roles
- Temporary test databases

## Example Test Run

```bash
$ go test ./internal/database -v
=== RUN   TestNewManager
--- PASS: TestNewManager (2.34s)
=== RUN   TestNewManagerWithInvalidConnection  
--- PASS: TestNewManagerWithInvalidConnection (2.12s)
=== RUN   TestUserExists
--- PASS: TestUserExists (2.45s)
=== RUN   TestCreateUser
=== RUN   TestCreateUser/Create_user_with_password_auth
=== RUN   TestCreateUser/Create_user_with_IAM_auth
=== RUN   TestCreateUser/Create_user_with_no_login
=== RUN   TestCreateUser/Create_user_with_connection_limit
--- PASS: TestCreateUser (8.92s)
...
PASS
ok      github.com/ben-vaughan-nttd/postgres-user-manager/internal/database    45.123s
```

This test suite provides comprehensive coverage of the database functionality while ensuring isolation and reliability through containerized testing.
