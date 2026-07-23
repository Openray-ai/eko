# Ekō Project Structure

This document describes the module structure and architecture of Ekō.

## Directory Structure

```
ekō/
├── cmd/                    # Application entry points
│   └── eko/               # Main application
│       └── main.go        # Entry point with initialization
│
├── internal/              # Private application code
│   ├── api/              # HTTP API layer
│   │   ├── handlers/     # HTTP request handlers
│   │   │   ├── sanitize.go    # Core sanitization endpoint
│   │   │   └── health.go      # Health check endpoint
│   │   ├── middleware/   # HTTP middleware
│   │   │   ├── logger.go      # Request logging
│   │   │   ├── cors.go        # CORS handling
│   │   │   └── recovery.go    # Panic recovery
│   │   └── routes/       # Route definitions
│   │       └── routes.go      # Route setup and registration
│   │
│   ├── core/             # Core business logic
│   │   ├── detector/     # Pattern detection engine
│   │   │   ├── detector.go    # Main detection logic
│   │   │   └── detector_test.go
│   │   ├── sanitizer/    # Sanitization logic
│   │   │   └── sanitizer.go   # Redaction and replacement
│   │   └── patterns/     # Pattern management
│   │       ├── pattern.go     # Pattern definitions
│   │       └── loader.go      # Pattern loading from YAML
│   │
│   ├── config/           # Configuration management
│   │   └── config.go     # Configuration structs and loading
│   │
│   └── proxy/            # Proxy implementations
│       ├── common/       # Shared proxy logic
│       │   └── proxy.go       # Base proxy interface
│       ├── openai/       # OpenAI proxy
│       │   └── openai.go      # OpenAI-specific handling
│       ├── anthropic/    # Anthropic proxy
│       │   └── anthropic.go   # Anthropic-specific handling
│       ├── gemini/       # Gemini proxy
│       │   └── gemini.go      # Gemini-specific handling
│       ├── deepseek/     # DeepSeek proxy
│       │   └── deepseek.go    # DeepSeek-specific handling
│       └── router/       # Provider-neutral model router
│           └── router.go      # Public proxy route handling
│
├── pkg/                   # Public library code (reusable)
│   └── (future public packages)
│
├── configs/              # Configuration templates
│   └── config.example.yaml    # Example configuration
│
├── patterns/             # Pattern definitions
│   ├── default/          # Built-in patterns
│   │   └── patterns.yaml      # Default pattern library
│   └── custom/           # User-defined patterns
│       └── .gitkeep
│
├── test/                 # Test utilities and integration tests
│   └── README.md         # Testing documentation
│
├── docs/                 # Documentation
│   └── (future documentation)
│
├── scripts/              # Build and deployment scripts
│   └── (future scripts)
│
├── go.mod                # Go module definition
├── go.sum                # Go dependencies lock
├── Makefile              # Build automation
├── .gitignore            # Git ignore rules
├── README.md             # Project README
├── technical.md          # Implementation plan
└── PROJECT_STRUCTURE.md  # This file
```

## Architecture Overview

### Layers

1. **Entry Point (`cmd/`)**: Application initialization and startup
2. **API Layer (`internal/api/`)**: HTTP handling, routing, middleware
3. **Core Layer (`internal/core/`)**: Business logic, detection, sanitization
4. **Proxy Layer (`internal/proxy/`)**: Provider-specific proxying
5. **Configuration (`internal/config/`)**: Configuration management

### Data Flow

```
HTTP Request
    ↓
[Middleware] → Logger, CORS, Recovery
    ↓
[Handler] → Parse and validate request
    ↓
[Sanitizer] → Orchestrate sanitization
    ↓
[Detector] → Pattern matching
    ↓
[Response] → Return sanitized output
```

### Proxy Data Flow

```
HTTP Request (OpenAI/Anthropic/Google format)
    ↓
[Proxy Handler] → Extract prompt from provider format
    ↓
[Sanitizer] → Detect and sanitize sensitive data
    ↓
[Proxy Client] → Forward sanitized request to provider
    ↓
[Response Handler] → Add violation headers, return response
```

## Key Components

### 1. Detector (`internal/core/detector/`)

**Purpose**: Pattern matching engine for detecting sensitive data

**Key Files**:
- `detector.go`: Core detection logic with concurrent pattern matching
- `detector_test.go`: Unit tests

**Responsibilities**:
- Load and compile regex patterns
- Scan input text for matches
- Return violations with metadata (type, severity, position)

### 2. Sanitizer (`internal/core/sanitizer/`)

**Purpose**: Redact or replace sensitive data

**Key Files**:
- `sanitizer.go`: Sanitization orchestration

**Responsibilities**:
- Coordinate with detector to find violations
- Apply redaction strategies (REDACT, REPLACE, etc.)
- Return sanitized output and violation details

### 3. Patterns (`internal/core/patterns/`)

**Purpose**: Pattern definition and loading

**Key Files**:
- `pattern.go`: Pattern struct definitions
- `loader.go`: Load patterns from YAML files

**Responsibilities**:
- Define pattern schema (name, regex, type, severity)
- Load patterns from default and custom directories
- Compile patterns into detector-ready format

### 4. Config (`internal/config/`)

**Purpose**: Application configuration

**Key Files**:
- `config.go`: Configuration structs and loading logic

**Responsibilities**:
- Load YAML configuration
- Provide default configuration
- Validate configuration values

### 5. API Handlers (`internal/api/handlers/`)

**Purpose**: HTTP request handling

**Key Files**:
- `sanitize.go`: Core sanitization API endpoint
- `health.go`: Health check endpoint

**Responsibilities**:
- Parse and validate HTTP requests
- Call core services
- Format HTTP responses

### 6. Middleware (`internal/api/middleware/`)

**Purpose**: HTTP request/response processing

**Key Files**:
- `logger.go`: Request logging
- `cors.go`: CORS headers
- `recovery.go`: Panic recovery

**Responsibilities**:
- Cross-cutting concerns (logging, errors, security)
- Request/response modification
- Error handling

### 7. Proxy (`internal/proxy/`)

**Purpose**: Provider-specific API proxying

**Key Files**:
- `common/proxy.go`: Shared proxy interface
- `openai/openai.go`: OpenAI-specific logic
- `anthropic/anthropic.go`: Anthropic-specific logic
- `gemini/gemini.go`: Gemini-specific logic
- `deepseek/deepseek.go`: DeepSeek-specific logic
- `router/router.go`: Provider-neutral model routing and public proxy handlers

**Responsibilities**:
- Extract prompts from provider-specific formats
- Forward sanitized requests to providers
- Add violation metadata to responses
- Handle streaming responses

## Design Principles

### 1. Separation of Concerns
Each package has a single, well-defined responsibility.

### 2. Dependency Direction
Dependencies flow inward:
```
API → Core Services → Utilities
```

### 3. Testability
All packages are independently testable with clear interfaces.

### 4. Internal vs Public
- `internal/`: Private to this application, cannot be imported by external projects
- `pkg/`: Public packages (future) that can be imported externally

### 5. Configuration Over Code
Patterns and behavior are configurable via YAML, not hardcoded.

## Adding New Features

### Adding a New Pattern
1. Add pattern definition to `patterns/default/patterns.yaml`
2. Or create custom file in `patterns/custom/`

### Adding a New Provider Proxy
1. Create new directory in `internal/proxy/`
2. Implement provider-specific request/response handling
3. Register routes in `internal/api/routes/`

### Adding a New API Endpoint
1. Create handler in `internal/api/handlers/`
2. Register route in `internal/api/routes/routes.go`

### Adding Configuration Options
1. Update structs in `internal/config/config.go`
2. Update `configs/config.example.yaml`
3. Handle new config in relevant components

## Testing Strategy

### Unit Tests
- Located alongside source files (`*_test.go`)
- Test individual functions and methods
- Use table-driven tests for multiple cases

### Integration Tests
- Located in `test/` directory
- Test end-to-end flows
- Test with real pattern files

### Running Tests
```bash
# All tests
make test

# With coverage
make test-coverage

# Specific package
go test ./internal/core/detector/...
```

## Build and Run

### Development
```bash
# Run with auto-reload
make run

# Run with config
make run-config

# Build binary
make build
```

### Production
```bash
# Build for current platform
make build

# Build for all platforms
make build-all

# Docker
make docker-build
make docker-run
```

## Future Enhancements

### Planned Additions
- `pkg/client/`: Go client library for Ekō API
- `internal/metrics/`: Prometheus metrics collection
- `internal/alerts/`: Alert notification system
- `docs/`: API documentation, architecture diagrams
- `scripts/`: CI/CD, deployment, migration scripts

### Scalability Considerations
- Pattern caching for performance
- Concurrent request processing
- Streaming response handling
- Connection pooling for provider requests

## Questions?

For implementation details, see:
- `technical.md`: Full implementation roadmap
- `README.md`: User-facing documentation
- Package-specific `README.md` files
