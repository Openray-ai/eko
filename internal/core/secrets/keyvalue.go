package secrets

import (
	"regexp"
	"strings"

	"eko/internal/core/patterns"
)

const (
	minGenericSecretLength  = 20
	minGenericSecretEntropy = 3.5
)

var genericAssignmentRegex = regexp.MustCompile(`(?i)(?:\bexport\s+)?["']?([a-z0-9_. -]*(?:client_secret|private_key|api_key|apikey|credential|password|secret|token)[a-z0-9_. -]*)["']?\s*(=|:|\bis\b)\s*("[^"]+"|'[^']+'|[^\s,;]+)`)

func detectGenericKeyValues(input string, providerFindings []Finding) []Finding {
	matches := genericAssignmentRegex.FindAllStringSubmatchIndex(input, -1)
	if len(matches) == 0 {
		return nil
	}

	findings := make([]Finding, 0, len(matches))
	for _, match := range matches {
		if len(match) < 8 || match[2] < 0 || match[6] < 0 {
			continue
		}
		if overlapsAny(match[0], match[7], providerFindings) {
			continue
		}

		key := input[match[2]:match[3]]
		rawValue := input[match[6]:match[7]]
		value, _ := trimQuotedValue(rawValue, match[6])
		if !isGenericSecretValue(value) {
			continue
		}

		findings = append(findings, Finding{
			Type:     patterns.TypeCredential,
			Severity: patterns.SeverityBlock,
			Pattern:  genericPatternForKey(key),
			Matched:  input[match[0]:match[7]],
			Position: match[0],
			End:      match[7],
		})
	}
	return findings
}

func isGenericSecretValue(value string) bool {
	if len(value) < minGenericSecretLength {
		return false
	}
	if !hasAtLeastTwoCharClasses(value) {
		return false
	}
	return shannonEntropy(value) >= minGenericSecretEntropy
}

func genericPatternForKey(key string) string {
	normalized := strings.ToLower(key)
	compact := canonicalKey(key)
	switch {
	case strings.Contains(normalized, "private_key") || strings.Contains(compact, "privatekey"):
		return patterns.PatternGenericPrivateKeyAssignment
	case strings.Contains(normalized, "api_key") || strings.Contains(compact, "apikey"):
		return patterns.PatternGenericAPIKeyAssignment
	case strings.Contains(compact, "token"):
		return patterns.PatternGenericTokenAssignment
	default:
		return patterns.PatternGenericSecretAssignment
	}
}
