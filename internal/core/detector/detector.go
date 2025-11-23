package detector

import (
	"eko/internal/core/patterns"
	"eko/internal/helpers/logger"
	"sort"
	"sync"
)

// Detector handles pattern matching and violation detection
type Detector struct {
	patterns map[string]*patterns.CompiledPattern
	mu       sync.RWMutex
}

// Violation represents a detected sensitive data match
type Violation struct {
	Type     string `json:"type"`
	Severity string `json:"severity"`
	Pattern  string `json:"pattern"`
	Matched  string `json:"matched"`
	Position int    `json:"position"`
	End      int    `json:"end"` // End position for deduplication
}

// New creates a new Detector instance
func New() *Detector {
	return &Detector{
		patterns: make(map[string]*patterns.CompiledPattern),
	}
}

// Detect scans input text for sensitive patterns using concurrent processing
func (d *Detector) Detect(input string) ([]Violation, error) {
	d.mu.RLock()
	patternList := make([]*patterns.CompiledPattern, 0, len(d.patterns))
	for _, p := range d.patterns {
		patternList = append(patternList, p)
	}
	d.mu.RUnlock()

	if len(patternList) == 0 {
		logger.Warn("No patterns loaded for detection", logger.Fields{})
		return []Violation{}, nil
	}

	// Channel to collect violations from goroutines
	violationsChan := make(chan []Violation, len(patternList))
	var wg sync.WaitGroup

	// Process each pattern concurrently
	for _, pattern := range patternList {
		wg.Add(1)
		go func(p *patterns.CompiledPattern) {
			defer wg.Done()
			violations := d.detectPattern(input, p)
			violationsChan <- violations
		}(pattern)
	}

	// Wait for all goroutines to complete and close channel
	go func() {
		wg.Wait()
		close(violationsChan)
	}()

	// Collect all violations
	var allViolations []Violation
	for violations := range violationsChan {
		allViolations = append(allViolations, violations...)
	}

	// Deduplicate overlapping violations
	deduped := d.deduplicateViolations(allViolations)

	// Sort by position for consistent output
	sort.Slice(deduped, func(i, j int) bool {
		return deduped[i].Position < deduped[j].Position
	})

	logger.Debug("Detection completed", logger.Fields{
		"input_length":     len(input),
		"patterns_checked": len(patternList),
		"violations_found": len(deduped),
	})

	return deduped, nil
}

// detectPattern finds all matches for a single pattern
func (d *Detector) detectPattern(input string, pattern *patterns.CompiledPattern) []Violation {
	matches := pattern.Regex.FindAllStringIndex(input, -1)
	if len(matches) == 0 {
		return nil
	}

	violations := make([]Violation, 0, len(matches))
	for _, match := range matches {
		start, end := match[0], match[1]
		matched := input[start:end]

		// Skip empty matches
		if matched == "" {
			continue
		}

		violations = append(violations, Violation{
			Type:     pattern.Type,
			Severity: pattern.Severity,
			Pattern:  pattern.Name,
			Matched:  matched,
			Position: start,
			End:      end,
		})
	}

	return violations
}

// deduplicateViolations removes overlapping violations, keeping higher severity ones
func (d *Detector) deduplicateViolations(violations []Violation) []Violation {
	if len(violations) <= 1 {
		return violations
	}

	// Sort by position, then by severity priority
	sort.Slice(violations, func(i, j int) bool {
		if violations[i].Position != violations[j].Position {
			return violations[i].Position < violations[j].Position
		}
		return d.getSeverityPriority(violations[i].Severity) > d.getSeverityPriority(violations[j].Severity)
	})

	// Remove overlapping violations
	deduped := make([]Violation, 0, len(violations))
	for _, v := range violations {
		// Check if this violation overlaps with any already kept
		overlaps := false
		for _, kept := range deduped {
			if d.overlaps(v, kept) {
				overlaps = true
				break
			}
		}
		if !overlaps {
			deduped = append(deduped, v)
		}
	}

	return deduped
}

// overlaps checks if two violations overlap in position
func (d *Detector) overlaps(v1, v2 Violation) bool {
	return (v1.Position >= v2.Position && v1.Position < v2.End) ||
		(v2.Position >= v1.Position && v2.Position < v1.End)
}

// getSeverityPriority returns priority value for severity (higher is more important)
func (d *Detector) getSeverityPriority(severity string) int {
	switch severity {
	case "BLOCK":
		return 3
	case "WARN":
		return 2
	case "LOG":
		return 1
	default:
		return 0
	}
}

// LoadPattern adds a compiled pattern to the detector
func (d *Detector) LoadPattern(pattern *patterns.CompiledPattern) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.patterns[pattern.Name] = pattern
}

// LoadPatterns adds multiple compiled patterns to the detector
func (d *Detector) LoadPatterns(patternList []*patterns.CompiledPattern) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, pattern := range patternList {
		d.patterns[pattern.Name] = pattern
	}

	logger.Info("Loaded patterns into detector", logger.Fields{
		"count": len(patternList),
		"total": len(d.patterns),
	})
}

// GetPatternCount returns the number of loaded patterns
func (d *Detector) GetPatternCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.patterns)
}

// GetPatternNames returns all loaded pattern names
func (d *Detector) GetPatternNames() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	names := make([]string, 0, len(d.patterns))
	for name := range d.patterns {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (d *Detector) LoadDefaultPatterns() {
	loader := patterns.NewLoader("patterns/default", "patterns/custom")
	compiledPatterns, err := loader.LoadAll()
	if err != nil {
		logger.Error("Failed to load patterns", logger.Fields{
			"error": err.Error(),
		})
		logger.Fatal("Cannot start without patterns", logger.Fields{})
	}

	d.LoadPatterns(compiledPatterns)
	logger.Info("Patterns loaded successfully", logger.Fields{
		"count": len(compiledPatterns),
	})
}
