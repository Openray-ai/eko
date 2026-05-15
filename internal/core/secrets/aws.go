package secrets

import (
	"regexp"

	"eko/internal/core/patterns"
)

var (
	awsAccessKeyIDRegex  = regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`)
	awsAssignmentRegex   = regexp.MustCompile(`(?i)(?:\bexport\s+)?["']?([A-Z0-9_ -]*AWS[A-Z0-9_ -]*SECRET[A-Z0-9_ -]*ACCESS[A-Z0-9_ -]*KEY|awsSecretAccessKey|secret_access_key|SecretAccessKey|AWS_SESSION_TOKEN|aws_session_token|awsSessionToken|session_token|SessionToken)["']?\s*(=|:|\bis\b)\s*("[^"]+"|'[^']+'|[^\s,;]+)`)
	awsSecretValueRegex  = regexp.MustCompile(`^[A-Za-z0-9/+=]{40}$`)
	awsSessionValueRegex = regexp.MustCompile(`^[A-Za-z0-9/+=]{80,}$`)
)

func detectAWS(input string) []Finding {
	var findings []Finding
	findings = append(findings, detectAWSAccessKeyIDs(input)...)
	findings = append(findings, detectAWSAssignments(input)...)
	return findings
}

func detectAWSAccessKeyIDs(input string) []Finding {
	matches := awsAccessKeyIDRegex.FindAllStringIndex(input, -1)
	if len(matches) == 0 {
		return nil
	}
	findings := make([]Finding, 0, len(matches))
	for _, match := range matches {
		findings = append(findings, Finding{
			Type:     patterns.TypeCredential,
			Severity: patterns.SeverityBlock,
			Pattern:  patterns.PatternAWSAccessKeyID,
			Matched:  input[match[0]:match[1]],
			Position: match[0],
			End:      match[1],
		})
	}
	return findings
}

func detectAWSAssignments(input string) []Finding {
	matches := awsAssignmentRegex.FindAllStringSubmatchIndex(input, -1)
	if len(matches) == 0 {
		return nil
	}

	findings := make([]Finding, 0, len(matches))
	for _, match := range matches {
		if len(match) < 8 || match[4] < 0 || match[6] < 0 {
			continue
		}
		key := input[match[2]:match[3]]
		rawValue := input[match[6]:match[7]]
		value, _ := trimQuotedValue(rawValue, match[6])

		switch {
		case isAWSSecretAccessKey(key) && awsSecretValueRegex.MatchString(value):
			findings = append(findings, Finding{
				Type:     patterns.TypeCredential,
				Severity: patterns.SeverityBlock,
				Pattern:  patterns.PatternAWSSecretAccessKey,
				Matched:  input[match[0]:match[7]],
				Position: match[0],
				End:      match[7],
			})
		case isAWSSessionToken(key) && awsSessionValueRegex.MatchString(value):
			findings = append(findings, Finding{
				Type:     patterns.TypeCredential,
				Severity: patterns.SeverityBlock,
				Pattern:  patterns.PatternAWSSessionToken,
				Matched:  input[match[0]:match[7]],
				Position: match[0],
				End:      match[7],
			})
		}
	}
	return findings
}

func isAWSSecretAccessKey(key string) bool {
	switch canonicalKey(key) {
	case "awssecretaccesskey", "secretaccesskey":
		return true
	default:
		return false
	}
}

func isAWSSessionToken(key string) bool {
	switch canonicalKey(key) {
	case "awssessiontoken", "sessiontoken":
		return true
	default:
		return false
	}
}
