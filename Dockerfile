# Multi-stage build
FROM golang:1.24 AS builder

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o postgres-user-manager main.go

# Final stage - using debian slim for better compatibility
FROM debian:bookworm-slim

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
