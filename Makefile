.PHONY: build run test test-integration test-all clean install lint fmt help docker-build docker-run docker-up docker-down docker-logs docker-test docker-push docker-clean bench bench-save bench-compare bench-profile-cpu bench-profile-mem bench-memory bench-ceiling

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

# Docker build (with BuildKit for better caching and performance)
docker-build:
	@echo "Building Docker image..."
	@DOCKER_BUILDKIT=1 docker build -t openray/eko:latest .
	@echo "Docker image built: openray/eko:latest"

# Docker run (with volume mounts for config, patterns, and reports)
docker-run:
	@echo "Running Docker container..."
	@docker run --rm \
		-p 8080:8080 \
		-v $(PWD)/configs/config.yaml:/app/configs/config.yaml:ro \
		-v $(PWD)/patterns/custom:/app/patterns/custom:ro \
		-v $(PWD)/reports:/app/reports:rw \
		openray/eko:latest

# Start services with docker-compose
docker-up:
	@echo "Starting services with docker-compose..."
	@docker compose up -d
	@echo "Services started. Use 'make docker-logs' to view logs"

# Stop docker-compose services
docker-down:
	@echo "Stopping services..."
	@docker compose down

# View docker-compose logs
docker-logs:
	@docker compose logs -f eko

# Test Docker setup (build and verify)
docker-test:
	@echo "Testing Docker setup..."
	@echo "1. Building image..."
	@$(MAKE) docker-build
	@echo "2. Checking image size..."
	@docker images openray/eko:latest --format "{{.Size}}"
	@echo "3. Verifying image was built successfully"
	@docker images openray/eko:latest
	@echo "Docker test complete. Run 'make docker-up' to start the service"

# Push to Docker registry
docker-push:
	@echo "Pushing to Docker registry..."
	@docker push openray/eko:latest

# Clean Docker artifacts (remove images and containers)
docker-clean:
	@echo "Cleaning Docker artifacts..."
	@docker compose down -v 2>/dev/null || true
	@docker rmi openray/eko:latest 2>/dev/null || true
	@echo "Docker cleanup complete"

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
	@echo ""
	@echo "Docker targets:"
	@echo "  docker-build     - Build Docker image with BuildKit"
	@echo "  docker-run       - Run Docker container with volume mounts"
	@echo "  docker-up        - Start services with docker-compose"
	@echo "  docker-down      - Stop docker-compose services"
	@echo "  docker-logs      - View docker-compose logs"
	@echo "  docker-test      - Test Docker setup (build and verify)"
	@echo "  docker-push      - Push image to Docker registry"
	@echo "  docker-clean     - Remove Docker images and containers"
	@echo ""
	@echo "Benchmark targets:"
	@echo "  bench            - Run all micro-benchmarks with -benchmem -count=5"
	@echo "  bench-save       - Run benchmarks and save timestamped results"
	@echo "  bench-compare    - Compare baseline.txt vs current.txt using benchstat"
	@echo "  bench-profile-cpu - Generate and open CPU profile (pprof)"
	@echo "  bench-profile-mem - Generate and open memory profile (pprof)"
	@echo "  bench-memory     - Run memory ceiling tests (TestMemory*)"
	@echo "  bench-ceiling    - Run 512MB boundary tests (TestMemoryCeiling*)"
	@echo ""
	@echo "  help             - Show this help message"

# ---- Benchmark targets ----

BENCH_PKGS=./internal/core/tokenizer/ ./internal/core/detector/ ./internal/core/sanitizer/
BENCH_DIR=benchmarks

# Run all micro-benchmarks
bench:
	@echo "Running micro-benchmarks..."
	@go test -bench=. -benchmem -count=5 $(BENCH_PKGS)

# Run benchmarks and save timestamped results
bench-save:
	@mkdir -p $(BENCH_DIR)
	@echo "Running benchmarks and saving results..."
	@go test -bench=. -benchmem -count=5 $(BENCH_PKGS) | tee $(BENCH_DIR)/bench-$$(date +%Y%m%d-%H%M%S).txt
	@echo "Results saved to $(BENCH_DIR)/"

# Compare baseline vs current using benchstat
bench-compare:
	@if [ ! -f $(BENCH_DIR)/baseline.txt ] || [ ! -f $(BENCH_DIR)/current.txt ]; then \
		echo "Error: $(BENCH_DIR)/baseline.txt and $(BENCH_DIR)/current.txt must both exist."; \
		echo "  1. Run 'make bench-save' and copy the result to $(BENCH_DIR)/baseline.txt"; \
		echo "  2. Make changes, run 'make bench-save' and copy to $(BENCH_DIR)/current.txt"; \
		exit 1; \
	fi
	@benchstat $(BENCH_DIR)/baseline.txt $(BENCH_DIR)/current.txt

# Generate and open CPU profile
bench-profile-cpu:
	@echo "Generating CPU profile..."
	@go test -bench=. -benchmem -cpuprofile=$(BENCH_DIR)/cpu.prof ./internal/core/tokenizer/
	@go tool pprof -http=:8081 $(BENCH_DIR)/cpu.prof

# Generate and open memory profile
bench-profile-mem:
	@echo "Generating memory profile..."
	@go test -bench=. -benchmem -memprofile=$(BENCH_DIR)/mem.prof ./internal/core/tokenizer/
	@go tool pprof -http=:8081 $(BENCH_DIR)/mem.prof

# Run memory ceiling tests
bench-memory:
	@echo "Running memory tests..."
	@go test -v -run TestMemory -timeout 10m ./benchmarks/

# Run 512MB boundary tests
bench-ceiling:
	@echo "Running 512MB ceiling tests..."
	@go test -v -run TestMemoryCeiling -timeout 30m ./benchmarks/
