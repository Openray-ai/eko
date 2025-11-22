package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

// LogLevel represents the severity of a log message
type LogLevel int

const (
	// DebugLevel for detailed debugging information
	DebugLevel LogLevel = iota
	// InfoLevel for general informational messages
	InfoLevel
	// WarnLevel for warning messages
	WarnLevel
	// ErrorLevel for error messages
	ErrorLevel
	// FatalLevel for fatal errors that require program termination
	FatalLevel
)

// String returns the string representation of the log level
func (l LogLevel) String() string {
	switch l {
	case DebugLevel:
		return "DEBUG"
	case InfoLevel:
		return "INFO"
	case WarnLevel:
		return "WARN"
	case ErrorLevel:
		return "ERROR"
	case FatalLevel:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// Color returns ANSI color code for the log level
func (l LogLevel) Color() string {
	switch l {
	case DebugLevel:
		return "\033[36m" // Cyan
	case InfoLevel:
		return "\033[32m" // Green
	case WarnLevel:
		return "\033[33m" // Yellow
	case ErrorLevel:
		return "\033[31m" // Red
	case FatalLevel:
		return "\033[35m" // Magenta
	default:
		return "\033[0m" // Reset
	}
}

// Logger represents a logger instance
type Logger struct {
	level      LogLevel
	output     io.Writer
	jsonFormat bool
	colorize   bool
	fields     map[string]interface{}
	mu         sync.Mutex
}

// Config holds logger configuration
type Config struct {
	Level      LogLevel
	Output     io.Writer
	JSONFormat bool
	Colorize   bool
}

// Fields represents structured logging fields
type Fields map[string]interface{}

// Global logger instance
var defaultLogger *Logger
var once sync.Once

// Initialize sets up the default logger
func Initialize(cfg Config) {
	once.Do(func() {
		defaultLogger = New(cfg)
	})
}

// New creates a new logger instance
func New(cfg Config) *Logger {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	return &Logger{
		level:      cfg.Level,
		output:     cfg.Output,
		jsonFormat: cfg.JSONFormat,
		colorize:   cfg.Colorize,
		fields:     make(map[string]interface{}),
	}
}

// Default returns the default logger instance
func Default() *Logger {
	if defaultLogger == nil {
		Initialize(Config{
			Level:      InfoLevel,
			Output:     os.Stdout,
			JSONFormat: false,
			Colorize:   true,
		})
	}
	return defaultLogger
}

// WithFields creates a new logger with additional fields
func (l *Logger) WithFields(fields Fields) *Logger {
	newLogger := &Logger{
		level:      l.level,
		output:     l.output,
		jsonFormat: l.jsonFormat,
		colorize:   l.colorize,
		fields:     make(map[string]interface{}),
	}

	// Copy existing fields
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}

	// Add new fields
	for k, v := range fields {
		newLogger.fields[k] = v
	}

	return newLogger
}

// SetLevel sets the minimum log level
func (l *Logger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// GetLevel returns the current log level
func (l *Logger) GetLevel() LogLevel {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.level
}

// log is the internal logging function
func (l *Logger) log(level LogLevel, msg string, fields Fields) {
	// Check if this level should be logged
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Merge logger fields with message fields
	allFields := make(map[string]interface{})
	for k, v := range l.fields {
		allFields[k] = v
	}
	for k, v := range fields {
		allFields[k] = v
	}

	if l.jsonFormat {
		l.writeJSON(level, msg, allFields)
	} else {
		l.writeText(level, msg, allFields)
	}

	// Exit on fatal
	if level == FatalLevel {
		os.Exit(1)
	}
}

// writeJSON writes log in JSON format
func (l *Logger) writeJSON(level LogLevel, msg string, fields map[string]interface{}) {
	entry := map[string]interface{}{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"level":     level.String(),
		"message":   msg,
	}

	// Add caller information
	if _, file, line, ok := runtime.Caller(3); ok {
		entry["caller"] = fmt.Sprintf("%s:%d", file, line)
	}

	// Add fields
	if len(fields) > 0 {
		entry["fields"] = fields
	}

	data, _ := json.Marshal(entry)
	fmt.Fprintln(l.output, string(data))
}

// writeText writes log in human-readable text format
func (l *Logger) writeText(level LogLevel, msg string, fields map[string]interface{}) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	levelStr := level.String()

	// Apply color if enabled
	if l.colorize {
		levelStr = fmt.Sprintf("%s%-5s\033[0m", level.Color(), levelStr)
	} else {
		levelStr = fmt.Sprintf("%-5s", levelStr)
	}

	// Build the log line
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[%s] %s | %s", timestamp, levelStr, msg))

	// Add fields if present
	if len(fields) > 0 {
		sb.WriteString(" |")
		for k, v := range fields {
			sb.WriteString(fmt.Sprintf(" %s=%v", k, v))
		}
	}

	fmt.Fprintln(l.output, sb.String())
}

// Debug logs a debug message
func (l *Logger) Debug(msg string, fields ...Fields) {
	f := mergeFields(fields...)
	l.log(DebugLevel, msg, f)
}

// Info logs an info message
func (l *Logger) Info(msg string, fields ...Fields) {
	f := mergeFields(fields...)
	l.log(InfoLevel, msg, f)
}

// Warn logs a warning message
func (l *Logger) Warn(msg string, fields ...Fields) {
	f := mergeFields(fields...)
	l.log(WarnLevel, msg, f)
}

// Error logs an error message
func (l *Logger) Error(msg string, fields ...Fields) {
	f := mergeFields(fields...)
	l.log(ErrorLevel, msg, f)
}

// Fatal logs a fatal message and exits
func (l *Logger) Fatal(msg string, fields ...Fields) {
	f := mergeFields(fields...)
	l.log(FatalLevel, msg, f)
}

// Debugf logs a formatted debug message
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.log(DebugLevel, fmt.Sprintf(format, args...), nil)
}

// Infof logs a formatted info message
func (l *Logger) Infof(format string, args ...interface{}) {
	l.log(InfoLevel, fmt.Sprintf(format, args...), nil)
}

// Warnf logs a formatted warning message
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.log(WarnLevel, fmt.Sprintf(format, args...), nil)
}

// Errorf logs a formatted error message
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.log(ErrorLevel, fmt.Sprintf(format, args...), nil)
}

// Fatalf logs a formatted fatal message and exits
func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.log(FatalLevel, fmt.Sprintf(format, args...), nil)
}

// mergeFields merges multiple Fields maps into one
func mergeFields(fields ...Fields) Fields {
	if len(fields) == 0 {
		return nil
	}

	merged := make(Fields)
	for _, f := range fields {
		for k, v := range f {
			merged[k] = v
		}
	}
	return merged
}

// Package-level convenience functions that use the default logger

// Debug logs a debug message using the default logger
func Debug(msg string, fields ...Fields) {
	Default().Debug(msg, fields...)
}

// Info logs an info message using the default logger
func Info(msg string, fields ...Fields) {
	Default().Info(msg, fields...)
}

// Warn logs a warning message using the default logger
func Warn(msg string, fields ...Fields) {
	Default().Warn(msg, fields...)
}

// Error logs an error message using the default logger
func Error(msg string, fields ...Fields) {
	Default().Error(msg, fields...)
}

// Fatal logs a fatal message using the default logger and exits
func Fatal(msg string, fields ...Fields) {
	Default().Fatal(msg, fields...)
}

// Debugf logs a formatted debug message using the default logger
func Debugf(format string, args ...interface{}) {
	Default().Debugf(format, args...)
}

// Infof logs a formatted info message using the default logger
func Infof(format string, args ...interface{}) {
	Default().Infof(format, args...)
}

// Warnf logs a formatted warning message using the default logger
func Warnf(format string, args ...interface{}) {
	Default().Warnf(format, args...)
}

// Errorf logs a formatted error message using the default logger
func Errorf(format string, args ...interface{}) {
	Default().Errorf(format, args...)
}

// Fatalf logs a formatted fatal message using the default logger and exits
func Fatalf(format string, args ...interface{}) {
	Default().Fatalf(format, args...)
}

// WithFields creates a logger with fields using the default logger
func WithFields(fields Fields) *Logger {
	return Default().WithFields(fields)
}

// SetLevel sets the log level on the default logger
func SetLevel(level LogLevel) {
	Default().SetLevel(level)
}
