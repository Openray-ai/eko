package secrets

import (
	"math"
	"strings"
	"unicode"
)

func overlaps(aStart, aEnd, bStart, bEnd int) bool {
	return aStart < bEnd && bStart < aEnd
}

func overlapsAny(start, end int, findings []Finding) bool {
	for _, f := range findings {
		if overlaps(start, end, f.Position, f.End) {
			return true
		}
	}
	return false
}

func trimQuotedValue(value string, start int) (string, int) {
	value = strings.TrimRightFunc(value, func(r rune) bool {
		return unicode.IsSpace(r) || r == ',' || r == ';'
	})
	if len(value) < 2 {
		return value, start
	}
	first := value[0]
	if first != '"' && first != '\'' {
		return value, start
	}
	closing := strings.IndexByte(value[1:], first)
	if closing < 0 {
		return value[1:], start + 1
	}
	return value[1 : closing+1], start + 1
}

func canonicalKey(key string) string {
	var b strings.Builder
	for _, r := range key {
		switch {
		case r == '-' || r == '_' || unicode.IsSpace(r):
			continue
		default:
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

func hasAtLeastTwoCharClasses(value string) bool {
	classes := 0
	var lower, upper, digit, symbol bool
	for _, r := range value {
		switch {
		case unicode.IsLower(r):
			lower = true
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsDigit(r):
			digit = true
		default:
			symbol = true
		}
	}
	for _, present := range []bool{lower, upper, digit, symbol} {
		if present {
			classes++
		}
	}
	return classes >= 2
}

func shannonEntropy(value string) float64 {
	if value == "" {
		return 0
	}
	counts := make(map[rune]int)
	total := 0
	for _, r := range value {
		counts[r]++
		total++
	}

	entropy := 0.0
	for _, count := range counts {
		p := float64(count) / float64(total)
		entropy -= p * math.Log2(p)
	}
	return entropy
}
