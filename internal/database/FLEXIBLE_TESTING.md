# Flexible Integration Testing Setup

This document describes the flexible integration testing setup that automatically adapts to different Docker environments.

## Overview

The `SetupFlexibleTestDatabase()` function provides an intelligent test setup that:

1. **Automatically detects your Docker environment** (Colima, Docker Desktop, Lima, Podman, etc.)
2. **Configures testcontainers appropriately** for each environment
3. **Handles common compatibility issues** (like ryuk with Colima)
4. **Provides retry logic** for database connections
5. **Ensures consistent behavior** across different development setups

## Supported Docker Environments

### Colima
- **Detection**: Checks for `.colima` in Docker socket path or `DOCKER_HOST`
- **Configuration**: Automatically disables ryuk to avoid socket path issues
- **Socket**: Usually at `~/.colima/default/docker.sock`

### Docker Desktop
- **Detection**: Standard Docker socket at `/var/run/docker.sock` on macOS
- **Configuration**: Uses default testcontainers settings (with optional ryuk disable)
- **Socket**: `/var/run/docker.sock`

### Lima
- **Detection**: Checks for `.lima` in Docker socket path
- **Configuration**: Disables ryuk (similar issues to Colima)
- **Socket**: Usually at `~/.lima/default/docker.sock`

### Podman
- **Detection**: Checks for `podman` in Docker socket path
- **Configuration**: Disables ryuk for compatibility
- **Socket**: Varies by Podman configuration

### Unknown Environments
- **Detection**: Falls back when specific environment can't be identified
- **Configuration**: Conservatively disables ryuk for maximum compatibility

## Usage

### Basic Test Setup

```go
func TestSomething(t *testing.T) {
    setup := SetupFlexibleTestDatabase(t)
    defer setup.Cleanup(t)
    
    // Your test code here
    exists, err := setup.Manager.UserExists("test_user")
    // ...
}
```

### Environment Configuration

The setup automatically:

1. **Detects your environment** and logs the detection result
2. **Sets appropriate environment variables** for testcontainers
3. **Configures retry logic** for database connections
4. **Forces IPv4** to avoid IPv6 connection issues

### Manual Override

You can override the automatic detection by setting environment variables:

```bash
# Force disable ryuk
export TESTCONTAINERS_RYUK_DISABLED=true

# Keep ryuk disabled persistently (won't be unset after tests)
export TESTCONTAINERS_PERSIST_RYUK_DISABLED=true

# Force no ryuk preference even for Docker Desktop
export TESTCONTAINERS_PREFER_NO_RYUK=true
```

## Environment Detection Logic

The detection follows this priority order:

1. **Check `DOCKER_HOST` environment variable** for specific patterns
2. **Check filesystem** for Docker socket files in standard locations
3. **Use platform-specific defaults** (e.g., `/var/run/docker.sock` on macOS)
4. **Fall back to "unknown"** with conservative settings

### Detection Examples

```go
// Colima detection
DOCKER_HOST=unix:///Users/user/.colima/default/docker.sock
// Results in: Type="colima", ryuk disabled

// Docker Desktop detection  
DOCKER_HOST="" (empty) and /var/run/docker.sock exists
// Results in: Type="docker-desktop", ryuk enabled (unless overridden)

// Lima detection
DOCKER_HOST=unix:///Users/user/.lima/default/docker.sock
// Results in: Type="lima", ryuk disabled
```

## Connection Reliability

The setup includes several reliability features:

### Retry Logic
- **3 attempts** to connect to the database
- **1-second delay** between attempts
- **Detailed logging** of connection failures

### IPv4 Preference
- **Forces IPv4** by converting "localhost" to "127.0.0.1"
- **Avoids IPv6 connection issues** common with containerized databases

### Extended Wait Time
- **2-second delay** after container reports ready
- **Ensures database is fully initialized** before connection attempts

## Troubleshooting

### Tests Fail with "connection refused"
1. **Check Docker status**: `docker ps`
2. **Verify testcontainers logs** in test output
3. **Try manual ryuk disable**: `export TESTCONTAINERS_RYUK_DISABLED=true`

### Environment not detected correctly
1. **Check detection logs** in test output
2. **Verify Docker socket path**: `echo $DOCKER_HOST`
3. **Run detection test**: `go test -run TestDockerEnvironmentDetection`

### Slow test startup
1. **Normal for first run** (downloads PostgreSQL image)
2. **Subsequent runs should be faster** (image cached)
3. **Consider CI-specific optimizations** if needed

## Migration from Previous Setups

### From SetupTestDatabase
Replace:
```go
setup := SetupTestDatabase(t)
```

With:
```go
setup := SetupFlexibleTestDatabase(t)
```

### From SetupColimaTestDatabase
Replace:
```go
setup := SetupColimaTestDatabase(t)
```

With:
```go
setup := SetupFlexibleTestDatabase(t)
```

The API is identical, only the setup function changes.

## Best Practices

1. **Use flexible setup by default** for new tests
2. **Let automatic detection work** rather than manual configuration
3. **Check test logs** to verify correct environment detection
4. **Use environment variables** for CI-specific overrides
5. **Keep retry logic intact** for connection reliability

## Future Enhancements

Potential improvements to consider:

1. **More sophisticated ryuk compatibility checking**
2. **Automatic fallback to local PostgreSQL** if containers fail
3. **Performance optimizations** for CI environments
4. **Support for additional Docker alternatives**
5. **Custom wait strategies** per environment type
