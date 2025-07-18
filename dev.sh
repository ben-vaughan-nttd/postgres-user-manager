#!/bin/bash

# PostgreSQL User Manager Development Helper Script

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Print colored output
print_status() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Help function
show_help() {
    echo "PostgreSQL User Manager Development Helper"
    echo ""
    echo "Usage: $0 [COMMAND]"
    echo ""
    echo "Commands:"
    echo "  setup       - Set up development environment"
    echo "  build       - Build the application"
    echo "  test        - Run tests"
    echo "  validate    - Validate example configuration"
    echo "  clean       - Clean build artifacts"
    echo "  docker      - Build Docker image"
    echo "  format      - Format code"
    echo "  help        - Show this help"
    echo ""
    echo "Examples:"
    echo "  $0 setup"
    echo "  $0 build"
    echo "  $0 test"
}

# Setup development environment
setup_dev() {
    print_status "Setting up development environment..."
    
    # Install dependencies
    print_status "Installing Go dependencies..."
    go mod tidy
    
    # Copy environment file if it doesn't exist
    if [ ! -f .env ]; then
        print_status "Creating .env file from template..."
        cp .env.example .env
        print_warning "Please update .env with your database credentials"
    fi
    
    # Copy config file if it doesn't exist
    if [ ! -f config.json ]; then
        print_status "Creating config.json from example..."
        cp config.example.json config.json
        print_warning "Please review and update config.json as needed"
    fi
    
    print_status "Development environment setup complete!"
    print_status "Next steps:"
    echo "  1. Update .env with your database credentials"
    echo "  2. Review config.json"
    echo "  3. Run: $0 build"
}

# Build application
build_app() {
    print_status "Building application..."
    go build -o postgres-user-manager main.go
    print_status "Build complete! Binary: postgres-user-manager"
}

# Run tests
run_tests() {
    print_status "Running tests..."
    go test -v ./...
    print_status "Tests complete!"
}

# Validate configuration
validate_config() {
    print_status "Validating example configuration..."
    if [ ! -f postgres-user-manager ]; then
        print_warning "Binary not found, building first..."
        build_app
    fi
    ./postgres-user-manager validate --config config.example.json
    print_status "Validation complete!"
}

# Clean build artifacts
clean_build() {
    print_status "Cleaning build artifacts..."
    rm -f postgres-user-manager
    rm -rf build/
    rm -f coverage.out coverage.html
    print_status "Clean complete!"
}

# Build Docker image
build_docker() {
    print_status "Building Docker image..."
    docker build -t postgres-user-manager:latest .
    print_status "Docker build complete!"
}

# Format code
format_code() {
    print_status "Formatting code..."
    go fmt ./...
    print_status "Code formatting complete!"
}

# Main script logic
case "${1:-help}" in
    setup)
        setup_dev
        ;;
    build)
        build_app
        ;;
    test)
        run_tests
        ;;
    validate)
        validate_config
        ;;
    clean)
        clean_build
        ;;
    docker)
        build_docker
        ;;
    format)
        format_code
        ;;
    help|*)
        show_help
        ;;
esac
