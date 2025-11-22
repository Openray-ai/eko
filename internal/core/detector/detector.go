package detector

import (
	"regexp"
	"sync"
)

// Detector handles pattern matching and violation detection
type Detector struct {
	patterns map[string]*CompiledPattern
	mu       sync.RWMutex
}

// CompiledPattern represents a compiled regex pattern with metadata
type CompiledPattern struct {
	Name        string
	Regex       *regexp.Regexp
	Type        string
	Severity    string
	Description string
}

// Violation represents a detected sensitive data match
type Violation struct {
	Type     string `json:"type"`
	Severity string `json:"severity"`
	Pattern  string `json:"pattern"`
	Matched  string `json:"matched"`
	Position int    `json:"position"`
}

// New creates a new Detector instance
func New() *Detector {
	return &Detector{
		patterns: make(map[string]*CompiledPattern),
	}
}

// Detect scans input text for sensitive patterns
func (d *Detector) Detect(input string) ([]Violation, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var violations []Violation

	// TODO: Implement concurrent pattern matching
	// TODO: Add context-aware detection

	return violations, nil
}

// LoadPattern adds a compiled pattern to the detector
func (d *Detector) LoadPattern(pattern *CompiledPattern) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.patterns[pattern.Name] = pattern
}
