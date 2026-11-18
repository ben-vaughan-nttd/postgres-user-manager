# Multi-stage build
# The builder (golang:<version>, Debian trixie) and the runtime image must share
# a glibc: pg_query_go requires CGO, so the binary is dynamically linked.
FROM golang:1.27.4 AS builder

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application (CGO is required by pg_query_go)
RUN CGO_ENABLED=1 GOOS=linux go build -o postgres-user-manager main.go

# Final stage - Debian trixie slim matches the builder's glibc
FROM debian:trixie-slim

# Install ca-certificates for HTTPS calls
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*

WORKDIR /root/

# Copy the binary from builder stage
COPY --from=builder /app/postgres-user-manager .

# Copy example configuration
COPY config.example.json .

# Create a non-root user
RUN useradd -r -s /bin/false appuser
USER appuser

# Set entrypoint
ENTRYPOINT ["./postgres-user-manager"]

# Default command
CMD ["--help"]
