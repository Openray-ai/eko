package sanitizer

import (
	"fmt"
	"strings"
)

// Change represents a single change between original and sanitized text
type Change struct {
	Position int    `json:"position"`
	OldText  string `json:"old_text"`
	NewText  string `json:"new_text"`
	Length   int    `json:"length"`
}

// DiffResult contains detailed diff information
type DiffResult struct {
	OriginalPrompt  string   `json:"original_prompt"`
	SanitizedPrompt string   `json:"sanitized_prompt"`
	Changes         []Change `json:"changes"`
	TotalChanges    int      `json:"total_changes"`
	DiffText        string   `json:"diff_text,omitempty"`        // Human-readable diff
	ColorizedDiff   string   `json:"colorized_diff,omitempty"`   // Terminal-colored diff
}

// GenerateDiff creates a detailed diff between original and sanitized text
func GenerateDiff(result *Result) *DiffResult {
	if result == nil {
		return nil
	}

	diff := &DiffResult{
		OriginalPrompt:  result.OriginalPrompt,
		SanitizedPrompt: result.SanitizedPrompt,
		Changes:         extractChanges(result),
	}

	diff.TotalChanges = len(diff.Changes)
	diff.DiffText = formatDiffText(diff)
	diff.ColorizedDiff = formatColorizedDiff(diff)

	return diff
}

// extractChanges extracts individual changes from violations
func extractChanges(result *Result) []Change {
	changes := make([]Change, 0, len(result.Violations))

	for _, v := range result.Violations {
		// Only include violations that were actually redacted
		if v.Severity == "BLOCK" || v.Severity == "WARN" {
			changes = append(changes, Change{
				Position: v.Position,
				OldText:  v.Matched,
				NewText:  getRedactionTextForViolation(v),
				Length:   v.End - v.Position,
			})
		}
	}

	return changes
}

// getRedactionTextForViolation returns the redaction text used for a violation
func getRedactionTextForViolation(v interface{}) string {
	// This is a simplified version - in practice, would use the same logic as sanitizer
	return "[REDACTED]"
}

// formatDiffText creates a git-like diff output
func formatDiffText(diff *DiffResult) string {
	if diff.TotalChanges == 0 {
		return "No changes"
	}

	var sb strings.Builder
	sb.WriteString("--- Original\n")
	sb.WriteString("+++ Sanitized\n")
	sb.WriteString("\n")

	// Split into lines for better diff display
	originalLines := strings.Split(diff.OriginalPrompt, "\n")
	sanitizedLines := strings.Split(diff.SanitizedPrompt, "\n")

	maxLines := len(originalLines)
	if len(sanitizedLines) > maxLines {
		maxLines = len(sanitizedLines)
	}

	for i := 0; i < maxLines; i++ {
		origLine := ""
		if i < len(originalLines) {
			origLine = originalLines[i]
		}

		sanitLine := ""
		if i < len(sanitizedLines) {
			sanitLine = sanitizedLines[i]
		}

		if origLine != sanitLine {
			if origLine != "" {
				sb.WriteString(fmt.Sprintf("-%s\n", origLine))
			}
			if sanitLine != "" {
				sb.WriteString(fmt.Sprintf("+%s\n", sanitLine))
			}
		} else {
			sb.WriteString(fmt.Sprintf(" %s\n", origLine))
		}
	}

	return sb.String()
}

// formatColorizedDiff creates a terminal-colorized diff output
func formatColorizedDiff(diff *DiffResult) string {
	if diff.TotalChanges == 0 {
		return "\033[32mNo changes - input is safe\033[0m"
	}

	var sb strings.Builder

	// Header
	sb.WriteString("\033[1m")  // Bold
	sb.WriteString("Sanitization Changes:\n")
	sb.WriteString("\033[0m")  // Reset
	sb.WriteString("\n")

	// Split into lines
	originalLines := strings.Split(diff.OriginalPrompt, "\n")
	sanitizedLines := strings.Split(diff.SanitizedPrompt, "\n")

	maxLines := len(originalLines)
	if len(sanitizedLines) > maxLines {
		maxLines = len(sanitizedLines)
	}

	for i := 0; i < maxLines; i++ {
		origLine := ""
		if i < len(originalLines) {
			origLine = originalLines[i]
		}

		sanitLine := ""
		if i < len(sanitizedLines) {
			sanitLine = sanitizedLines[i]
		}

		if origLine != sanitLine {
			if origLine != "" {
				sb.WriteString("\033[31m")  // Red
				sb.WriteString(fmt.Sprintf("- %s\n", origLine))
				sb.WriteString("\033[0m")   // Reset
			}
			if sanitLine != "" {
				sb.WriteString("\033[32m")  // Green
				sb.WriteString(fmt.Sprintf("+ %s\n", sanitLine))
				sb.WriteString("\033[0m")   // Reset
			}
		} else {
			sb.WriteString(fmt.Sprintf("  %s\n", origLine))
		}
	}

	// Summary
	sb.WriteString("\n")
	sb.WriteString("\033[1m")  // Bold
	sb.WriteString(fmt.Sprintf("Total changes: %d\n", diff.TotalChanges))
	sb.WriteString("\033[0m")  // Reset

	return sb.String()
}

// FormatSummary creates a concise summary of sanitization results
func FormatSummary(result *Result) string {
	if result.Safe {
		return "✓ Input is safe - no sensitive data detected"
	}

	return fmt.Sprintf(
		"⚠ Sanitized %d violation(s) in %.2fms\n"+
		"  - Total violations: %d\n"+
		"  - Redacted: %d\n"+
		"  - Original length: %d chars\n"+
		"  - Sanitized length: %d chars",
		result.RedactedCount,
		result.ProcessingTimeMs,
		len(result.Violations),
		result.RedactedCount,
		len(result.OriginalPrompt),
		len(result.SanitizedPrompt),
	)
}

// FormatViolationsList creates a formatted list of violations
func FormatViolationsList(result *Result) string {
	if len(result.Violations) == 0 {
		return "No violations found"
	}

	var sb strings.Builder
	sb.WriteString("Violations Detected:\n\n")

	for i, v := range result.Violations {
		sb.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, v.Severity, v.Pattern))
		sb.WriteString(fmt.Sprintf("   Type: %s\n", v.Type))
		sb.WriteString(fmt.Sprintf("   Position: %d\n", v.Position))
		sb.WriteString(fmt.Sprintf("   Matched: \"%s\"\n", truncate(v.Matched, 50)))
		sb.WriteString("\n")
	}

	return sb.String()
}

// truncate truncates a string to maxLen with ellipsis
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
