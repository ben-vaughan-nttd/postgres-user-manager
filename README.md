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

### From Source

```bash
git clone https://github.com/ben-vaughan-nttd/postgres-user-manager.git
cd postgres-user-manager
go build -o postgres-user-manager main.go
```

### Using Go Install

```bash
go install github.com/ben-vaughan-nttd/postgres-user-manager@latest
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
      "enabled": true,
      "description": "Application user"
    }
  ],
  "groups": [
    {
      "name": "app_group",
      "privileges": ["CONNECT", "TEMPORARY"],
      "databases": ["myapp_db"],
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
| `enabled` | boolean | Whether the user should be created/maintained | Yes |
| `description` | string | User description | No |

### Group Configuration Fields

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| `name` | string | Group/role name | Yes |
| `privileges` | array | Privileges to grant to the group | No |
| `databases` | array | Databases to grant privileges on | No |
| `description` | string | Group description | No |
| `inherit` | boolean | Whether group members inherit privileges | No |

### Supported Privileges

- `CONNECT` - Connect to database
- `TEMPORARY` - Create temporary tables
- `ALL` - All privileges on database
- `CREATE` - Create schemas/tables
- `USAGE` - Use schemas
- And other standard PostgreSQL privileges

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
go build -o postgres-user-manager main.go
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
