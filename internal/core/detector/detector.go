package detector

import (
	"context"
	"runtime"
	"sort"
	"sync"

	"eko/internal/core/patterns"
	"eko/internal/core/secrets"
	"eko/internal/core/slm"
	"eko/internal/helpers/logger"
)

// SLMRunner is the contract the detector uses for an optional contextual
// detection source (e.g. the slm-sidecar). Implementations are expected to be
// safe for concurrent use and to return (nil, nil) for soft failures (breaker
// open, oversize input) so the detector can proceed with regex-only results.
type SLMRunner interface {
	Detect(ctx context.Context, input string) ([]slm.Violation, error)
}

// Detector handles pattern matching and violation detection
type Detector struct {
	patterns map[string]*patterns.CompiledPattern
	slm      SLMRunner
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

// SetSLM attaches an optional SLM runner. The detector takes a copy of the
// reference; callers may pass nil to disable. Safe to call before Detect calls
// begin; not designed to be swapped at runtime.
func (d *Detector) SetSLM(runner SLMRunner) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.slm = runner
}

// Detect scans input text for sensitive patterns. Equivalent to
// DetectWithContext(context.Background(), input); kept for callers that don't
// have a context to pass.
func (d *Detector) Detect(input string) ([]Violation, error) {
	return d.DetectWithContext(context.Background(), input)
}

// DetectWithContext runs regex pattern matching and (when configured) the SLM
// runner concurrently, then merges and deduplicates the combined violations.
func (d *Detector) DetectWithContext(ctx context.Context, input string) ([]Violation, error) {
	d.mu.RLock()
	patternList := make([]*patterns.CompiledPattern, 0, len(d.patterns))
	patternMap := make(map[string]*patterns.CompiledPattern, len(d.patterns))
	for _, p := range d.patterns {
		patternList = append(patternList, p)
		patternMap[p.Name] = p
	}
	slmRunner := d.slm
	d.mu.RUnlock()

	// Honor an explicit per-request opt-in/opt-out (POST /v1/sanitize sets
	// this from the body's `slm` field). When the caller hasn't expressed a
	// preference, the configured runner is used as-is.
	if slmRunner != nil {
		if enabled, set := slm.RequestDecision(ctx); set && !enabled {
			slmRunner = nil
		}
	}

	allViolations := convertSecretFindings(secrets.Detect(input))

	if len(patternList) == 0 {
		logger.Warn("No patterns loaded for detection", logger.Fields{})
		deduped := d.deduplicateViolations(allViolations)
		sort.Slice(deduped, func(i, j int) bool {
			return deduped[i].Position < deduped[j].Position
		})
		return deduped, nil
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

	// Channel to collect violations from goroutines (regex patterns + optional SLM)
	chanCap := len(patternList)
	if slmRunner != nil {
		chanCap++
	}
	violationsChan := make(chan []Violation, chanCap)
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

	// Run the optional SLM runner alongside the regex pool. It does not consume
	// from the regex semaphore — its bottleneck is a single network call, not
	// CPU-bound goroutines, so it doesn't compete with regex workers.
	if slmRunner != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			slmViolations, err := slmRunner.Detect(ctx, input)
			if err != nil {
				logger.Warn("SLM detection failed; using regex-only results", logger.Fields{
					"error": err.Error(),
				})
				violationsChan <- nil
				return
			}
			violationsChan <- convertSLMViolations(slmViolations)
		}()
	}

	// Wait for all goroutines to complete and close channel
	go func() {
		wg.Wait()
		close(violationsChan)
	}()

	// Collect all violations
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

// convertSLMViolations adapts the slm package's Violation type into the
// detector's. Kept inline so the slm package doesn't need to know about
// detector and we don't need a third bridge package.
func convertSLMViolations(in []slm.Violation) []Violation {
	if len(in) == 0 {
		return nil
	}
	out := make([]Violation, 0, len(in))
	for _, v := range in {
		out = append(out, Violation{
			Type:     v.Type,
			Severity: v.Severity,
			Pattern:  v.Pattern,
			Matched:  v.Matched,
			Position: v.Position,
			End:      v.End,
		})
	}
	return out
}

func convertSecretFindings(in []secrets.Finding) []Violation {
	if len(in) == 0 {
		return nil
	}
	out := make([]Violation, 0, len(in))
	for _, f := range in {
		out = append(out, Violation{
			Type:     f.Type,
			Severity: f.Severity,
			Pattern:  f.Pattern,
			Matched:  f.Matched,
			Position: f.Position,
			End:      f.End,
		})
	}
	return out
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

	// Sort by position to group overlap clusters deterministically.
	sort.Slice(violations, func(i, j int) bool {
		if violations[i].Position != violations[j].Position {
			return violations[i].Position < violations[j].Position
		}
		return d.betterViolation(violations[i], violations[j])
	})

	// Remove overlapping violations by clustering intersecting spans and keeping
	// the best candidate from each cluster. This lets provider-specific secret
	// findings beat earlier overlapping generic/SLM spans.
	deduped := make([]Violation, 0, len(violations))
	best := violations[0]
	clusterEnd := violations[0].End
	for _, v := range violations[1:] {
		if v.Position < clusterEnd {
			if d.betterViolation(v, best) {
				best = v
			}
			if v.End > clusterEnd {
				clusterEnd = v.End
			}
			continue
		}

		deduped = append(deduped, best)
		best = v
		clusterEnd = v.End
	}
	deduped = append(deduped, best)

	sort.Slice(deduped, func(i, j int) bool {
		if deduped[i].Position != deduped[j].Position {
			return deduped[i].Position < deduped[j].Position
		}
		return d.betterViolation(deduped[i], deduped[j])
	})

	return deduped
}

func (d *Detector) betterViolation(a, b Violation) bool {
	priorityA := d.getSeverityPriority(a.Severity)
	priorityB := d.getSeverityPriority(b.Severity)
	if priorityA != priorityB {
		return priorityA > priorityB
	}
	typePriorityA := d.getTypePriority(a.Type)
	typePriorityB := d.getTypePriority(b.Type)
	if typePriorityA != typePriorityB {
		return typePriorityA > typePriorityB
	}
	secretPriorityA := d.getSecretPatternPriority(a.Pattern)
	secretPriorityB := d.getSecretPatternPriority(b.Pattern)
	if secretPriorityA != secretPriorityB {
		return secretPriorityA > secretPriorityB
	}
	lenA := a.End - a.Position
	lenB := b.End - b.Position
	if lenA != lenB {
		return lenA > lenB
	}
	return a.Position < b.Position
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

func (d *Detector) getSecretPatternPriority(patternName string) int {
	switch patternName {
	case patterns.PatternAWSAccessKeyID,
		patterns.PatternAWSSecretAccessKey,
		patterns.PatternAWSSessionToken:
		return 4
	case patterns.PatternOpenAIAPIKey,
		patterns.PatternAnthropicAPIKey,
		patterns.PatternGoogleAPIKey,
		patterns.PatternAWSAccessKey:
		return 3
	case patterns.PatternGenericSecretAssignment,
		patterns.PatternGenericAPIKeyAssignment,
		patterns.PatternGenericTokenAssignment,
		patterns.PatternGenericPrivateKeyAssignment:
		return 2
	case "slm_secret":
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
