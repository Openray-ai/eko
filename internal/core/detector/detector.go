package detector

import (
	"eko/internal/core/patterns"
	"eko/internal/helpers/logger"
	"runtime"
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
	patternMap := make(map[string]*patterns.CompiledPattern, len(d.patterns))
	for _, p := range d.patterns {
		patternList = append(patternList, p)
		patternMap[p.Name] = p
	}
	d.mu.RUnlock()

	if len(patternList) == 0 {
		logger.Warn("No patterns loaded for detection", logger.Fields{})
		return []Violation{}, nil
	}

	// Build a request-local semaphore sized to available parallelism, capped at
	// the number of patterns. Using a local channel prevents cross-request
	// head-of-line blocking: each Detect call competes only against its own
	// goroutines, not those of concurrent callers.
	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}
	if workers > len(patternList) {
		workers = len(patternList)
	}
	sem := make(chan struct{}, workers)

	// Channel to collect violations from goroutines
	violationsChan := make(chan []Violation, len(patternList))
	var wg sync.WaitGroup

	// Process each pattern concurrently, bounded by the request-local semaphore.
	for _, pattern := range patternList {
		wg.Add(1)
		sem <- struct{}{}
		go func(p *patterns.CompiledPattern) {
			defer wg.Done()
			defer func() { <-sem }()
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
	allViolations = append(allViolations, d.detectAdvanced(input, patternMap, sem)...)

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
		priorityI := d.getSeverityPriority(violations[i].Severity)
		priorityJ := d.getSeverityPriority(violations[j].Severity)
		if priorityI != priorityJ {
			return priorityI > priorityJ
		}
		typePriorityI := d.getTypePriority(violations[i].Type)
		typePriorityJ := d.getTypePriority(violations[j].Type)
		if typePriorityI != typePriorityJ {
			return typePriorityI > typePriorityJ
		}
		lenI := violations[i].End - violations[i].Position
		lenJ := violations[j].End - violations[j].Position
		return lenI > lenJ
	})

	// Remove overlapping violations using a sweep-line O(n) pass.
	// Because violations are sorted left-to-right by Position, any new candidate
	// whose Position >= maxEnd cannot overlap any already-kept violation.
	deduped := make([]Violation, 0, len(violations))
	maxEnd := 0
	for _, v := range violations {
		if v.Position >= maxEnd {
			deduped = append(deduped, v)
			if v.End > maxEnd {
				maxEnd = v.End
			}
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

func (d *Detector) getTypePriority(patternType string) int {
	switch patternType {
	case patterns.TypeCredential:
		return 3
	case patterns.TypeFinancial:
		return 2
	case patterns.TypePII:
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
