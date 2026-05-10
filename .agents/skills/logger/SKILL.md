---
name: logger
description: Add structured logging to Ekō with proper log levels, fields, and formatting. Use this skill when implementing logging in handlers, middleware, core services, or any component that needs observability.
---

# Ekō Structured Logger Skill

This skill helps you implement production-ready structured logging throughout the Ekō application using the built-in logger package at `internal/helpers/logger`.

## When to Use This Skill

Use this skill when you need to:
- Add logging to HTTP handlers or middleware
- Log errors, warnings, or debug information
- Add structured fields for better observability
- Create request-scoped loggers with context
- Implement logging in core services (detector, sanitizer, proxy)
- Debug issues with detailed contextual information

## Quick Reference

### Import Statement
```go
import "eko/internal/helpers/logger"
```

### Log Levels (in order of severity)
- `logger.Debug()` - Detailed diagnostic information
- `logger.Info()` - General informational messages
- `logger.Warn()` - Warning messages
- `logger.Error()` - Error messages
- `logger.Fatal()` - Critical errors (exits program)

## Instructions

### 1. Basic Logging

For simple log messages without structured data:

```go
// Simple messages
logger.Info("Server started")
logger.Error("Database connection failed")
logger.Debug("Processing request")

// Formatted messages
logger.Infof("Server listening on port %d", 8080)
logger.Errorf("Failed to connect: %v", err)
```

**When to use:** Quick status updates, simple error messages, startup/shutdown logs.

### 2. Structured Logging with Fields

For logs that need searchable, structured context:

```go
logger.Info("User authenticated", logger.Fields{
    "user_id": "12345",
    "ip": "192.168.1.1",
    "method": "oauth",
})

logger.Error("Database query failed", logger.Fields{
    "query": "SELECT * FROM users",
    "error": err.Error(),
    "duration_ms": 1500,
})
```

**When to use:** Production logging, error tracking, metrics collection, audit trails.

### 3. Contextual Logging (Request-Scoped)

For maintaining context across multiple log statements:

```go
// Create logger with persistent context
requestLog := logger.WithFields(logger.Fields{
    "request_id": requestID,
    "user_id": userID,
})

// All subsequent logs include the context
requestLog.Info("Validating input")
requestLog.Debug("Calling external API")
requestLog.Info("Request completed", logger.Fields{
    "duration_ms": 45,
    "status": "success",
})
```

**When to use:** HTTP handlers, long-running operations, distributed tracing.

### 4. Error Logging Best Practices

Always include relevant context when logging errors:

```go
// Good - provides context
result, err := sanitizer.Sanitize(input)
if err != nil {
    logger.Error("Sanitization failed", logger.Fields{
        "error": err.Error(),
        "input_length": len(input),
        "sanitizer_version": "1.0",
    })
    return err
}

// Avoid - no context
if err != nil {
    logger.Errorf("Error: %v", err)
}
```

### 5. HTTP Handler Logging Pattern

Standard pattern for HTTP request handlers:

```go
func (h *Handler) Handle(c *gin.Context) {
    // Create request-scoped logger
    log := logger.WithFields(logger.Fields{
        "request_id": c.GetHeader("X-Request-ID"),
        "path": c.Request.URL.Path,
        "method": c.Request.Method,
    })

    log.Info("Request received")

    // Process request
    result, err := h.service.Process(data)
    if err != nil {
        log.Error("Processing failed", logger.Fields{
            "error": err.Error(),
        })
        c.JSON(500, gin.H{"error": "processing failed"})
        return
    }

    log.Info("Request completed", logger.Fields{
        "status": 200,
        "violations": len(result.Violations),
    })

    c.JSON(200, result)
}
```

## Examples

### Example 1: Adding Logging to a Core Service

**Scenario:** Add logging to the pattern detector

```go
package detector

import (
    "eko/internal/helpers/logger"
    "regexp"
)

func (d *Detector) Detect(input string) ([]Violation, error) {
    logger.Debug("Starting detection", logger.Fields{
        "input_length": len(input),
        "patterns_loaded": len(d.patterns),
    })

    d.mu.RLock()
    defer d.mu.RUnlock()

    var violations []Violation

    for name, pattern := range d.patterns {
        matches := pattern.Regex.FindAllStringIndex(input, -1)
        if len(matches) > 0 {
            logger.Warn("Pattern matched", logger.Fields{
                "pattern": name,
                "matches": len(matches),
                "severity": pattern.Severity,
            })

            for _, match := range matches {
                violations = append(violations, Violation{
                    Type: pattern.Type,
                    Severity: pattern.Severity,
                    Pattern: name,
                    Position: match[0],
                })
            }
        }
    }

    logger.Info("Detection completed", logger.Fields{
        "violations_found": len(violations),
    })

    return violations, nil
}
```

### Example 2: Logging in Proxy Handlers

**Scenario:** Add logging to OpenAI proxy endpoint

```go
func (p *OpenAIProxy) HandleChatCompletion(c *gin.Context) {
    log := logger.WithFields(logger.Fields{
        "provider": "openai",
        "endpoint": "chat/completions",
        "request_id": c.GetHeader("X-Request-ID"),
    })

    log.Info("Proxy request received")

    var req ChatCompletionRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        log.Error("Invalid request format", logger.Fields{
            "error": err.Error(),
        })
        c.JSON(400, gin.H{"error": "invalid request"})
        return
    }

    log.Debug("Sanitizing messages", logger.Fields{
        "message_count": len(req.Messages),
    })

    // Sanitize messages
    violations, err := p.sanitizeMessages(req.Messages)
    if err != nil {
        log.Error("Sanitization failed", logger.Fields{
            "error": err.Error(),
        })
        c.JSON(500, gin.H{"error": "sanitization failed"})
        return
    }

    if len(violations) > 0 {
        log.Warn("Violations detected", logger.Fields{
            "violation_count": len(violations),
            "action": "sanitized",
        })
    }

    log.Info("Forwarding to OpenAI", logger.Fields{
        "model": req.Model,
        "violations_sanitized": len(violations),
    })

    // Forward to OpenAI...
}
```

### Example 3: Error Recovery with Stack Traces

**Scenario:** Log panics with full context in middleware

```go
func Recovery() gin.HandlerFunc {
    return func(c *gin.Context) {
        defer func() {
            if err := recover(); err != nil {
                logger.Error("Panic recovered", logger.Fields{
                    "error": fmt.Sprintf("%v", err),
                    "path": c.Request.URL.Path,
                    "method": c.Request.Method,
                    "client_ip": c.ClientIP(),
                    "stack_trace": string(debug.Stack()),
                })

                c.JSON(500, gin.H{"error": "internal server error"})
                c.Abort()
            }
        }()
        c.Next()
    }
}
```

### Example 4: Performance Logging

**Scenario:** Log operation timing and performance metrics

```go
func (s *Sanitizer) Sanitize(input string) (*Result, error) {
    start := time.Now()

    log := logger.WithFields(logger.Fields{
        "operation": "sanitize",
        "input_length": len(input),
    })

    log.Debug("Sanitization started")

    // Detect violations
    violations, err := s.detector.Detect(input)
    if err != nil {
        log.Error("Detection failed", logger.Fields{
            "error": err.Error(),
            "elapsed_ms": time.Since(start).Milliseconds(),
        })
        return nil, err
    }

    // Apply sanitization...

    elapsed := time.Since(start)

    log.Info("Sanitization completed", logger.Fields{
        "violations": len(violations),
        "elapsed_ms": elapsed.Milliseconds(),
        "safe": len(violations) == 0,
    })

    return result, nil
}
```

## Best Practices

### ✅ DO:

1. **Use appropriate log levels**
   - Debug: Variable values, detailed flow
   - Info: Normal operations, success cases
   - Warn: Degraded performance, deprecations
   - Error: Failures that need attention
   - Fatal: Cannot continue

2. **Add structured fields for important data**
   ```go
   logger.Error("API call failed", logger.Fields{
       "endpoint": endpoint,
       "status_code": resp.StatusCode,
       "error": err.Error(),
   })
   ```

3. **Use request-scoped loggers in handlers**
   ```go
   log := logger.WithFields(logger.Fields{"request_id": requestID})
   ```

4. **Log timing for operations**
   ```go
   logger.Info("Operation completed", logger.Fields{
       "duration_ms": time.Since(start).Milliseconds(),
   })
   ```

### ❌ DON'T:

1. **Don't log sensitive data**
   ```go
   // BAD
   logger.Info("User login", logger.Fields{
       "password": password, // Never log passwords, API keys, tokens
   })
   ```

2. **Don't use formatted logs when fields are better**
   ```go
   // BAD
   logger.Infof("User %s logged in from %s", userID, ip)

   // GOOD
   logger.Info("User logged in", logger.Fields{
       "user_id": userID,
       "ip": ip,
   })
   ```

3. **Don't log in tight loops without guards**
   ```go
   // BAD
   for _, item := range largeList {
       logger.Debug("Processing item", logger.Fields{"item": item})
   }

   // GOOD
   logger.Debug("Processing items", logger.Fields{"count": len(largeList)})
   ```

## Configuration

The logger is configured in `configs/config.example.yaml`:

```yaml
logging:
  level: "info"        # debug, info, warn, error, fatal
  format: "text"       # text (development) or json (production)
  colorize: true       # colored output
  output_file: ""      # optional file path
```

## Testing Logging

When writing tests, capture log output:

```go
func TestServiceWithLogging(t *testing.T) {
    var buf bytes.Buffer
    logger.Initialize(logger.Config{
        Level: logger.DebugLevel,
        Output: &buf,
        JSONFormat: false,
        Colorize: false,
    })

    // Run test...

    output := buf.String()
    if !strings.Contains(output, "expected log message") {
        t.Error("Expected log not found")
    }
}
```

## Common Patterns

### Pattern 1: Service Initialization
```go
func NewService() *Service {
    logger.Info("Initializing service", logger.Fields{
        "component": "pattern_loader",
        "version": "1.0.0",
    })
    return &Service{}
}
```

### Pattern 2: Background Jobs
```go
func (w *Worker) Run() {
    log := logger.WithFields(logger.Fields{
        "worker_id": w.ID,
        "component": "background_worker",
    })

    log.Info("Worker started")
    defer log.Info("Worker stopped")

    for job := range w.jobs {
        log.Debug("Processing job", logger.Fields{"job_id": job.ID})
        // Process...
    }
}
```

### Pattern 3: External API Calls
```go
func (c *Client) CallExternalAPI(endpoint string) error {
    log := logger.WithFields(logger.Fields{
        "component": "external_api",
        "endpoint": endpoint,
    })

    log.Debug("Calling external API")

    resp, err := http.Get(endpoint)
    if err != nil {
        log.Error("API call failed", logger.Fields{
            "error": err.Error(),
        })
        return err
    }

    log.Info("API call successful", logger.Fields{
        "status_code": resp.StatusCode,
    })
    return nil
}
```

## Reference Documentation

- Full guide: `docs/LOGGER.md`
- Package docs: `internal/helpers/logger/README.md`
- Examples: `internal/helpers/logger/example_test.go`
- Implementation: `internal/helpers/logger/logger.go`

## Summary

The Ekō logger provides production-ready structured logging with:
- Multiple log levels (Debug, Info, Warn, Error, Fatal)
- Structured fields for searchability
- Contextual logging with WithFields
- Text format for development, JSON for production
- Thread-safe operation
- Zero external dependencies

Always prefer structured logging with fields over formatted strings for better observability and debugging.
