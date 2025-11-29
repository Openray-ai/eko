package test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

const (
	// Test server configuration
	testServerURL = "http://localhost:8080/v1"
	testAPIKey    = "sk-test-fake-key-for-integration-testing"
	testTimeout   = 30 * time.Second
)

func TestChatCompletionsProxy(t *testing.T) {
	if os.Getenv("EKO_INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set EKO_INTEGRATION_TEST=true to run")
	}

	tests := []struct {
		name               string
		message            string
		expectViolations   bool
		minViolationCount  int
		expectedViolations []string
	}{
		{
			name:               "Sanitize email address",
			message:            "My email is john.doe@example.com. Can you help me?",
			expectViolations:   true,
			minViolationCount:  1,
			expectedViolations: []string{"pii"},
		},
		{
			name:               "Sanitize MongoDB connection string",
			message:            "Connect to mongodb://admin:password123@localhost:27017/mydb",
			expectViolations:   true,
			minViolationCount:  1,
			expectedViolations: []string{"credential"},
		},
		{
			name:               "Sanitize multiple violations",
			message:            "My email is test@example.com and database is postgres://user:pass@localhost:5432/db",
			expectViolations:   true,
			minViolationCount:  2,
			expectedViolations: []string{"pii", "credential"},
		},
		{
			name:              "Clean message with no sensitive data",
			message:           "What is the weather like today?",
			expectViolations:  false,
			minViolationCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedHeaders http.Header
			httpClient := &http.Client{
				Transport: &headerCaptureTransport{
					underlying: http.DefaultTransport,
					onResponse: func(resp *http.Response) {
						capturedHeaders = resp.Header.Clone()
					},
				},
			}

			client := openai.NewClient(
				option.WithAPIKey(testAPIKey),
				option.WithBaseURL(testServerURL),
				option.WithHTTPClient(httpClient),
			)

			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()

			_, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
				Messages: []openai.ChatCompletionMessageParamUnion{
					openai.UserMessage(tt.message),
				},
				Model: openai.ChatModelGPT4o,
			})

			if err == nil {
				t.Fatal("Expected error from OpenAI (401), but got success")
			}

			if !isAuthError(err) {
				t.Fatalf("Expected 401 auth error, got: %v", err)
			}

			if tt.expectViolations {
				violationsFound := capturedHeaders.Get("X-Eko-Violations-Found")
				if violationsFound == "" {
					t.Error("Expected X-Eko-Violations-Found header, but not found")
				}

				redactedCount := capturedHeaders.Get("X-Eko-Redacted-Count")
				if redactedCount == "" {
					t.Error("Expected X-Eko-Redacted-Count header, but not found")
				}

				violationSummary := capturedHeaders.Get("X-Eko-Violation-Summary")
				if violationSummary == "" {
					t.Error("Expected X-Eko-Violation-Summary header, but not found")
				}

				t.Logf("Violation Headers - Found: %s, Redacted: %s, Summary: %s",
					violationsFound, redactedCount, violationSummary)
			} else {
				violationsFound := capturedHeaders.Get("X-Eko-Violations-Found")
				if violationsFound != "0" && violationsFound != "" {
					t.Errorf("Expected no violations, but found: %s", violationsFound)
				}
			}
		})
	}
}

func TestResponsesProxy(t *testing.T) {
	if os.Getenv("EKO_INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set EKO_INTEGRATION_TEST=true to run")
	}

	tests := []struct {
		name              string
		input             string
		expectViolations  bool
		minViolationCount int
	}{
		{
			name:              "Sanitize sensitive data in string input",
			input:             "My email is admin@company.com and API key is sk-1234567890",
			expectViolations:  true,
			minViolationCount: 1,
		},
		{
			name:              "Sanitize PostgreSQL connection",
			input:             "Database: postgres://dbuser:secretpass@db.example.com:5432/production",
			expectViolations:  true,
			minViolationCount: 1,
		},
		{
			name:              "Clean input with no sensitive data",
			input:             "Tell me a story about a unicorn",
			expectViolations:  false,
			minViolationCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody := map[string]interface{}{
				"model": "gpt-4.1",
				"input": tt.input,
			}
			bodyBytes, err := json.Marshal(reqBody)
			if err != nil {
				t.Fatalf("Failed to marshal request: %v", err)
			}

			req, err := http.NewRequest("POST", testServerURL+"/responses", bytes.NewReader(bodyBytes))
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+testAPIKey)

			client := &http.Client{Timeout: testTimeout}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Failed to make request: %v", err)
			}
			defer resp.Body.Close()

			_, _ = io.ReadAll(resp.Body)

			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("Expected 401, got %d", resp.StatusCode)
			}

			if tt.expectViolations {
				violationsFound := resp.Header.Get("X-Eko-Violations-Found")
				if violationsFound == "" {
					t.Error("Expected X-Eko-Violations-Found header, but not found")
				}

				redactedCount := resp.Header.Get("X-Eko-Redacted-Count")
				if redactedCount == "" {
					t.Error("Expected X-Eko-Redacted-Count header, but not found")
				}

				violationSummary := resp.Header.Get("X-Eko-Violation-Summary")
				if violationSummary == "" {
					t.Error("Expected X-Eko-Violation-Summary header, but not found")
				}

				t.Logf("Violation Headers - Found: %s, Redacted: %s, Summary: %s",
					violationsFound, redactedCount, violationSummary)
			} else {
				violationsFound := resp.Header.Get("X-Eko-Violations-Found")
				if violationsFound != "0" && violationsFound != "" {
					t.Errorf("Expected no violations, but found: %s", violationsFound)
				}
			}
		})
	}
}

func TestResponsesProxyWithArrayInput(t *testing.T) {
	if os.Getenv("EKO_INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set EKO_INTEGRATION_TEST=true to run")
	}

	reqBody := map[string]interface{}{
		"model": "gpt-4.1",
		"input": []map[string]interface{}{
			{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type": "input_text",
						"text": "My email is secret@example.com and connect to mongodb://user:pass@localhost/db",
					},
					{
						"type":      "input_image",
						"image_url": "https://example.com/image.jpg",
					},
				},
			},
		},
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	req, err := http.NewRequest("POST", testServerURL+"/responses", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testAPIKey)

	client := &http.Client{Timeout: testTimeout}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	_, _ = io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Expected 401, got %d", resp.StatusCode)
	}

	violationsFound := resp.Header.Get("X-Eko-Violations-Found")
	if violationsFound == "" {
		t.Error("Expected X-Eko-Violations-Found header, but not found")
	}

	redactedCount := resp.Header.Get("X-Eko-Redacted-Count")
	violationSummary := resp.Header.Get("X-Eko-Violation-Summary")

	t.Logf("Array input test - Violations Found: %s, Redacted: %s, Summary: %s",
		violationsFound, redactedCount, violationSummary)
}

func TestChatCompletionsProxyStreaming(t *testing.T) {
	if os.Getenv("EKO_INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set EKO_INTEGRATION_TEST=true to run")
	}

	tests := []struct {
		name             string
		message          string
		expectViolations bool
	}{
		{
			name:             "Streaming with email violation",
			message:          "My email is user@example.com. What's the weather?",
			expectViolations: true,
		},
		{
			name:             "Streaming with credential violation",
			message:          "Connect to postgres://admin:secret@localhost:5432/db",
			expectViolations: true,
		},
		{
			name:             "Streaming with no violations",
			message:          "Tell me a joke about programming",
			expectViolations: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody := map[string]interface{}{
				"model": "gpt-4o",
				"messages": []map[string]interface{}{
					{
						"role":    "user",
						"content": tt.message,
					},
				},
				"stream": true,
			}
			bodyBytes, err := json.Marshal(reqBody)
			if err != nil {
				t.Fatalf("Failed to marshal request: %v", err)
			}

			req, err := http.NewRequest("POST", testServerURL+"/chat/completions", bytes.NewReader(bodyBytes))
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+testAPIKey)

			client := &http.Client{Timeout: testTimeout}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Failed to make request: %v", err)
			}
			defer resp.Body.Close()

			// Verify SSE headers
			contentType := resp.Header.Get("Content-Type")
			if !strings.Contains(contentType, "text/event-stream") {
				t.Errorf("Expected SSE content type, got: %s", contentType)
			}

			// Read SSE stream
			scanner := bufio.NewScanner(resp.Body)
			foundViolationEvent := false
			eventCount := 0

			for scanner.Scan() {
				line := scanner.Text()
				if !strings.HasPrefix(line, "data: ") {
					continue
				}

				data := strings.TrimPrefix(line, "data: ")
				if data == "[DONE]" {
					break
				}

				eventCount++
				var event map[string]interface{}
				if err := json.Unmarshal([]byte(data), &event); err != nil {
					// Log but don't fail - might be error event from OpenAI
					t.Logf("Failed to parse event: %v", err)
					continue
				}

				if eventCount == 1 {
					// Check if first event is violation event
					if eventType, ok := event["type"].(string); ok && eventType == "eko.violation" {
						foundViolationEvent = true
						t.Logf("First event is violation event: %+v", event)
					}
				}
			}

			if tt.expectViolations && !foundViolationEvent {
				t.Error("Expected violation event as first SSE event, but not found")
			} else if !tt.expectViolations && foundViolationEvent {
				t.Error("Did not expect violation event, but found one")
			}

			t.Logf("Processed %d SSE events", eventCount)
		})
	}
}

func TestResponsesProxyStreaming(t *testing.T) {
	if os.Getenv("EKO_INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set EKO_INTEGRATION_TEST=true to run")
	}

	tests := []struct {
		name             string
		input            string
		expectViolations bool
	}{
		{
			name:             "Streaming responses with PII violation",
			input:            "My contact is admin@company.com and phone 555-1234",
			expectViolations: true,
		},
		{
			name:             "Streaming responses with clean input",
			input:            "What is the capital of France?",
			expectViolations: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody := map[string]interface{}{
				"model": "gpt-4.1",
				"input": tt.input,
			}
			bodyBytes, err := json.Marshal(reqBody)
			if err != nil {
				t.Fatalf("Failed to marshal request: %v", err)
			}

			req, err := http.NewRequest("POST", testServerURL+"/responses", bytes.NewReader(bodyBytes))
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+testAPIKey)

			client := &http.Client{Timeout: testTimeout}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Failed to make request: %v", err)
			}
			defer resp.Body.Close()

			// Verify SSE headers
			contentType := resp.Header.Get("Content-Type")
			if !strings.Contains(contentType, "text/event-stream") {
				t.Errorf("Expected SSE content type, got: %s", contentType)
			}

			// Read SSE stream
			scanner := bufio.NewScanner(resp.Body)
			foundViolationEvent := false
			eventCount := 0

			for scanner.Scan() {
				line := scanner.Text()
				if !strings.HasPrefix(line, "data: ") {
					continue
				}

				data := strings.TrimPrefix(line, "data: ")
				eventCount++

				var event map[string]interface{}
				if err := json.Unmarshal([]byte(data), &event); err != nil {
					// Log but don't fail - might be different event format
					t.Logf("Failed to parse event: %v", err)
					continue
				}

				if eventCount == 1 {
					// Check if first event is violation event
					if eventType, ok := event["type"].(string); ok && eventType == "eko.violation" {
						foundViolationEvent = true
						t.Logf("First event is violation event: %+v", event)
					}
				}
			}

			if tt.expectViolations && !foundViolationEvent {
				t.Error("Expected violation event as first SSE event, but not found")
			} else if !tt.expectViolations && foundViolationEvent {
				t.Error("Did not expect violation event, but found one")
			}

			t.Logf("Processed %d SSE events", eventCount)
		})
	}
}

type headerCaptureTransport struct {
	underlying http.RoundTripper
	onResponse func(*http.Response)
}

func (t *headerCaptureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.underlying.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	if t.onResponse != nil {
		t.onResponse(resp)
	}

	return resp, nil
}

func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	return contains(errMsg, "401") ||
		contains(errMsg, "Unauthorized") ||
		contains(errMsg, "invalid_api_key") ||
		contains(errMsg, "Incorrect API key")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(indexOf(s, substr) >= 0))
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
