package slm

import (
	"maps"

	"eko/internal/core/patterns"
)

// DefaultLabels is the canonical mapping from SLM span labels (as emitted by
// iamSamurai/privacy-filter-nigeria over openai/privacy-filter) to Ekō
// Violation Type/Severity/Pattern fields. Operators can override individual
// entries via config.
func DefaultLabels() map[string]LabelMapping {
	return map[string]LabelMapping{
		"private_bvn":                    {Type: patterns.TypePII, Severity: patterns.SeverityBlock, Pattern: "slm_bvn"},
		"private_nin":                    {Type: patterns.TypePII, Severity: patterns.SeverityBlock, Pattern: "slm_nin"},
		"account_number":                 {Type: patterns.TypeFinancial, Severity: patterns.SeverityBlock, Pattern: "slm_account_number"},
		"private_passport_number":        {Type: patterns.TypePII, Severity: patterns.SeverityBlock, Pattern: "slm_passport"},
		"private_drivers_license_number": {Type: patterns.TypePII, Severity: patterns.SeverityBlock, Pattern: "slm_drivers_license"},
		"private_voters_card_number":     {Type: patterns.TypePII, Severity: patterns.SeverityBlock, Pattern: "slm_voters_card"},
		"secret":                         {Type: patterns.TypeCredential, Severity: patterns.SeverityBlock, Pattern: "slm_secret"},
		"private_email":                  {Type: patterns.TypePII, Severity: patterns.SeverityWarn, Pattern: "slm_email"},
		"private_phone":                  {Type: patterns.TypePII, Severity: patterns.SeverityWarn, Pattern: "slm_phone"},
		"private_person":                 {Type: patterns.TypePII, Severity: patterns.SeverityWarn, Pattern: "slm_person"},
		"private_address":                {Type: patterns.TypePII, Severity: patterns.SeverityWarn, Pattern: "slm_address"},
		"private_url":                    {Type: patterns.TypePII, Severity: patterns.SeverityLog, Pattern: "slm_url"},
		"private_date":                   {Type: patterns.TypePII, Severity: patterns.SeverityLog, Pattern: "slm_date"},
	}
}

// MergeLabelOverrides applies user overrides on top of defaults. Empty fields
// in an override fall back to the default; unknown labels in overrides are
// added to the result.
func MergeLabelOverrides(defaults, overrides map[string]LabelMapping) map[string]LabelMapping {
	merged := make(map[string]LabelMapping, len(defaults)+len(overrides))
	maps.Copy(merged, defaults)
	for k, ov := range overrides {
		cur := merged[k]
		if ov.Type != "" {
			cur.Type = ov.Type
		}
		if ov.Severity != "" {
			cur.Severity = ov.Severity
		}
		if ov.Pattern != "" {
			cur.Pattern = ov.Pattern
		}
		merged[k] = cur
	}
	return merged
}
