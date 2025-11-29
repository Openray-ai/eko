.PHONY: build run test test-integration test-all clean install lint fmt help

# Binary name
BINARY_NAME=eko
BUILD_DIR=bin

# Build the application
build:
	@echo "Building $(BINARY_NAME)..."
	@go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/eko
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Run the application
run:
	@echo "Running $(BINARY_NAME)..."
	@go run ./cmd/eko

# Run with config
run-config:
	@echo "Running $(BINARY_NAME) with config..."
	@EKO_CONFIG=configs/config.example.yaml go run ./cmd/eko

# Run tests (excludes integration tests)
test:
	@echo "Running unit tests..."
	@go test -v $$(go list ./... | grep -v /test)

# Run integration tests
test-integration:
	@echo "Running integration tests..."
	@echo "Make sure the server is running on localhost:8080"
	@EKO_INTEGRATION_TEST=true go test -v ./test/

# Run all tests (unit + integration)
test-all:
	@echo "Running all tests..."
	@$(MAKE) test
	@echo ""
	@$(MAKE) test-integration

# Run tests with coverage (excludes integration tests)
test-coverage:
	@echo "Running tests with coverage..."
	@go test -cover -coverprofile=coverage.out $$(go list ./... | grep -v /test)
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html
	@echo "Clean complete"

# Install dependencies
install:
	@echo "Installing dependencies..."
	@go mod download
	@go mod tidy

# Lint code
lint:
	@echo "Running linters..."
	@go vet ./...
	@gofmt -l .

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...

# Build for multiple platforms
build-all:
	@echo "Building for multiple platforms..."
	@GOOS=linux GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/eko
	@GOOS=darwin GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/eko
	@GOOS=darwin GOARCH=arm64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/eko
	@GOOS=windows GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./cmd/eko
	@echo "Multi-platform build complete"

# Docker build
docker-build:
	@echo "Building Docker image..."
	@docker build -t openray/eko:latest .

# Docker run
docker-run:
	@echo "Running Docker container..."
	@docker run -p 8080:8080 openray/eko:latest

# Help
help:
	@echo "Available targets:"
	@echo "  build            - Build the application"
	@echo "  run              - Run the application"
	@echo "  run-config       - Run with example config"
	@echo "  test             - Run unit tests (excludes integration tests)"
	@echo "  test-integration - Run integration tests (requires server running)"
	@echo "  test-all         - Run all tests (unit + integration)"
	@echo "  test-coverage    - Run tests with coverage report"
	@echo "  clean            - Clean build artifacts"
	@echo "  install          - Install dependencies"
	@echo "  lint             - Run linters"
	@echo "  fmt              - Format code"
	@echo "  build-all        - Build for multiple platforms"
	@echo "  docker-build     - Build Docker image"
	@echo "  docker-run       - Run Docker container"
	@echo "  help             - Show this help message"
