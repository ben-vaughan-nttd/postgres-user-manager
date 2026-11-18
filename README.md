# PostgreSQL User Manager

A command-line tool for managing PostgreSQL users, groups, and privileges with idempotent operations.

## AWS RDS Aurora PostgreSQL Support

This tool provides first-class support for **AWS RDS Aurora PostgreSQL with IAM database authentication**. Key features include:

- ✅ **IAM Authentication**: Create users that authenticate using AWS IAM instead of passwords
- ✅ **Automatic RDS IAM Role**: Automatically grants `rds_iam` role to IAM users
- ✅ **SSL Enforcement**: Automatically enforces SSL for IAM authentication
- ✅ **Mixed Authentication**: Support both password and IAM users in the same database
- ✅ **Connection Limits**: Proper connection management for IAM token limitations

For detailed AWS RDS setup and configuration, see [AWS RDS IAM Authentication Guide](docs/AWS_RDS_IAM_GUIDE.md).

## Installation

### Development setup

Install the repository's Go version with [mise](https://mise.jdx.dev/):

```bash
mise install
```

The Go version is discovered from `go.mod`; the `toolchain` directive selects the
preferred patch release.

### Build requirements and platforms

Permission statements are validated with PostgreSQL's own parser through
[`pg_query_go`](https://github.com/pganalyze/pg_query_go), which embeds the
parser in C. Building from source therefore needs **CGO** and a C compiler
(`gcc` or `clang`) on `PATH`.

Release binaries and container images are built for:

| OS | Architectures |
|----|---------------|
| Linux | `amd64`, `arm64` |
| macOS (darwin) | `amd64`, `arm64` |
| Windows | `amd64` |

32-bit x86 (`386`) is no longer built because the embedded parser has to be
cross compiled with a C toolchain for every target. The container image is
based on Debian trixie and is dynamically linked.

#### Linux runtime requirements

Because of CGO, the Linux release binaries are **dynamically linked against
glibc** (`libc.so.6` and the dynamic loader) rather than being static. They run
only on glibc-based distributions:

- **glibc 2.34 or newer** is required. This is the highest glibc symbol version
  in the `linux/amd64` and `linux/arm64` binaries built in the
  `goreleaser-cross` release image, and it can rise if that image changes. Check
  a binary with `objdump -T postgres-user-manager | grep -o 'GLIBC_[0-9.]*' | sort -uV | tail -1`.
- That covers Debian 12 and newer, Ubuntu 22.04 and newer, and RHEL 9 and
  newer (and compatible distributions). Older releases such as Debian 11,
  Ubuntu 20.04 and RHEL 8 are **not supported**.
- musl-based systems such as **Alpine Linux are not supported**. Use the
  container image, or build from source on the target system.
- The container image is based on `debian:trixie-slim` (glibc 2.41) and needs no
  extra setup.

### From Source

```bash
git clone https://github.com/ben-vaughan-nttd/postgres-user-manager.git
cd postgres-user-manager
CGO_ENABLED=1 go build -o postgres-user-manager main.go
```

### Using Go Install

```bash
CGO_ENABLED=1 go install github.com/ben-vaughan-nttd/postgres-user-manager@latest
```

## Environment Variables

The tool supports both traditional password authentication and AWS RDS IAM database authentication.

### Password Authentication (Traditional)

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `POSTGRES_HOST` | Database host | `localhost` | No |
| `POSTGRES_PORT` | Database port | `5432` | No |
| `POSTGRES_DB` | Database name | `postgres` | No |
| `POSTGRES_USER` | Database username | `postgres` | No |
| `POSTGRES_PASSWORD` | Database password | - | **Yes** |
| `POSTGRES_SSLMODE` | SSL mode | `prefer` | No |
| `POSTGRES_IAM_AUTH` | Enable IAM auth | `false` | No |

### IAM Authentication (AWS RDS Aurora)

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `POSTGRES_HOST` | RDS Aurora endpoint | `localhost` | No |
| `POSTGRES_PORT` | Database port | `5432` | No |
| `POSTGRES_DB` | Database name | `postgres` | No |
| `POSTGRES_USER` | Database username | `postgres` | No |
| `POSTGRES_SSLMODE` | SSL mode | `require` | No |
| `POSTGRES_IAM_AUTH` | Enable IAM auth | `true` | **Yes** |
| `POSTGRES_IAM_TOKEN` | IAM auth token | - | No (auto-generated) |
| `AWS_REGION` | AWS region | `us-east-1` | **Yes** |
| `AWS_ACCESS_KEY_ID` | AWS access key | - | No (if using IAM role) |
| `AWS_SECRET_ACCESS_KEY` | AWS secret key | - | No (if using IAM role) |

### Example Environment Setup

#### Traditional Password Authentication
```bash
export POSTGRES_HOST=localhost
export POSTGRES_PORT=5432
export POSTGRES_DB=myapp
export POSTGRES_USER=admin
export POSTGRES_PASSWORD=your_secure_password
export POSTGRES_SSLMODE=require
```

#### AWS RDS Aurora with IAM Authentication
```bash
export POSTGRES_HOST=my-aurora-cluster.cluster-xxxxx.us-east-1.rds.amazonaws.com
export POSTGRES_PORT=5432
export POSTGRES_DB=postgres
export POSTGRES_USER=your_admin_user
export POSTGRES_SSLMODE=require
export POSTGRES_IAM_AUTH=true
export AWS_REGION=us-east-1
```

## Configuration File

The tool uses JSON configuration files to define the desired state of users and groups.

### Configuration Structure

```json
{
  "users": [
    {
      "username": "app_user",
      "password": "secure_password_123",
      "groups": ["app_group", "read_only"],
      "privileges": ["CONNECT"],
      "databases": ["myapp_db"],
      "permissions": [
        "GRANT USAGE ON SCHEMA app TO app_user",
        "GRANT SELECT, INSERT, UPDATE ON TABLE app.orders TO app_user",
        "GRANT USAGE ON SEQUENCE app.orders_id_seq TO app_user"
      ],
      "enabled": true,
      "description": "Application user"
    }
  ],
  "groups": [
    {
      "name": "app_group",
      "privileges": ["CONNECT", "TEMPORARY"],
      "databases": ["myapp_db"],
      "permissions": [
        "GRANT SELECT ON ALL TABLES IN SCHEMA app TO app_group"
      ],
      "description": "Main application group",
      "inherit": true
    }
  ]
}
```

### User Configuration Fields

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| `username` | string | PostgreSQL username | Yes |
| `password` | string | User password (optional, can be generated) | No |
| `groups` | array | Groups/roles to assign user to | No |
| `privileges` | array | Direct privileges to grant | No |
| `databases` | array | Databases to grant privileges on | No |
| `permissions` | array | `GRANT` statements describing the user's desired privileges (see [Permissions](#permissions)) | No |
| `enabled` | boolean | Whether the user should be created/maintained | Yes |
| `description` | string | User description | No |

### Group Configuration Fields

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| `name` | string | Group/role name | Yes |
| `privileges` | array | Privileges to grant to the group | No |
| `databases` | array | Databases to grant privileges on | No |
| `permissions` | array | `GRANT` statements describing the group's desired privileges (see [Permissions](#permissions)) | No |
| `description` | string | Group description | No |
| `inherit` | boolean | Whether group members inherit privileges | No |

### Supported Privileges

The `privileges` and `databases` fields grant **database-level** privileges. Each
`privileges` entry is combined with each `databases` entry into
`GRANT <privilege> ON DATABASE <database> TO <role>`, so the valid values are the
DATABASE privileges in the [support matrix](#permission-support-matrix) below:

- `CONNECT` - Connect to database
- `CREATE` - Create schemas in the database
- `TEMPORARY` (or `TEMP`) - Create temporary tables
- `ALL` - `CONNECT`, `CREATE` and `TEMPORARY`

For schema, table and sequence privileges use `permissions`.

## Permissions

### Declarative model

`permissions` is a list of SQL `GRANT` statements that together describe the
**desired privileges** of one user or group, in the same spirit as a Terraform
resource: the configuration states what should exist.

- `permissions` accepts **`GRANT` statements only**. A `REVOKE` in the
  configuration is rejected. Access is removed by deleting the `GRANT` from the
  configuration, not by adding a `REVOKE`.
- Today `sync` applies the grants. A future `sync` will compare the declared
  grants with the database and revoke what the configuration no longer
  declares. The validator already returns a normalized rule that supports this
  comparison (and can generate the matching `REVOKE`), so configurations written
  now will keep their meaning.
- `privileges` + `databases` remain supported. They are turned into
  `GRANT ... ON DATABASE ...` statements and validated the same way.
- Role membership is not a permission. Use `groups` for it.

### Validation

Every `GRANT` or `REVOKE` the tool runs is parsed with the PostgreSQL parser,
checked against the support matrix, and executed from the canonical SQL rendered
from the parsed statement (never from the raw text). This applies to
`permissions`, to `privileges`/`databases`, and to the `create-user` flags.

- Exactly one statement is allowed per entry. `;`-chained statements,
  statements hidden behind comments, and dollar-quoted blocks are rejected.
- Validation runs when the configuration is loaded, so `validate`, `sync` and
  `create-user` all fail with an actionable error **before** any database change.
- Statements in a `permissions` list are validated together before any is
  executed: if one entry is rejected, none of that list runs.

```bash
postgres-user-manager validate --config config.json
```

### Grantee rule

Each statement must name **exactly one grantee, and it must be the user or
group the statement is listed under**. Identifiers follow PostgreSQL rules:
unquoted names are folded to lower case and quoted names keep their exact case.

| Listed under role | Grantee in statement | Accepted |
|-------------------|----------------------|----------|
| `app_user` | `app_user` | yes |
| `app_user` | `App_User` (unquoted, folds to `app_user`) | yes |
| `app_user` | `"app_user"` | yes |
| `App_User` | `"App_User"` | yes |
| `App_User` | `App_User` (unquoted, folds to `app_user`) | no |
| `app_user` | `someone_else` | no |

`PUBLIC`, `CURRENT_USER`, `CURRENT_ROLE`, `SESSION_USER` and multiple grantees
are rejected.

### Permission support matrix

| Object | Privileges | Notes |
|--------|------------|-------|
| `DATABASE` | `CONNECT`, `CREATE`, `TEMPORARY` / `TEMP`, `ALL` | `ALL` = `CONNECT`, `CREATE`, `TEMPORARY` |
| `SCHEMA` | `USAGE`, `CREATE`, `ALL` | `ALL` = `USAGE`, `CREATE` |
| `TABLE` | `SELECT`, `INSERT`, `UPDATE`, `DELETE`, `TRUNCATE`, `REFERENCES`, `TRIGGER`, `MAINTAIN`, `ALL` | `ALL` includes `MAINTAIN`. `ALL TABLES IN SCHEMA` is supported. Schema-qualified names (`schema.table`) are supported. |
| `TABLE` columns | `SELECT`, `INSERT`, `UPDATE`, `REFERENCES`, `ALL` | `GRANT SELECT (a, b) ON TABLE t ...`. `ALL (cols)` means `SELECT`, `INSERT`, `UPDATE`, `REFERENCES`. Every privilege in one statement must use the same column list, and column and table-level privileges cannot be mixed in one statement. Not allowed with `ALL TABLES IN SCHEMA`. |
| `SEQUENCE` | `USAGE`, `SELECT`, `UPDATE`, `ALL` | `ALL SEQUENCES IN SCHEMA` is supported. |

Rejected, with an error that names the offending element:

- Any statement other than `GRANT`/`REVOKE` on privileges, including role
  membership (`GRANT role TO role`), `DO`, `COPY`, `CALL`, and
  `ALTER DEFAULT PRIVILEGES`.
- Grantees `PUBLIC`, `CURRENT_USER`, `CURRENT_ROLE`, `SESSION_USER`, and more than
  one grantee.
- `GRANTED BY`, `WITH GRANT OPTION`, `GRANT OPTION FOR`, and `CASCADE`.
- Functions, procedures, routines, types, domains, languages, large objects,
  foreign servers and foreign data wrappers, tablespaces, and parameters.
- Database-qualified table names (`db.schema.table`).

`REVOKE` is accepted by the validator itself (the database manager's
`RevokePrivileges` and a future `sync` use it) but not in `permissions`.

### Normalization

Equivalent statements produce the same normalized rule: keywords and unquoted
identifiers are case-insensitive, `TEMP` equals `TEMPORARY`, and privileges,
objects and columns are de-duplicated and sorted.

`ALL` is expanded to the concrete privileges only for comparison, so
`GRANT ALL ON TABLE t` and a statement that lists every table privilege are the
same rule. When the tool runs the statement, a **full privilege set is
canonicalized back to `ALL`** (`ALL (columns)` for column grants), never to the
explicit list. This applies to every object type and to `ALL TABLES IN SCHEMA`.

What `ALL` means is decided by the server that runs the statement, so it depends
on the PostgreSQL version. For example, `ALL` on a table includes `MAINTAIN` on
PostgreSQL 17 and newer, and does not on PostgreSQL 16 and older, where
`MAINTAIN` does not exist. Rendering the full set as `ALL` keeps such statements
valid on older servers instead of sending a `MAINTAIN` they would reject.
Explicitly naming `MAINTAIN` together with every other table privilege is also
sent as `ALL`; to grant `MAINTAIN` alone, list it on its own.

### Parser version policy

Statements are parsed with the **PostgreSQL 17** grammar embedded in
`pg_query_go` v6 (pinned in `go.mod`). The grammar version does not depend on the
server version. Syntax that is not valid in the PostgreSQL 17 grammar is rejected
with an error that names the grammar version. Validation checks the statement
against the PostgreSQL 17 support matrix; it does not check what the target
server supports. `MAINTAIN` is a PostgreSQL 17 privilege: the validator accepts
it, but servers older than 17 will reject a statement that names it. Full
privilege sets are canonicalized to `ALL` (see [Normalization](#normalization)),
whose meaning follows the target server's version. Moving to a newer grammar means upgrading
`pg_query_go` and is treated as a deliberate change.

## Usage

### Commands

#### Sync Configuration

Synchronize the database state with your configuration file:

```bash
# Basic sync
postgres-user-manager sync --config config.json

# Dry run (preview changes)
postgres-user-manager sync --config config.json --dry-run

# Verbose output
postgres-user-manager sync --config config.json --verbose
```

#### Create Individual User

Create a single user with specific settings:

```bash
# Basic user creation (password auth)
postgres-user-manager create-user myuser

# User with password and groups
postgres-user-manager create-user myuser \
  --password "secure_pass" \
  --groups "app_group,read_only" \
  --privileges "CONNECT" \
  --databases "myapp_db"

# IAM authenticated user for AWS RDS
postgres-user-manager create-user iam_user \
  --auth-method iam \
  --groups "app_group" \
  --privileges "CONNECT" \
  --databases "myapp_db" \
  --iam-role "arn:aws:iam::123456789012:role/RDSAccessRole" \
  --connection-limit 10

# Service account (no login)
postgres-user-manager create-user service_account \
  --auth-method iam \
  --groups "app_group" \
  --can-login=false

# Dry run
postgres-user-manager create-user myuser --dry-run
```

#### Drop User

Remove a user from the database:

```bash
# Drop user
postgres-user-manager drop-user myuser

# Dry run
postgres-user-manager drop-user myuser --dry-run
```

#### List Users

List all database users:

```bash
postgres-user-manager list-users
```

#### Validate Configuration

Validate your configuration file without making changes:

```bash
postgres-user-manager validate --config config.json
```

### Global Flags

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--config` | `-c` | Path to configuration file | `./config.json` |
| `--dry-run` | - | Show what would be done without executing | `false` |
| `--verbose` | `-v` | Enable verbose output | `false` |
| `--help` | `-h` | Show help information | - |

## Examples

### Complete Workflow

1. **Create configuration file**:
```bash
cp config.example.json config.json
# Edit config.json to match your requirements
```

2. **Set environment variables**:
```bash
export POSTGRES_PASSWORD=your_admin_password
export POSTGRES_HOST=your_db_host
export POSTGRES_DB=your_database
```

3. **Validate configuration**:
```bash
postgres-user-manager validate --config config.json
```

4. **Preview changes**:
```bash
postgres-user-manager sync --config config.json --dry-run --verbose
```

5. **Apply changes**:
```bash
postgres-user-manager sync --config config.json --verbose
```

### CI/CD Integration

The tool is designed to be CI/CD friendly:

```yaml
# GitHub Actions example
- name: Sync PostgreSQL Users
  env:
    POSTGRES_PASSWORD: ${{ secrets.POSTGRES_PASSWORD }}
    POSTGRES_HOST: ${{ secrets.POSTGRES_HOST }}
  run: |
    postgres-user-manager validate --config config.json
    postgres-user-manager sync --config config.json
```

## Security Best Practices

1. **Environment Variables**: Always use environment variables for sensitive data like passwords
2. **Least Privilege**: Grant only necessary privileges to users and groups
3. **Configuration Review**: Review configuration changes in pull requests
4. **Dry Run**: Always test with `--dry-run` first in production environments
5. **Backup**: Backup your database before making significant user changes

## Future Enhancements

This tool is designed with future AWS Cognito integration in mind:

- **Event-Driven Updates**: Listen to AWS Cognito events for automatic user management
- **JWT Integration**: Support for JWT-based authentication flows
- **Group Synchronization**: Automatic sync of Cognito groups to PostgreSQL roles
- **Audit Logging**: Enhanced audit trails for compliance

## Development

### Building

```bash
CGO_ENABLED=1 go build -o postgres-user-manager main.go
```

Build every release platform (linux amd64/arm64, darwin amd64/arm64,
windows amd64) with the C cross compilers in the `goreleaser-cross` image:

```bash
make build-all
```

### Running Tests

```bash
go test ./...
```

### Code Quality

```bash
go vet ./...
go fmt ./...
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

[License details here]

## Support

For issues and questions:
- Create an issue on GitHub
- Check the documentation
- Review example configurations
