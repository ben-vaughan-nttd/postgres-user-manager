# AWS RDS Aurora PostgreSQL IAM Authentication Guide

This document provides detailed guidance on using the PostgreSQL User Manager with AWS RDS Aurora PostgreSQL and IAM database authentication.

## Overview

AWS RDS Aurora PostgreSQL supports IAM database authentication, which allows you to authenticate to your database using AWS IAM credentials instead of traditional database passwords. This provides enhanced security through:

- Centralized identity and access management
- Automatic credential rotation
- Integration with AWS security features
- Elimination of hardcoded database passwords

## Prerequisites

1. **AWS RDS Aurora PostgreSQL cluster** with IAM database authentication enabled
2. **AWS IAM roles and policies** configured for database access
3. **SSL/TLS connection** (required for IAM authentication)
4. **AWS CLI or SDK** configured with appropriate credentials

## Configuration

### 1. Enable IAM Authentication on RDS Cluster

When creating or modifying your Aurora PostgreSQL cluster:

```bash
# Create cluster with IAM auth enabled
aws rds create-db-cluster \
  --db-cluster-identifier my-aurora-cluster \
  --engine aurora-postgresql \
  --master-username postgres \
  --master-user-password mypassword \
  --enable-iam-database-authentication

# Or modify existing cluster
aws rds modify-db-cluster \
  --db-cluster-identifier my-aurora-cluster \
  --enable-iam-database-authentication \
  --apply-immediately
```

### 2. Create IAM Policy for Database Access

Create an IAM policy that allows connecting to your database:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "rds-db:connect"
      ],
      "Resource": [
        "arn:aws:rds-db:us-east-1:123456789012:dbuser:cluster-XXXXXXXXX/iam_user"
      ]
    }
  ]
}
```

### 3. Create IAM Role for Database Access

```bash
# Create role
aws iam create-role \
  --role-name RDSAccessRole \
  --assume-role-policy-document file://trust-policy.json

# Attach policy
aws iam attach-role-policy \
  --role-name RDSAccessRole \
  --policy-arn arn:aws:iam::123456789012:policy/RDSAccessPolicy
```

### 4. Environment Variables

Set up environment variables for IAM authentication:

```bash
# Database connection
export POSTGRES_HOST=my-aurora-cluster.cluster-xxxxx.us-east-1.rds.amazonaws.com
export POSTGRES_PORT=5432
export POSTGRES_DB=postgres
export POSTGRES_USER=your_admin_user
export POSTGRES_SSLMODE=require

# IAM Authentication
export POSTGRES_IAM_AUTH=true
export AWS_REGION=us-east-1

# AWS Credentials (if not using instance profile/role)
export AWS_ACCESS_KEY_ID=your_access_key
export AWS_SECRET_ACCESS_KEY=your_secret_key
```

## User Configuration

### Configuration File Examples

#### IAM Authentication User
```json
{
  "username": "iam_app_user",
  "groups": ["app_group"],
  "privileges": ["CONNECT"],
  "databases": ["myapp"],
  "enabled": true,
  "description": "Application user with IAM authentication",
  "auth_method": "iam",
  "iam_role": "arn:aws:iam::123456789012:role/RDSAccessRole",
  "can_login": true,
  "connection_limit": 20
}
```

#### Mixed Authentication Setup
```json
{
  "users": [
    {
      "username": "admin_user",
      "password": "secure_password",
      "auth_method": "password",
      "groups": ["admin_group"],
      "can_login": true,
      "description": "Admin user with password auth"
    },
    {
      "username": "app_user_iam",
      "auth_method": "iam",
      "groups": ["app_group"],
      "can_login": true,
      "description": "App user with IAM auth"
    }
  ]
}
```

## Usage Examples

### Creating IAM-Enabled Users

```bash
# Create user with IAM authentication
postgres-user-manager create-user iam_user \
  --auth-method iam \
  --groups "app_group" \
  --privileges "CONNECT" \
  --databases "myapp" \
  --iam-role "arn:aws:iam::123456789012:role/RDSAccessRole" \
  --description "IAM authenticated user"

# Create user with password authentication
postgres-user-manager create-user pwd_user \
  --auth-method password \
  --password "secure_password" \
  --groups "app_group"
```

### Synchronizing Configuration

```bash
# Dry run to preview changes
postgres-user-manager sync --config config.json --dry-run --verbose

# Apply configuration
postgres-user-manager sync --config config.json --verbose
```

## Important Considerations

### 1. SSL/TLS Requirements

IAM authentication **requires** SSL/TLS connections. The tool automatically sets `sslmode=require` when IAM authentication is enabled.

### 2. RDS IAM Role

Users created with IAM authentication automatically receive the `rds_iam` role, which is required for IAM database authentication in RDS.

### 3. Password Handling

- Users with `auth_method: "iam"` do not need passwords
- Passwords specified for IAM users are ignored
- Traditional password users still require passwords

### 4. Connection Limits

Set appropriate connection limits for IAM users since IAM token generation has API rate limits:

```json
{
  "username": "iam_user",
  "auth_method": "iam",
  "connection_limit": 10
}
```

### 5. Service Accounts

For service accounts that don't need direct login:

```json
{
  "username": "service_account",
  "auth_method": "iam",
  "can_login": false,
  "groups": ["app_group"]
}
```

## Application Integration

### 1. IAM Token Generation

In your application, generate IAM authentication tokens:

```go
// Example for Go applications
import (
    "github.com/aws/aws-sdk-go/aws/session"
    "github.com/aws/aws-sdk-go/service/rds/rdsutils"
)

func generateIAMToken(region, endpoint, username string) (string, error) {
    sess := session.Must(session.NewSession())
    return rdsutils.BuildAuthToken(endpoint, region, username, sess.Config.Credentials)
}
```

### 2. Connection String

Use the generated token as the password:

```go
connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=require",
    host, port, username, iamToken, database)
```

## Monitoring and Troubleshooting

### 1. Dry Run Mode

Always test with dry run first:

```bash
postgres-user-manager sync --config config.json --dry-run --verbose
```

### 2. Verbose Logging

Enable verbose logging for detailed operation information:

```bash
postgres-user-manager sync --config config.json --verbose
```

### 3. Common Issues

**SSL Connection Required**
- Ensure `POSTGRES_SSLMODE=require`
- Verify Aurora cluster has SSL enabled

**IAM Permission Denied**
- Check IAM policy allows `rds-db:connect`
- Verify IAM role has correct permissions
- Ensure user exists in database with `rds_iam` role

**Token Expiration**
- IAM tokens expire after 15 minutes
- Implement token refresh in applications
- Consider connection pooling strategies

## Security Best Practices

1. **Use IAM roles** instead of long-term credentials when possible
2. **Rotate IAM credentials** regularly
3. **Limit connection counts** for IAM users
4. **Use least privilege** principle for database permissions
5. **Monitor database access** through CloudTrail and RDS logs
6. **Enable encryption** at rest and in transit

## AWS CLI Helper Commands

```bash
# List clusters with IAM auth status
aws rds describe-db-clusters \
  --query 'DBClusters[*].[DBClusterIdentifier,IAMDatabaseAuthenticationEnabled]' \
  --output table

# Generate IAM token for testing
aws rds generate-db-auth-token \
  --hostname my-cluster.cluster-xxxxx.us-east-1.rds.amazonaws.com \
  --port 5432 \
  --username iam_user \
  --region us-east-1
```
