.PHONY: build clean test fmt vet lint run-example install deps

# Build configuration
APP_NAME := postgres-user-manager
BUILD_DIR := build
VERSION := $(shell git describe --tags --always --dirty)
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

# Go commands
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOCLEAN := $(GOCMD) clean
GOGET := $(GOCMD) get
GOFMT := $(GOCMD) fmt
GOVET := $(GOCMD) vet

# Default target
all: deps fmt vet test build

# Install dependencies
deps:
	$(GOGET) -d ./...
	$(GOCMD) mod tidy

# Build the application
build:
	mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME) main.go

# Build for multiple platforms
build-all:
	mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-linux-amd64 main.go
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-darwin-amd64 main.go
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-darwin-arm64 main.go
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-windows-amd64.exe main.go

# Run tests
test:
	$(GOTEST) -v ./...

# Run tests with coverage
test-coverage:
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

# Format code
fmt:
	$(GOFMT) ./...

# Vet code
vet:
	$(GOVET) ./...

# Install the application
install:
	$(GOCMD) install $(LDFLAGS) .

# Clean build artifacts
clean:
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

# Run example commands
run-example:
	@echo "Building application..."
	@make build
	@echo "\nValidating example configuration..."
	@./$(BUILD_DIR)/$(APP_NAME) validate --config config.example.json
	@echo "\nShowing help..."
	@./$(BUILD_DIR)/$(APP_NAME) --help

# Development setup
dev-setup:
	@echo "Setting up development environment..."
	$(GOGET) -u golang.org/x/tools/cmd/goimports
	$(GOGET) -u github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Lint code (requires golangci-lint)
lint:
	golangci-lint run

# Quick development build and test
dev: fmt vet test build

# Docker build
docker-build:
	docker build -t $(APP_NAME):$(VERSION) .

# Help
help:
	@echo "Available targets:"
	@echo "  build         - Build the application"
	@echo "  build-all     - Build for multiple platforms"
	@echo "  test          - Run tests"
	@echo "  test-coverage - Run tests with coverage"
	@echo "  fmt           - Format code"
	@echo "  vet           - Vet code"
	@echo "  lint          - Lint code (requires golangci-lint)"
	@echo "  install       - Install the application"
	@echo "  clean         - Clean build artifacts"
	@echo "  deps          - Install dependencies"
	@echo "  run-example   - Build and run example commands"
	@echo "  dev-setup     - Set up development environment"
	@echo "  dev           - Quick development build and test"
	@echo "  docker-build  - Build Docker image"
	@echo "  help          - Show this help"
