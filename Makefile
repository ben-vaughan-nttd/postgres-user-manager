.PHONY: all build build-all build-all-native check-go-version test-go-version check clean test fmt vet lint run-example install deps

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
all: deps fmt check test build

# Verify the Docker builder uses the module's preferred Go toolchain.
check-go-version:
	@scripts/check-go-version.sh Dockerfile go.mod

test-go-version:
	@scripts/check-go-version-test.sh

check: check-go-version test-go-version vet

# Install dependencies
deps:
	$(GOGET) -d ./...
	$(GOCMD) mod tidy

# Build the application
build: check-go-version
	mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME) main.go

# Build for multiple platforms.
#
# pg_query_go embeds the PostgreSQL parser in C, so every target needs CGO and a
# C cross compiler. The matrix is linux amd64/arm64, darwin amd64/arm64 and
# windows amd64 (386 is not built). build-all runs build-all-native inside the
# goreleaser-cross image, which provides all of the compilers below; run
# build-all-native directly only where equivalent toolchains are on PATH.
CROSS_IMAGE ?= ghcr.io/goreleaser/goreleaser-cross:v1.27
CC_LINUX_AMD64 ?= x86_64-linux-gnu-gcc
CC_LINUX_ARM64 ?= aarch64-linux-gnu-gcc
CC_DARWIN_AMD64 ?= o64-clang
CC_DARWIN_ARM64 ?= oa64-clang
CC_WINDOWS_AMD64 ?= x86_64-w64-mingw32-gcc

build-all:
	docker run --rm \
		--entrypoint make \
		-v "$(CURDIR)":/work -w /work \
		-e GOFLAGS=-buildvcs=false \
		$(CROSS_IMAGE) build-all-native VERSION="$(VERSION)"

build-all-native:
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=1 CC=$(CC_LINUX_AMD64) GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-linux-amd64 main.go
	CGO_ENABLED=1 CC=$(CC_LINUX_ARM64) GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-linux-arm64 main.go
	CGO_ENABLED=1 CC=$(CC_DARWIN_AMD64) GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-darwin-amd64 main.go
	CGO_ENABLED=1 CC=$(CC_DARWIN_ARM64) GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-darwin-arm64 main.go
	CGO_ENABLED=1 CC=$(CC_WINDOWS_AMD64) GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-windows-amd64.exe main.go

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
	@echo "  build-all     - Build all release platforms (CGO, runs in the goreleaser-cross image)"
	@echo "  build-all-native - Build all release platforms with local cross compilers"
	@echo "  test          - Run tests"
	@echo "  test-coverage - Run tests with coverage"
	@echo "  fmt           - Format code"
	@echo "  vet           - Vet code"
	@echo "  check-go-version - Verify Docker and Go toolchain versions"
	@echo "  test-go-version  - Test Go version validation failure paths"
	@echo "  lint          - Lint code (requires golangci-lint)"
	@echo "  install       - Install the application"
	@echo "  clean         - Clean build artifacts"
	@echo "  deps          - Install dependencies"
	@echo "  run-example   - Build and run example commands"
	@echo "  dev-setup     - Set up development environment"
	@echo "  dev           - Quick development build and test"
	@echo "  docker-build  - Build Docker image"
	@echo "  help          - Show this help"
