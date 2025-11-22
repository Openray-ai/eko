# Logger Implementation Guide

This document provides a complete guide to the logger implementation in Ekō.

## Overview

Ekō includes a production-ready structured logging system with the following features:

- **Multiple Log Levels**: Debug, Info, Warn, Error, Fatal
- **Structured Logging**: Add contextual fields to any log message
- **Flexible Output**: Text format for development, JSON for production
- **Thread-Safe**: Safe for concurrent use across goroutines
- **Zero Dependencies**: Built using only Go standard library
- **Configuration-Driven**: Fully configurable via YAML

## Quick Start

### Basic Usage

```go
import "eko/internal/helpers/logger"

// Simple logging
logger.Info("Server started")
logger.Error("Database connection failed")
logger.Debug("Processing request")

// Formatted logging
logger.Infof("Server listening on port %d", 8080)
logger.Errorf("Failed to connect: %v", err)
```

### Structured Logging

```go
// Add context with fields
logger.Info("User authenticated", logger.Fields{
    "user_id": "12345",
    "ip": "192.168.1.1",
    "method": "oauth",
})

// Create contextual logger
requestLog := logger.WithFields(logger.Fields{
    "request_id": "abc-123",
})

requestLog.Info("Processing started")
requestLog.Info("Processing completed", logger.Fields{
    "duration_ms": 45,
    "status": "success",
})
```

## Configuration

### Via YAML Config

```yaml
logging:
  level: "info"        # debug, info, warn, error, fatal
  format: "text"       # text or json
  colorize: true       # colored output (development)
  output_file: ""      # optional file output
```

### Programmatic Configuration

```go
import "eko/internal/helpers/logger"

logger.Initialize(logger.Config{
    Level:      logger.InfoLevel,
    Output:     os.Stdout,
    JSONFormat: false,
    Colorize:   true,
})
```

## Log Levels

| Level | Use Case | Example |
|-------|----------|---------|
| **Debug** | Detailed diagnostic info | Variable values, function entry/exit |
| **Info** | General information | Server started, request completed |
| **Warn** | Warning conditions | High memory usage, deprecated API |
| **Error** | Error conditions | Database error, API call failed |
| **Fatal** | Critical errors | Config load failed, exits program |

### Setting Log Level

```go
// From string
level, _ := logger.ParseLevel("DEBUG")
logger.SetLevel(level)

// Directly
logger.SetLevel(logger.WarnLevel)
```

## Output Formats

### Text Format (Development)

Readable format with optional colors:

```
[2025-11-22 12:30:45] INFO  | Server started | port=8080 env=development
[2025-11-22 12:30:46] WARN  | High memory usage | usage=85% threshold=80%
[2025-11-22 12:30:47] ERROR | Database error | error=connection timeout
```

### JSON Format (Production)

Machine-readable format for log aggregation:

```json
{"timestamp":"2025-11-22T12:30:45Z","level":"INFO","message":"Server started","fields":{"port":8080}}
{"timestamp":"2025-11-22T12:30:46Z","level":"WARN","message":"High memory usage","fields":{"usage":"85%"}}
{"timestamp":"2025-11-22T12:30:47Z","level":"ERROR","message":"Database error","fields":{"error":"timeout"}}
```

## Integration Examples

### HTTP Request Logging

```go
func handleRequest(c *gin.Context) {
    // Create request-scoped logger
    log := logger.WithFields(logger.Fields{
        "request_id": c.GetHeader("X-Request-ID"),
        "path":       c.Request.URL.Path,
        "method":     c.Request.Method,
    })

    log.Info("Request received")

    // Process request...

    log.Info("Request completed", logger.Fields{
        "status":      c.Writer.Status(),
        "duration_ms": 123,
    })
}
```

### Error Handling

```go
func processData(data string) error {
    logger.Debug("Processing data", logger.Fields{
        "data_length": len(data),
    })

    result, err := sanitize(data)
    if err != nil {
        logger.Error("Sanitization failed", logger.Fields{
            "error":       err.Error(),
            "data_length": len(data),
        })
        return err
    }

    logger.Info("Sanitization successful", logger.Fields{
        "violations": len(result.Violations),
        "safe":       result.Safe,
    })
    return nil
}
```

### Application Startup

```go
func main() {
    // Initialize logger
    logger.Initialize(logger.Config{
        Level:      logger.InfoLevel,
        Output:     os.Stdout,
        JSONFormat: os.Getenv("ENV") == "production",
        Colorize:   os.Getenv("ENV") != "production",
    })

    logger.Info("Starting Ekō", logger.Fields{
        "version": "1.0.0",
        "env":     os.Getenv("ENV"),
    })

    // Application code...
}
```

## Best Practices

### 1. Use Appropriate Log Levels

```go
// Good
logger.Debug("Variable value", logger.Fields{"value": x})
logger.Info("User logged in", logger.Fields{"user_id": uid})
logger.Warn("Rate limit approaching", logger.Fields{"current": 95, "limit": 100})
logger.Error("Database query failed", logger.Fields{"error": err})
logger.Fatal("Config file not found", logger.Fields{"path": configPath})

// Avoid
logger.Info("x = 42") // Use Debug for variable dumps
logger.Error("User logged in") // Not an error
```

### 2. Add Context with Fields

```go
// Good - structured and searchable
logger.Error("Payment processing failed", logger.Fields{
    "transaction_id": txID,
    "amount":        100.50,
    "currency":      "USD",
    "error":         err.Error(),
})

// Avoid - unstructured
logger.Errorf("Payment failed for transaction %s: %v", txID, err)
```

### 3. Use Request-Scoped Loggers

```go
// Create logger with request context
requestLog := logger.WithFields(logger.Fields{
    "request_id": requestID,
    "user_id":    userID,
})

// Use throughout request lifecycle
requestLog.Info("Validating input")
requestLog.Debug("Calling external API")
requestLog.Info("Request completed")
```

### 4. Don't Log Sensitive Data

```go
// Bad - logs password
logger.Info("Login attempt", logger.Fields{
    "username": username,
    "password": password, // NEVER log passwords
})

// Good - logs only necessary info
logger.Info("Login attempt", logger.Fields{
    "username": username,
})
```

### 5. Use JSON Format in Production

```yaml
# Development
logging:
  format: "text"
  colorize: true

# Production
logging:
  format: "json"
  colorize: false
```

## Advanced Usage

### Custom Logger Instances

```go
// Create separate logger for specific component
auditLogger := logger.New(logger.Config{
    Level:      logger.InfoLevel,
    Output:     auditLogFile,
    JSONFormat: true,
    Colorize:   false,
})

auditLogger.Info("User action", logger.Fields{
    "action":  "delete_resource",
    "user_id": userID,
})
```

### Log to File

```go
logFile, err := os.OpenFile("app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
if err != nil {
    panic(err)
}
defer logFile.Close()

logger.Initialize(logger.Config{
    Level:      logger.InfoLevel,
    Output:     logFile,
    JSONFormat: true,
    Colorize:   false,
})
```

### Dynamic Log Level

```go
// Change log level at runtime
if os.Getenv("DEBUG") == "true" {
    logger.SetLevel(logger.DebugLevel)
}

// Check current level
if logger.Default().GetLevel() <= logger.DebugLevel {
    // Expensive debug operation only if debug is enabled
    logger.Debug("Detailed state", logger.Fields{
        "state": expensiveStateSnapshot(),
    })
}
```

## Testing

The logger is fully tested with comprehensive test coverage:

```bash
# Run logger tests
go test ./internal/helpers/logger/...

# Run with coverage
go test -cover ./internal/helpers/logger/...

# Run with verbose output
go test -v ./internal/helpers/logger/...
```

## Performance Considerations

1. **Level Filtering**: Logs below the configured level are not processed
2. **Lazy Evaluation**: Use level checks for expensive operations
3. **Thread-Safe**: Safe for concurrent use, internally synchronized
4. **Minimal Allocations**: Efficient field handling

```go
// Good - only evaluate if debug is enabled
if logger.Default().GetLevel() <= logger.DebugLevel {
    logger.Debug("State dump", logger.Fields{
        "state": expensiveOperation(),
    })
}

// Avoid - always evaluates expensive operation
logger.Debug("State dump", logger.Fields{
    "state": expensiveOperation(), // Called even if Debug is disabled
})
```

## Troubleshooting

### Logs Not Appearing

Check log level configuration:
```go
logger.SetLevel(logger.DebugLevel) // Enable all logs
```

### JSON Format Not Working

Ensure JSONFormat is enabled:
```yaml
logging:
  format: "json"  # Must be "json" not "JSON"
```

### Colors Not Showing

Check colorize setting and ensure not using JSON:
```yaml
logging:
  format: "text"
  colorize: true
```

## Migration from Standard Log

```go
// Before
import "log"
log.Println("Server started")
log.Printf("Port: %d", port)

// After
import "eko/internal/helpers/logger"
logger.Info("Server started")
logger.Infof("Port: %d", port)

// Or with fields
logger.Info("Server started", logger.Fields{"port": port})
```

## Summary

The Ekō logger provides:
- ✅ Production-ready structured logging
- ✅ Multiple output formats (text/JSON)
- ✅ Flexible configuration
- ✅ Thread-safe operation
- ✅ Zero external dependencies
- ✅ Comprehensive test coverage
- ✅ Easy migration path

For more details, see:
- Package documentation: `internal/helpers/logger/README.md`
- Examples: `internal/helpers/logger/example_test.go`
- Tests: `internal/helpers/logger/logger_test.go`
