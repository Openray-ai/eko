FROM golang:1.24.1-alpine AS builder

# Install build dependencies
# - git: required for go mod download
# - ca-certificates: for HTTPS during build
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /build

# ============================================================================
# OPTIMIZATION: Copy go.mod and go.sum FIRST for better layer caching
# This allows Docker to cache the dependencies layer separately from code
# Dependencies change ~1% as often as source code, so 99% of builds
# will reuse this cached layer instead of re-downloading dependencies
# ============================================================================
COPY go.mod go.sum ./

# Download dependencies (this layer will be cached unless go.mod/go.sum change)
RUN go mod download && go mod verify


COPY . .

# ============================================================================
# Build the binary with optimizations
# CGO_ENABLED=0: Static binary (no C dependencies, portable across any Linux)
# GOOS=linux GOARCH=amd64: Target platform
# -ldflags="-s -w": Strip debug info to reduce binary size (~30% reduction)
#   -s: strip symbol table
#   -w: strip DWARF debugging info
# -trimpath: Remove file system paths from binary (security & reproducibility)
# ============================================================================
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -trimpath \
    -o /build/bin/eko \
    ./cmd/eko


FROM alpine:3.19

# Install runtime dependencies
# - ca-certificates: for HTTPS API calls to OpenAI/Anthropic/Google
# - tzdata: for correct timezone handling in logs
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user for security
# - UID 1000 is conventional for application users (matches most dev environments)
# - No password, no shell access (/sbin/nologin)
# - Prevents container escape attacks
# - Required for Kubernetes security contexts
RUN addgroup -g 1000 eko && \
    adduser -D -u 1000 -G eko -s /sbin/nologin eko

# Set working directory
WORKDIR /app

# Create required directories with proper permissions
RUN mkdir -p /app/configs /app/patterns /app/reports && \
    chown -R eko:eko /app

# Copy binary from builder stage
COPY --from=builder --chown=eko:eko /build/bin/eko /app/eko

# Copy default patterns (these are part of the application)
COPY --from=builder --chown=eko:eko /build/patterns /app/patterns

# Copy example config for reference (actual config will be volume-mounted)
COPY --from=builder --chown=eko:eko /build/configs/config.example.yaml /app/configs/

# Switch to non-root user
USER eko

# Expose application port
EXPOSE 8080

# Set environment variable for config location
# Can be overridden at runtime via docker-compose or docker run
ENV EKO_CONFIG=/app/configs/config.yaml

# Health check configuration
# - Checks /health endpoint every 30s
# - 3s timeout per check
# - Start checking after 5s (allow startup time)
# - 3 consecutive failures = unhealthy
# - Uses wget (included in Alpine) for HTTP check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the binary
# Note: Config, patterns, and reports directories should be mounted as volumes
ENTRYPOINT ["/app/eko"]
