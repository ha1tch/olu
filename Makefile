.PHONY: build run clean test install deps fmt lint benchmark test-race test-unit test-integration

# Binary name
BINARY_NAME=olu
MIGRATE_BINARY=olu-migrate
MAIN_PATH=./cmd/olu
MIGRATE_PATH=./cmd/olu-migrate

# Build the application
build:
	@echo "Building ${BINARY_NAME}..."
	@go build -o ${BINARY_NAME} ${MAIN_PATH}
	@echo "Build complete: ${BINARY_NAME}"

# Build migration tool
build-migrate:
	@echo "Building ${MIGRATE_BINARY}..."
	@go build -o ${MIGRATE_BINARY} ${MIGRATE_PATH}
	@echo "Build complete: ${MIGRATE_BINARY}"

# Build all binaries
build-all-tools: build build-migrate
	@echo "All tools built successfully"

# Run the application
run: build
	@echo "Running ${BINARY_NAME}..."
	@./${BINARY_NAME}

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@go clean
	@rm -f ${BINARY_NAME}
	@rm -f ${MIGRATE_BINARY}
	@rm -rf data/*
	@rm -f *.db
	@echo "Clean complete"

# Run tests
test:
	@echo "Running tests..."
	@go test -v ./...

# Run tests with coverage
coverage:
	@echo "Running tests with coverage..."
	@go test -v -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run benchmarks
benchmark:
	@echo "Running benchmarks..."
	@go test -bench=. -benchmem ./pkg/server/
	@echo "Benchmark complete"

# Run benchmarks with longer duration
benchmark-long:
	@echo "Running benchmarks (5s each)..."
	@go test -bench=. -benchmem -benchtime=5s ./pkg/server/

# Run specific benchmark
benchmark-%:
	@echo "Running benchmark $*..."
	@go test -bench=$* -benchmem ./pkg/server/

# Run tests with race detector
test-race:
	@echo "Running tests with race detector..."
	@go test -race ./...

# Run unit tests only (storage layer)
test-unit:
	@echo "Running unit tests..."
	@go test -v ./pkg/storage/

# Run SQLite tests only
test-sqlite:
	@echo "Running SQLite tests..."
	@go test -v ./pkg/storage/ -run TestSQLite

# Run integration tests only
test-integration:
	@echo "Running integration tests..."
	@go test -v ./pkg/server/

# Quick test (no verbose, cached results ok)
test-quick:
	@go test ./...

# Generate test report in JSON
test-report:
	@echo "Generating test report..."
	@go test -v -json ./... > test-report.json
	@echo "Test report: test-report.json"

# All checks before commit
pre-commit: clean build test test-race
	@echo "✓ All pre-commit checks passed!"

# Install dependencies
deps:
	@echo "Installing dependencies..."
	@go mod download
	@go mod tidy

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...

# Run linter
lint:
	@echo "Running linter..."
	@golangci-lint run ./...

# Build for multiple platforms
build-all:
	@echo "Building for multiple platforms..."
	@GOOS=linux GOARCH=amd64 go build -o ${BINARY_NAME}-linux-amd64 ${MAIN_PATH}
	@GOOS=darwin GOARCH=amd64 go build -o ${BINARY_NAME}-darwin-amd64 ${MAIN_PATH}
	@GOOS=darwin GOARCH=arm64 go build -o ${BINARY_NAME}-darwin-arm64 ${MAIN_PATH}
	@GOOS=windows GOARCH=amd64 go build -o ${BINARY_NAME}-windows-amd64.exe ${MAIN_PATH}
	@echo "Multi-platform build complete"

# Install the binary
install: build
	@echo "Installing ${BINARY_NAME}..."
	@go install ${MAIN_PATH}

# Development mode with auto-reload (requires air)
dev:
	@which air > /dev/null || (echo "Installing air..." && go install github.com/cosmtrek/air@latest)
	@air

# Docker build
docker-build:
	@echo "Building Docker image..."
	@docker build -t olu:latest .

# Docker run
docker-run:
	@echo "Running Docker container..."
	@docker run -p 9090:9090 -v $(PWD)/data:/app/data olu:latest

# Help
help:
	@echo "Available targets:"
	@echo ""
	@echo "Build & Run:"
	@echo "  build           - Build the application"
	@echo "  build-migrate   - Build migration tool"
	@echo "  build-all-tools - Build all binaries"
	@echo "  run             - Build and run the application"
	@echo "  clean           - Remove build artifacts and data"
	@echo "  install         - Install binary"
	@echo "  build-all       - Build for multiple platforms"
	@echo ""
	@echo "Testing:"
	@echo "  test            - Run all tests"
	@echo "  test-unit       - Run unit tests (storage layer)"
	@echo "  test-sqlite     - Run SQLite tests only"
	@echo "  test-integration - Run integration tests (server)"
	@echo "  test-race       - Run tests with race detector"
	@echo "  test-quick      - Quick test run (cached results)"
	@echo "  test-report     - Generate JSON test report"
	@echo "  coverage        - Run tests with coverage report"
	@echo ""
	@echo "Benchmarking:"
	@echo "  benchmark       - Run all benchmarks"
	@echo "  benchmark-long  - Run benchmarks with 5s duration"
	@echo "  benchmark-NAME  - Run specific benchmark"
	@echo ""
	@echo "Development:"
	@echo "  deps        - Install dependencies"
	@echo "  fmt         - Format code"
	@echo "  lint        - Run linter"
	@echo "  dev         - Run in development mode with auto-reload"
	@echo "  pre-commit  - Run all checks before committing"
	@echo ""
	@echo "Docker:"
	@echo "  docker-build - Build Docker image"
	@echo "  docker-run   - Run Docker container"
	@echo ""
	@echo "  help        - Show this help message"
