# Test Suite

This directory contains integration tests and test utilities for Ekō.

## Structure

- `integration/` - End-to-end integration tests
- `fixtures/` - Test data and fixtures
- `helpers/` - Test helper functions

## Running Tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run specific package tests
go test ./internal/core/detector/...

# Run with verbose output
go test -v ./...
```

## Writing Tests

Follow Go testing conventions:
- Test files end with `_test.go`
- Test functions start with `Test`
- Use table-driven tests for multiple cases
- Use `t.Helper()` for test helper functions
