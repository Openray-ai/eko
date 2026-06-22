package common

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

func ForwardJSON(req RouteRequest, provider ProviderConfig, path string, body []byte) (*RouteResponse, error) {
	resp, err := DoJSON(req, provider, path, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	return &RouteResponse{
		StatusCode:  resp.StatusCode,
		ContentType: contentType,
		Body:        respBody,
	}, nil
}

func ForwardStream(req RouteRequest, provider ProviderConfig, path string, body []byte) (*RouteResponse, error) {
	resp, err := DoJSON(req, provider, path, body)
	if err != nil {
		return nil, err
	}
	return &RouteResponse{
		StatusCode:  resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Stream:      resp,
	}, nil
}

func DoJSON(req RouteRequest, provider ProviderConfig, path string, body []byte) (*http.Response, error) {
	upstreamURL := strings.TrimRight(provider.BaseURL, "/") + path
	httpReq, err := http.NewRequestWithContext(req.Context, http.MethodPost, upstreamURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	CopyProviderHeaders(req, httpReq, provider.Name)
	client := &http.Client{Timeout: time.Duration(provider.Timeout) * time.Second}
	return client.Do(httpReq)
}

func CopyProviderHeaders(req RouteRequest, upstream *http.Request, provider ProviderName) {
	upstream.Header.Set("Content-Type", "application/json")
	if auth := req.Headers.Get("Authorization"); auth != "" {
		upstream.Header.Set("Authorization", auth)
	}
	switch provider {
	case ProviderAnthropic:
		if key := req.Headers.Get("x-api-key"); key != "" {
			upstream.Header.Set("x-api-key", key)
		}
		if version := req.Headers.Get("anthropic-version"); version != "" {
			upstream.Header.Set("anthropic-version", version)
		}
		if beta := req.Headers.Get("anthropic-beta"); beta != "" {
			upstream.Header.Set("anthropic-beta", beta)
		}
	case ProviderGemini:
		if key := req.Query.Get("key"); key != "" {
			query := upstream.URL.Query()
			query.Set("key", key)
			upstream.URL.RawQuery = query.Encode()
		}
		if key := req.Headers.Get("x-goog-api-key"); key != "" {
			upstream.Header.Set("x-goog-api-key", key)
		}
	}
}

func ProxyFailure(provider ProviderName, route, model string, err error) *ProxyError {
	status := http.StatusBadGateway
	errorType := "proxy_error"
	message := fmt.Sprintf("failed to communicate with %s provider", provider)
	if isTimeoutError(err) {
		status = http.StatusGatewayTimeout
		errorType = "proxy_timeout"
		message = fmt.Sprintf("%s provider request timed out", provider)
	}
	return &ProxyError{
		Status:   status,
		Type:     errorType,
		Message:  message,
		Provider: provider,
		Model:    model,
		Route:    route,
	}
}

func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
