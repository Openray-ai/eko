package secrets

// Finding is a deterministic credential finding produced by the secrets
// detector. It intentionally mirrors detector.Violation without importing the
// detector package, so detector can call this package without an import cycle.
type Finding struct {
	Type     string
	Severity string
	Pattern  string
	Matched  string
	Position int
	End      int
}

// Detect runs provider-aware and generic contextual secret detection.
func Detect(input string) []Finding {
	aws := detectAWS(input)
	generic := detectGenericKeyValues(input, aws)

	findings := make([]Finding, 0, len(aws)+len(generic))
	findings = append(findings, aws...)
	findings = append(findings, generic...)
	return findings
}
