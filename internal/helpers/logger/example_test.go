package logger_test

import (
	"bytes"
	"eko/internal/helpers/logger"
	"fmt"
	"os"
)

// ExampleLogger_basic demonstrates basic logging
func ExampleLogger_basic() {
	// Initialize logger with text format
	logger.Initialize(logger.Config{
		Level:      logger.InfoLevel,
		Output:     os.Stdout,
		JSONFormat: false,
		Colorize:   false,
	})

	// Simple logging
	logger.Info("Application started")
	logger.Warn("This is a warning")
	logger.Error("An error occurred")
}

// ExampleLogger_formatted demonstrates formatted logging
func ExampleLogger_formatted() {
	logger.Initialize(logger.Config{
		Level:      logger.DebugLevel,
		Output:     os.Stdout,
		JSONFormat: false,
		Colorize:   false,
	})

	port := 8080
	logger.Infof("Server listening on port %d", port)
	logger.Debugf("Debug mode enabled: %v", true)
}

// ExampleLogger_withFields demonstrates structured logging with fields
func ExampleLogger_withFields() {
	var buf bytes.Buffer
	logger.Initialize(logger.Config{
		Level:      logger.InfoLevel,
		Output:     &buf,
		JSONFormat: false,
		Colorize:   false,
	})

	// Log with fields
	logger.Info("User logged in", logger.Fields{
		"user_id": "12345",
		"ip":      "192.168.1.1",
		"method":  "oauth",
	})

	fmt.Println("Fields are included in the log output")
	// Output: Fields are included in the log output
}

// ExampleLogger_contextual demonstrates contextual logging
func ExampleLogger_contextual() {
	var buf bytes.Buffer
	logger.Initialize(logger.Config{
		Level:      logger.InfoLevel,
		Output:     &buf,
		JSONFormat: false,
		Colorize:   false,
	})

	// Create a logger with context
	requestLogger := logger.WithFields(logger.Fields{
		"request_id": "abc-123",
		"endpoint":   "/v1/sanitize",
	})

	// All logs from this logger will include the context
	requestLogger.Info("Request received")
	requestLogger.Info("Processing request")
	requestLogger.Info("Request completed", logger.Fields{
		"duration_ms": 45,
	})

	fmt.Println("Context is preserved across multiple log calls")
	// Output: Context is preserved across multiple log calls
}

// ExampleLogger_levels demonstrates different log levels
func ExampleLogger_levels() {
	var buf bytes.Buffer
	logger.Initialize(logger.Config{
		Level:      logger.DebugLevel, // Log everything
		Output:     &buf,
		JSONFormat: false,
		Colorize:   false,
	})

	logger.Debug("Detailed debugging information")
	logger.Info("General informational message")
	logger.Warn("Warning message")
	logger.Error("Error message")

	fmt.Println("All log levels logged")
	// Output: All log levels logged
}

// ExampleLogger_json demonstrates JSON format logging
func ExampleLogger_json() {
	var buf bytes.Buffer
	logger.Initialize(logger.Config{
		Level:      logger.InfoLevel,
		Output:     &buf,
		JSONFormat: true, // Enable JSON format
		Colorize:   false,
	})

	logger.Info("Server started", logger.Fields{
		"port": 8080,
		"env":  "production",
	})

	fmt.Println("JSON formatted log entry created")
	// Output: JSON formatted log entry created
}

// ExampleLogger_levelFiltering demonstrates log level filtering
func ExampleLogger_levelFiltering() {
	var buf bytes.Buffer
	logger.Initialize(logger.Config{
		Level:      logger.WarnLevel, // Only warn and above
		Output:     &buf,
		JSONFormat: false,
		Colorize:   false,
	})

	logger.Debug("This won't be logged")
	logger.Info("This won't be logged either")
	logger.Warn("This will be logged")
	logger.Error("This will also be logged")

	fmt.Println("Only warnings and errors are logged")
	// Output: Only warnings and errors are logged
}

// ExampleParseLevel demonstrates parsing log level from string
func ExampleParseLevel() {
	level, err := logger.ParseLevel("INFO")
	if err != nil {
		fmt.Println("Error parsing level")
		return
	}

	fmt.Printf("Parsed level: %s\n", level.String())
	// Output: Parsed level: INFO
}
