package detector

import (
	"encoding/base64"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"eko/internal/core/patterns"
)

var (
	cardCandidateRegex       = regexp.MustCompile(`(?:\d[ -]?){13,19}`)
	phoneCandidateRegex      = regexp.MustCompile(`\+?\d(?:[\d\s().-]{6,}\d)`)
	ibanCandidateRegex       = regexp.MustCompile(`(?i)\b[a-z]{2}[0-9]{2}[a-z0-9 ]{11,40}\b`)
	ibanCanonicalRegex       = regexp.MustCompile(`^[A-Z]{2}[0-9]{2}[A-Z0-9]{11,30}$`)
	passwordAssignRegex      = regexp.MustCompile(`(?i)\b([a-zA-Z0-9@._$!-]{3,32})\s*[:=]\s*("[^"]+"|'[^']+'|[^\s]+)`)
	splitEmailRegex          = regexp.MustCompile(`\b([A-Za-z0-9._%+-]{1,64})\s*(?:\+\s*)?@\s*([A-Za-z0-9.-]+\.[A-Za-z]{2,})\b`)
	base64CandidateRegex     = regexp.MustCompile(`[A-Za-z0-9+/]{24,}={0,2}`)
	credentialSignatureRegex = regexp.MustCompile(`(?i)(?:\b(?:AKIA|ASIA)[0-9A-Z]{15,16}\b|\bsk-(?:[A-Za-z0-9]{48}|(?:proj|live|test)-[A-Za-z0-9_-]{20,})\b|postgres://|mongodb(?:\+srv)?://|mysql://|-----BEGIN (?:RSA|DSA|EC|OPENSSH) PRIVATE KEY-----)`)
	nigeriaIntlPhoneRegex    = regexp.MustCompile(`^\+234\d{10}$`)
	nigeriaLocalPhoneRegex   = regexp.MustCompile(`^0[789][01]\d{8}$`)
	kenyaIntlPhoneRegex      = regexp.MustCompile(`^\+254\d{9}$`)
	kenyaLocalPhoneRegex     = regexp.MustCompile(`^07\d{8}$`)
	saIntlPhoneRegex         = regexp.MustCompile(`^\+27\d{9}$`)
	saLocalPhoneRegex        = regexp.MustCompile(`^0[6-8]\d{8}$`)
	ghanaIntlPhoneRegex      = regexp.MustCompile(`^\+233\d{9}$`)
	ghanaLocalPhoneRegex     = regexp.MustCompile(`^0[2-5]\d{8}$`)
)

type normalizedView struct {
	text      string
	byteStart []int
	byteEnd   []int
}

func (v normalizedView) mapRange(start, end int) (int, int, bool) {
	if start < 0 || end <= start || end > len(v.byteStart) {
		return 0, 0, false
	}
	origStart := v.byteStart[start]
	origEnd := v.byteEnd[end-1]
	if origStart < 0 || origEnd <= origStart {
		return 0, 0, false
	}
	return origStart, origEnd, true
}

func (d *Detector) detectAdvanced(input string, patternMap map[string]*patterns.CompiledPattern) []Violation {
	violations := make([]Violation, 0)
	violations = append(violations, d.detectCanonicalEmails(input, patternMap)...)
	violations = append(violations, d.detectPhonesWithSeparators(input, patternMap)...)
	violations = append(violations, d.detectCreditCardsWithSeparators(input, patternMap)...)
	violations = append(violations, d.detectCanonicalIBAN(input, patternMap)...)
	violations = append(violations, d.detectObfuscatedPasswordAssignments(input, patternMap)...)
	violations = append(violations, d.detectSplitEmails(input, patternMap)...)
	violations = append(violations, d.detectBase64EncodedSecrets(input)...)
	violations = append(violations, d.detectEscapedCredentialURIs(input, patternMap)...)
	return violations
}

func (d *Detector) detectCanonicalEmails(input string, patternMap map[string]*patterns.CompiledPattern) []Violation {
	pattern := patternMap[patterns.PatternEmail]
	if pattern == nil {
		return nil
	}

	view := canonicalizeForEmail(input)
	matches := pattern.Regex.FindAllStringIndex(view.text, -1)
	if len(matches) == 0 {
		return nil
	}

	violations := make([]Violation, 0, len(matches))
	for _, m := range matches {
		start, end, ok := view.mapRange(m[0], m[1])
		if !ok || end > len(input) {
			continue
		}
		canonicalMatched := view.text[m[0]:m[1]]
		violations = append(violations, Violation{
			Type:     pattern.Type,
			Severity: pattern.Severity,
			Pattern:  pattern.Name,
			Matched:  canonicalMatched,
			Position: start,
			End:      end,
		})
	}
	return violations
}

func canonicalizeForEmail(input string) normalizedView {
	text, starts, ends := make([]byte, 0, len(input)), make([]int, 0, len(input)), make([]int, 0, len(input))
	for i := 0; i < len(input); {
		if i+2 < len(input) && input[i] == '%' && isHex(input[i+1]) && isHex(input[i+2]) {
			decoded, err := strconv.ParseUint(input[i+1:i+3], 16, 8)
			if err == nil {
				b := byte(decoded)
				text = append(text, b)
				starts = append(starts, i)
				ends = append(ends, i+3)
				i += 3
				continue
			}
		}

		r, size := utf8.DecodeRuneInString(input[i:])
		folded := foldConfusableRune(r)
		buf := make([]byte, utf8.RuneLen(folded))
		utf8.EncodeRune(buf, folded)
		for _, b := range buf {
			text = append(text, b)
			starts = append(starts, i)
			ends = append(ends, i+size)
		}
		i += size
	}
	return normalizedView{text: string(text), byteStart: starts, byteEnd: ends}
}

func foldConfusableRune(r rune) rune {
	switch r {
	case 'а':
		return 'a'
	case 'е':
		return 'e'
	case 'о':
		return 'o'
	case 'р':
		return 'p'
	case 'с':
		return 'c'
	case 'х':
		return 'x'
	case 'у':
		return 'y'
	case 'і':
		return 'i'
	case 'ј':
		return 'j'
	case 'А':
		return 'A'
	case 'В':
		return 'B'
	case 'Е':
		return 'E'
	case 'К':
		return 'K'
	case 'М':
		return 'M'
	case 'Н':
		return 'H'
	case 'О':
		return 'O'
	case 'Р':
		return 'P'
	case 'С':
		return 'C'
	case 'Т':
		return 'T'
	case 'Х':
		return 'X'
	case 'Υ':
		return 'Y'
	default:
		return r
	}
}

func (d *Detector) detectPhonesWithSeparators(input string, patternMap map[string]*patterns.CompiledPattern) []Violation {
	locs := phoneCandidateRegex.FindAllStringIndex(input, -1)
	if len(locs) == 0 {
		return nil
	}

	violations := make([]Violation, 0, len(locs))
	for _, loc := range locs {
		raw := input[loc[0]:loc[1]]
		normalized := normalizePhone(raw)
		patternName := regionalPhonePattern(normalized)
		if patternName == "" {
			continue
		}
		pattern := patternMap[patternName]
		if pattern == nil {
			continue
		}
		violations = append(violations, Violation{
			Type:     pattern.Type,
			Severity: pattern.Severity,
			Pattern:  pattern.Name,
			Matched:  raw,
			Position: loc[0],
			End:      loc[1],
		})
	}
	return violations
}

func normalizePhone(value string) string {
	var b strings.Builder
	b.Grow(len(value))
	for i, r := range value {
		if r == '+' && i == 0 {
			b.WriteRune(r)
			continue
		}
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func regionalPhonePattern(value string) string {
	switch {
	case nigeriaIntlPhoneRegex.MatchString(value), nigeriaLocalPhoneRegex.MatchString(value):
		return patterns.PatternNigerianPhone
	case kenyaIntlPhoneRegex.MatchString(value), kenyaLocalPhoneRegex.MatchString(value):
		return patterns.PatternKenyanPhone
	case saIntlPhoneRegex.MatchString(value), saLocalPhoneRegex.MatchString(value):
		return patterns.PatternSouthAfricanPhone
	case ghanaIntlPhoneRegex.MatchString(value), ghanaLocalPhoneRegex.MatchString(value):
		return patterns.PatternGhanaianPhone
	default:
		return ""
	}
}

func (d *Detector) detectCreditCardsWithSeparators(input string, patternMap map[string]*patterns.CompiledPattern) []Violation {
	pattern := patternMap[patterns.PatternCreditCard]
	if pattern == nil {
		return nil
	}

	locs := cardCandidateRegex.FindAllStringIndex(input, -1)
	if len(locs) == 0 {
		return nil
	}

	violations := make([]Violation, 0, len(locs))
	for _, loc := range locs {
		raw := strings.TrimSpace(input[loc[0]:loc[1]])
		digits := stripNonDigits(raw)
		if len(digits) < 13 || len(digits) > 19 {
			continue
		}
		if !passesLuhn(digits) {
			continue
		}
		violations = append(violations, Violation{
			Type:     pattern.Type,
			Severity: pattern.Severity,
			Pattern:  pattern.Name,
			Matched:  digits,
			Position: loc[0],
			End:      loc[1],
		})
	}
	return violations
}

func stripNonDigits(value string) string {
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func passesLuhn(number string) bool {
	sum := 0
	alt := false
	for i := len(number) - 1; i >= 0; i-- {
		d := int(number[i] - '0')
		if d < 0 || d > 9 {
			return false
		}
		if alt {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		alt = !alt
	}
	return sum%10 == 0
}

func (d *Detector) detectCanonicalIBAN(input string, patternMap map[string]*patterns.CompiledPattern) []Violation {
	pattern := patternMap[patterns.PatternIBAN]
	if pattern == nil {
		return nil
	}

	locs := ibanCandidateRegex.FindAllStringIndex(input, -1)
	if len(locs) == 0 {
		return nil
	}

	violations := make([]Violation, 0, len(locs))
	for _, loc := range locs {
		raw := input[loc[0]:loc[1]]
		canonical := strings.ToUpper(strings.ReplaceAll(raw, " ", ""))
		if !ibanCanonicalRegex.MatchString(canonical) {
			continue
		}
		if !passesIBANMod97(canonical) {
			continue
		}
		violations = append(violations, Violation{
			Type:     pattern.Type,
			Severity: pattern.Severity,
			Pattern:  pattern.Name,
			Matched:  canonical,
			Position: loc[0],
			End:      loc[1],
		})
	}
	return violations
}

func passesIBANMod97(iban string) bool {
	if len(iban) < 15 || len(iban) > 34 {
		return false
	}
	rearranged := iban[4:] + iban[:4]
	remainder := 0
	for _, r := range rearranged {
		switch {
		case r >= '0' && r <= '9':
			remainder = (remainder*10 + int(r-'0')) % 97
		case r >= 'A' && r <= 'Z':
			val := int(r-'A') + 10
			remainder = (remainder*100 + val) % 97
		default:
			return false
		}
	}
	return remainder == 1
}

func (d *Detector) detectObfuscatedPasswordAssignments(input string, patternMap map[string]*patterns.CompiledPattern) []Violation {
	pattern := patternMap[patterns.PatternPasswordVar]
	if pattern == nil {
		return nil
	}

	matches := passwordAssignRegex.FindAllStringSubmatchIndex(input, -1)
	if len(matches) == 0 {
		return nil
	}

	violations := make([]Violation, 0, len(matches))
	for _, m := range matches {
		if len(m) < 6 {
			continue
		}
		fullStart, fullEnd := m[0], m[1]
		key := input[m[2]:m[3]]
		value := input[m[4]:m[5]]
		if !looksLikePasswordKey(key) {
			continue
		}
		value = strings.Trim(value, `"'`)
		if len(value) < 8 {
			continue
		}
		violations = append(violations, Violation{
			Type:     pattern.Type,
			Severity: pattern.Severity,
			Pattern:  pattern.Name,
			Matched:  input[fullStart:fullEnd],
			Position: fullStart,
			End:      fullEnd,
		})
	}
	return violations
}

func looksLikePasswordKey(key string) bool {
	key = strings.ToLower(key)
	key = strings.Map(func(r rune) rune {
		switch r {
		case '@':
			return 'a'
		case '0':
			return 'o'
		case '$', '5':
			return 's'
		case '1', '!':
			return 'i'
		case '3':
			return 'e'
		case '7':
			return 't'
		default:
			if unicode.IsLetter(r) {
				return r
			}
			return -1
		}
	}, key)
	return key == "password" || key == "passwd" || key == "pwd"
}

func (d *Detector) detectSplitEmails(input string, patternMap map[string]*patterns.CompiledPattern) []Violation {
	pattern := patternMap[patterns.PatternEmail]
	if pattern == nil {
		return nil
	}

	matches := splitEmailRegex.FindAllStringSubmatchIndex(input, -1)
	if len(matches) == 0 {
		return nil
	}

	violations := make([]Violation, 0, len(matches))
	for _, m := range matches {
		if len(m) < 6 {
			continue
		}
		full := input[m[0]:m[1]]
		if !strings.Contains(full, " ") && !strings.Contains(full, "+") {
			continue
		}
		local := input[m[2]:m[3]]
		domain := input[m[4]:m[5]]
		canonical := local + "@" + domain
		violations = append(violations, Violation{
			Type:     pattern.Type,
			Severity: pattern.Severity,
			Pattern:  pattern.Name,
			Matched:  canonical,
			Position: m[0],
			End:      m[1],
		})
	}
	return violations
}

func (d *Detector) detectBase64EncodedSecrets(input string) []Violation {
	locs := base64CandidateRegex.FindAllStringIndex(input, -1)
	if len(locs) == 0 {
		return nil
	}

	violations := make([]Violation, 0)
	for _, loc := range locs {
		candidate := input[loc[0]:loc[1]]
		if len(candidate)%4 != 0 {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(candidate)
		if err != nil || len(decoded) == 0 || len(decoded) > 4096 {
			continue
		}
		if !credentialSignatureRegex.Match(decoded) {
			continue
		}
		violations = append(violations, Violation{
			Type:     patterns.TypeCredential,
			Severity: patterns.SeverityWarn,
			Pattern:  "base64_encoded_secret",
			Matched:  candidate,
			Position: loc[0],
			End:      loc[1],
		})
	}

	return violations
}

func (d *Detector) detectEscapedCredentialURIs(input string, patternMap map[string]*patterns.CompiledPattern) []Violation {
	view := collapseEscapedWhitespace(input)
	if view.text == input {
		return nil
	}

	candidates := []string{patterns.PatternPostgresConn, patterns.PatternMongoDBConn, patterns.PatternMySQLConn}
	violations := make([]Violation, 0)
	for _, name := range candidates {
		pattern := patternMap[name]
		if pattern == nil {
			continue
		}
		matches := pattern.Regex.FindAllStringIndex(view.text, -1)
		for _, m := range matches {
			start, end, ok := view.mapRange(m[0], m[1])
			if !ok || end > len(input) {
				continue
			}
			violations = append(violations, Violation{
				Type:     pattern.Type,
				Severity: pattern.Severity,
				Pattern:  pattern.Name,
				Matched:  input[start:end],
				Position: start,
				End:      end,
			})
		}
	}
	return violations
}

func collapseEscapedWhitespace(input string) normalizedView {
	text, starts, ends := make([]byte, 0, len(input)), make([]int, 0, len(input)), make([]int, 0, len(input))
	for i := 0; i < len(input); {
		if i+1 < len(input) && input[i] == '\\' {
			next := input[i+1]
			if next == 'n' || next == 'r' || next == 't' {
				i += 2
				continue
			}
		}

		r, size := utf8.DecodeRuneInString(input[i:])
		if r == '\n' || r == '\r' || r == '\t' {
			i += size
			continue
		}
		buf := make([]byte, utf8.RuneLen(r))
		utf8.EncodeRune(buf, r)
		for _, b := range buf {
			text = append(text, b)
			starts = append(starts, i)
			ends = append(ends, i+size)
		}
		i += size
	}
	return normalizedView{text: string(text), byteStart: starts, byteEnd: ends}
}

func isHex(value byte) bool {
	return (value >= '0' && value <= '9') ||
		(value >= 'a' && value <= 'f') ||
		(value >= 'A' && value <= 'F')
}
