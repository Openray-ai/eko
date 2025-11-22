package sanitizer

import "eko/internal/core/detector"

// Sanitizer handles redaction and replacement of sensitive data
type Sanitizer struct {
	detector *detector.Detector
}

// Result contains the sanitized output and violations found
type Result struct {
	SanitizedPrompt  string               `json:"sanitized_prompt"`
	Violations       []detector.Violation `json:"violations"`
	Safe             bool                 `json:"safe"`
	ProcessingTimeMs float64              `json:"processing_time_ms"`
}

// New creates a new Sanitizer instance
func New(det *detector.Detector) *Sanitizer {
	return &Sanitizer{
		detector: det,
	}
}

// Sanitize detects and redacts sensitive data from input
func (s *Sanitizer) Sanitize(input string) (*Result, error) {
	// TODO: Implement sanitization logic
	// TODO: Add configurable redaction strategies

	violations, err := s.detector.Detect(input)
	if err != nil {
		return nil, err
	}

	return &Result{
		SanitizedPrompt:  input,
		Violations:       violations,
		Safe:             len(violations) == 0,
		ProcessingTimeMs: 0,
	}, nil
}
