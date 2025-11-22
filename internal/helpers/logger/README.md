# Logger Package

A production-ready structured logging package for Ekō with support for multiple log levels, JSON/text formats, and contextual fields.

## Features

- **Multiple Log Levels**: Debug, Info, Warn, Error, Fatal
- **Structured Logging**: Add contextual fields to log messages
- **Flexible Output**: JSON format for production, colorized text for development
- **Thread-Safe**: Safe for concurrent use
- **Zero Dependencies**: Uses only Go standard library
- **Context Preservation**: Chain loggers with fields

## Usage

### Basic Logging

```go
import "eko/internal/helpers/logger"

// Simple logging
logger.Info("Server started")
logger.Debug("Debug information")
logger.Warn("This is a warning")
logger.Error("An error occurred")

// Formatted logging
logger.Infof("Server listening on port %d", 8080)
logger.Errorf("Failed to connect: %v", err)
```

### Structured Logging with Fields

```go
// Add fields to log messages
logger.Info("User logged in", logger.Fields{
    "user_id": "12345",
    "ip": "192.168.1.1",
    "method": "oauth",
})

// Create a logger with persistent fields
requestLogger := logger.WithFields(logger.Fields{
    "request_id": "abc-123",
    "endpoint": "/v1/sanitize",
})

requestLogger.Info("Processing request")
requestLogger.Info("Request completed", logger.Fields{
    "duration_ms": 45,
    "status": 200,
})
```

### Configuration

```go
import "eko/internal/helpers/logger"

// Initialize with custom configuration
logger.Initialize(logger.Config{
    Level:      logger.InfoLevel,     // Minimum level to log
    Output:     os.Stdout,            // Where to write logs
    JSONFormat: false,                // true for JSON, false for text
    Colorize:   true,                 // Colorize output (only for text format)
})

// Create a custom logger instance
customLogger := logger.New(logger.Config{
    Level:      logger.DebugLevel,
    Output:     logFile,
    JSONFormat: true,
    Colorize:   false,
})
```

### Log Levels

```go
// Set log level from string
level, err := logger.ParseLevel("INFO")
logger.SetLevel(level)

// Available levels (in order of severity)
logger.DebugLevel  // Most verbose
logger.InfoLevel   // General information
logger.WarnLevel   // Warning messages
logger.ErrorLevel  // Error messages
logger.FatalLevel  // Fatal errors (exits program)
```

### Output Formats

**Text Format (Development)**:
```
[2025-11-22 12:30:45] INFO  | Server started | port=8080 env=development
[2025-11-22 12:30:46] WARN  | High memory usage | usage=85% threshold=80%
[2025-11-22 12:30:47] ERROR | Database connection failed | error=timeout
```

**JSON Format (Production)**:
```json
{"timestamp":"2025-11-22T12:30:45Z","level":"INFO","message":"Server started","fields":{"port":8080,"env":"development"}}
{"timestamp":"2025-11-22T12:30:46Z","level":"WARN","message":"High memory usage","fields":{"usage":"85%","threshold":"80%"}}
{"timestamp":"2025-11-22T12:30:47Z","level":"ERROR","message":"Database connection failed","fields":{"error":"timeout"}}
```

## Examples

### Request Logging

```go
func handleRequest(c *gin.Context) {
    // Create request-scoped logger
    log := logger.WithFields(logger.Fields{
        "request_id": c.GetHeader("X-Request-ID"),
        "path": c.Request.URL.Path,
        "method": c.Request.Method,
    })

    log.Info("Request received")

    // Process request...

    log.Info("Request completed", logger.Fields{
        "status": c.Writer.Status(),
        "duration_ms": 123,
    })
}
```

### Error Logging

```go
func processData(data string) error {
    result, err := sanitize(data)
    if err != nil {
        logger.Error("Sanitization failed", logger.Fields{
            "error": err.Error(),
            "data_length": len(data),
        })
        return err
    }

    logger.Info("Sanitization successful", logger.Fields{
        "violations": len(result.Violations),
    })
    return nil
}
```

### Application Startup

```go
func main() {
    // Initialize logger based on environment
    logLevel := logger.InfoLevel
    if os.Getenv("DEBUG") == "true" {
        logLevel = logger.DebugLevel
    }

    logger.Initialize(logger.Config{
        Level:      logLevel,
        Output:     os.Stdout,
        JSONFormat: os.Getenv("ENV") == "production",
        Colorize:   os.Getenv("ENV") != "production",
    })

    logger.Info("Starting Ekō", logger.Fields{
        "version": "1.0.0",
        "env": os.Getenv("ENV"),
    })

    // Application code...
}
```

## Best Practices

1. **Use Appropriate Levels**:
   - `Debug`: Detailed diagnostic information
   - `Info`: General informational messages (default)
   - `Warn`: Warning messages for potentially harmful situations
   - `Error`: Error messages for serious problems
   - `Fatal`: Critical errors that require program termination

2. **Add Context with Fields**:
   ```go
   // Good
   logger.Error("Database query failed", logger.Fields{
       "query": "SELECT * FROM users",
       "error": err.Error(),
       "duration_ms": 1500,
   })

   // Avoid
   logger.Error(fmt.Sprintf("Database query failed: %v", err))
   ```

3. **Use Request-Scoped Loggers**:
   ```go
   requestLogger := logger.WithFields(logger.Fields{
       "request_id": requestID,
   })
   // Use requestLogger throughout request handling
   ```

4. **Don't Log Sensitive Data**:
   ```go
   // Bad - logs password
   logger.Info("User login", logger.Fields{"password": password})

   // Good - logs only necessary info
   logger.Info("User login", logger.Fields{"user_id": userID})
   ```

5. **Use JSON Format in Production**:
   - Makes logs machine-readable
   - Easier to parse and analyze
   - Better for log aggregation systems

## Thread Safety

The logger is safe for concurrent use. Multiple goroutines can call logger methods simultaneously.

```go
go logger.Info("Message from goroutine 1")
go logger.Info("Message from goroutine 2")
```

## Testing

```bash
# Run logger tests
go test ./internal/helpers/logger/...

# Run with coverage
go test -cover ./internal/helpers/logger/...
```
