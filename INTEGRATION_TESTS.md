# Integration Tests

The database tests in `internal/database` run against a real PostgreSQL
instance started with [Testcontainers](https://golang.testcontainers.org/).
Each test gets its own `postgres:15-alpine` container, so tests do not touch
any existing database. Docker is required.

The Testcontainers code lives only in `_test.go` files, so it is not part of the
production build or the release binary.

## Test setup

There is a single setup, `SetupFlexibleTestDatabase`, defined in
`internal/database/flexible_testsetup_test.go`.

```go
setup := SetupFlexibleTestDatabase(t)
defer setup.Cleanup(t)

manager := setup.GetManager()
```

It returns a `FlexibleTestDatabaseSetup` with these helpers:

- `Cleanup(t)` closes the manager connection, terminates the container and
  unsets `TESTCONTAINERS_RYUK_DISABLED` unless
  `TESTCONTAINERS_PERSIST_RYUK_DISABLED` is `true`.
- `ResetDatabase(t)` drops the fixed set of test users and roles that the tests
  create.
- `CreateTestDatabase(t, name)` creates an additional database.
- `DropTestDatabase(t, name)` drops a database created with `CreateTestDatabase`.
- `GetManager()` returns the `Manager` connected to the container.

The `DatabaseTestSetup` interface (`GetManager`, `Cleanup`, `ResetDatabase`) is
used by the `verify*` helpers in `privileges_test.go`.

## Docker environment detection

`SetupFlexibleTestDatabase` inspects the Docker environment and configures
Testcontainers to match it. In particular it decides whether to disable the
Ryuk resource reaper, which does not work with some Docker setups. It
recognizes Colima, Lima, Podman and Docker Desktop, and falls back to treating
the environment as unknown (Ryuk disabled).

Docker Desktop is only detected on macOS (`runtime.GOOS == "darwin"`, with
`/var/run/docker.sock` present). On Linux, including CI, `/var/run/docker.sock`
is treated as unknown, so Ryuk is disabled there.

The setup reads these environment variables:

| Variable | Effect |
|----------|--------|
| `DOCKER_HOST` | Used to detect the Docker provider (Colima, Lima or Podman) from the socket path. |
| `CI` | When `true` and Docker Desktop is detected, Ryuk is disabled. |
| `TESTCONTAINERS_RYUK_DISABLED` | When `true`, detection is skipped and Ryuk stays disabled. The setup also sets this itself when the detected environment needs it. `Cleanup` unsets it after each test unless `TESTCONTAINERS_PERSIST_RYUK_DISABLED=true`. |
| `TESTCONTAINERS_PREFER_NO_RYUK` | When `true` and Docker Desktop is detected, Ryuk is disabled. |
| `TESTCONTAINERS_PERSIST_RYUK_DISABLED` | When `true`, `Cleanup` leaves `TESTCONTAINERS_RYUK_DISABLED` set instead of unsetting it. |

## Test files

All files are in `internal/database/`.

| File | Contents |
|------|----------|
| `flexible_testsetup_test.go` | The test database setup and Docker environment detection. |
| `database_test.go` | Manager creation and connection, user creation, deletion and existence checks, dry-run mode, invalid connections. |
| `groups_test.go` | Group creation and existence, user-to-group membership, user information retrieval, duplicate groups. |
| `privileges_test.go` | Granting and revoking privileges for users and groups, full configuration sync, error handling during sync, dry-run mode. |
| `edge_cases_test.go` | Special characters in usernames and passwords, connection limit variations, IAM authentication flow, `quoteIdentifier` and `escapeString`, non-existent users and groups. |
| `flexible_test.go` | `TestFlexibleSetup` and `TestDockerEnvironmentDetection`. |
| `port_testhelper_test.go` | `mappedPortInt` helper and `TestMappedPortInt`. |

## Running the tests

Start Docker, then run:

```bash
go test ./internal/database -v
```

Run a subset with `-run`:

```bash
go test ./internal/database -v -run TestCreateUser
go test ./internal/database -v -run TestGroup
go test ./internal/database -v -run TestSyncConfiguration
go test ./internal/database -v -run TestFlexibleSetup
```

## Not covered

The tests make no AWS calls and do not generate IAM tokens. To simulate RDS,
the setup creates a local PostgreSQL role named `rds_iam` in the test container.
IAM-auth users are created in the container and granted that local role.
