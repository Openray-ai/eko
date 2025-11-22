package logger

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

func TestLogLevel_String(t *testing.T) {
	tests := []struct {
		level    LogLevel
		expected string
	}{
		{DebugLevel, "DEBUG"},
		{InfoLevel, "INFO"},
		{WarnLevel, "WARN"},
		{ErrorLevel, "ERROR"},
		{FatalLevel, "FATAL"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.level.String(); got != tt.expected {
				t.Errorf("LogLevel.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected LogLevel
		wantErr  bool
	}{
		{"DEBUG", DebugLevel, false},
		{"debug", DebugLevel, false},
		{"INFO", InfoLevel, false},
		{"info", InfoLevel, false},
		{"WARN", WarnLevel, false},
		{"WARNING", WarnLevel, false},
		{"ERROR", ErrorLevel, false},
		{"FATAL", FatalLevel, false},
		{"invalid", InfoLevel, true},
		{"", InfoLevel, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseLevel(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseLevel() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.expected {
				t.Errorf("ParseLevel() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestLogger_New(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{
		Level:      InfoLevel,
		Output:     &buf,
		JSONFormat: false,
		Colorize:   false,
	})

	if logger == nil {
		t.Fatal("New() returned nil")
	}
	if logger.level != InfoLevel {
		t.Errorf("expected level InfoLevel, got %v", logger.level)
	}
}

func TestLogger_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{
		Level:      DebugLevel,
		Output:     &buf,
		JSONFormat: false,
		Colorize:   false,
	})

	logger.Info("test message")
	output := buf.String()

	if !strings.Contains(output, "INFO") {
		t.Error("output should contain INFO level")
	}
	if !strings.Contains(output, "test message") {
		t.Error("output should contain the message")
	}
}

func TestLogger_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{
		Level:      DebugLevel,
		Output:     &buf,
		JSONFormat: true,
		Colorize:   false,
	})

	logger.Info("test message")
	output := buf.String()

	var entry map[string]interface{}
	if err := json.Unmarshal([]byte(output), &entry); err != nil {
		t.Fatalf("failed to parse JSON output: %v", err)
	}

	if entry["level"] != "INFO" {
		t.Errorf("expected level INFO, got %v", entry["level"])
	}
	if entry["message"] != "test message" {
		t.Errorf("expected message 'test message', got %v", entry["message"])
	}
}

func TestLogger_WithFields(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{
		Level:      DebugLevel,
		Output:     &buf,
		JSONFormat: false,
		Colorize:   false,
	})

	logger.Info("test", Fields{"key": "value"})
	output := buf.String()

	if !strings.Contains(output, "key=value") {
		t.Error("output should contain field key=value")
	}
}

func TestLogger_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{
		Level:      WarnLevel, // Only warn and above
		Output:     &buf,
		JSONFormat: false,
		Colorize:   false,
	})

	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")

	output := buf.String()

	if strings.Contains(output, "debug message") {
		t.Error("debug message should not be logged")
	}
	if strings.Contains(output, "info message") {
		t.Error("info message should not be logged")
	}
	if !strings.Contains(output, "warn message") {
		t.Error("warn message should be logged")
	}
}

func TestLogger_FormattedMethods(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{
		Level:      DebugLevel,
		Output:     &buf,
		JSONFormat: false,
		Colorize:   false,
	})

	logger.Infof("formatted %s %d", "message", 42)
	output := buf.String()

	if !strings.Contains(output, "formatted message 42") {
		t.Error("output should contain formatted message")
	}
}

func TestLogger_WithFieldsChaining(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{
		Level:      DebugLevel,
		Output:     &buf,
		JSONFormat: false,
		Colorize:   false,
	})

	contextLogger := logger.WithFields(Fields{"request_id": "123"})
	contextLogger.Info("test", Fields{"user": "john"})

	output := buf.String()

	if !strings.Contains(output, "request_id=123") {
		t.Error("output should contain request_id field")
	}
	if !strings.Contains(output, "user=john") {
		t.Error("output should contain user field")
	}
}

func TestLogger_SetLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{
		Level:      InfoLevel,
		Output:     &buf,
		JSONFormat: false,
		Colorize:   false,
	})

	// Debug should not be logged
	logger.Debug("debug 1")
	if buf.Len() > 0 {
		t.Error("debug message should not be logged at Info level")
	}

	// Change level to Debug
	logger.SetLevel(DebugLevel)

	// Now debug should be logged
	logger.Debug("debug 2")
	if !strings.Contains(buf.String(), "debug 2") {
		t.Error("debug message should be logged after level change")
	}
}

func TestDefaultLogger(t *testing.T) {
	// Reset the default logger
	defaultLogger = nil
	once = sync.Once{}

	logger := Default()
	if logger == nil {
		t.Fatal("Default() returned nil")
	}

	// Calling again should return the same instance
	logger2 := Default()
	if logger != logger2 {
		t.Error("Default() should return the same instance")
	}
}

func TestPackageLevelFunctions(t *testing.T) {
	// Reset default logger
	defaultLogger = nil
	once = sync.Once{}

	var buf bytes.Buffer
	Initialize(Config{
		Level:      DebugLevel,
		Output:     &buf,
		JSONFormat: false,
		Colorize:   false,
	})

	Info("package level info")
	output := buf.String()

	if !strings.Contains(output, "package level info") {
		t.Error("package level Info should work")
	}
}
