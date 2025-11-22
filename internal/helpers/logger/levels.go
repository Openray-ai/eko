package logger

import (
	"fmt"
	"strings"
)

// ParseLevel converts a string to a LogLevel
func ParseLevel(level string) (LogLevel, error) {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "DEBUG":
		return DebugLevel, nil
	case "INFO":
		return InfoLevel, nil
	case "WARN", "WARNING":
		return WarnLevel, nil
	case "ERROR":
		return ErrorLevel, nil
	case "FATAL":
		return FatalLevel, nil
	default:
		return InfoLevel, fmt.Errorf("invalid log level: %s", level)
	}
}

// MustParseLevel converts a string to LogLevel or panics
func MustParseLevel(level string) LogLevel {
	l, err := ParseLevel(level)
	if err != nil {
		panic(err)
	}
	return l
}

// LevelFromEnv gets log level from environment variable or returns default
func LevelFromEnv(envVar string, defaultLevel LogLevel) LogLevel {
	// This will be implemented when we add environment variable support
	// For now, return the default
	return defaultLevel
}
