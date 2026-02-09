package tokenizer

import (
	"strconv"
	"strings"
	"unicode"

	"eko/internal/core/detector"
)

type TokenGenerator interface {
	Generate(v detector.Violation, vault *Vault) (string, error)
}

type Tokenizer struct {
	generators       map[string]TokenGenerator
	phoneGenerator   TokenGenerator
	defaultGenerator TokenGenerator
}

func NewTokenizer() *Tokenizer {
	emailGenerator := &EmailGenerator{}
	numericGenerator := &NumericFixedLengthGenerator{}
	phoneGenerator := &PhoneGenerator{}
	ibanGenerator := &IBANLikeGenerator{}
	defaultGenerator := &DefaultGenerator{}

	generators := map[string]TokenGenerator{
		"email":               emailGenerator,
		"nigerian_bvn":        numericGenerator,
		"nigerian_account":    numericGenerator,
		"credit_card":         numericGenerator,
		"south_african_id":    numericGenerator,
		"iban":                ibanGenerator,
		"nigerian_phone":      phoneGenerator,
		"kenyan_phone":        phoneGenerator,
		"south_african_phone": phoneGenerator,
		"ghanaian_phone":      phoneGenerator,
	}

	return &Tokenizer{
		generators:       generators,
		phoneGenerator:   phoneGenerator,
		defaultGenerator: defaultGenerator,
	}
}

func (t *Tokenizer) Generate(v detector.Violation, vault *Vault) (string, error) {
	if v.Type == "credential" {
		return "", ErrCredentialTokenization
	}

	if token, exists := vault.GetToken(v.Matched); exists {
		return token, nil
	}

	generator := t.generatorForViolation(v)
	token, err := generator.Generate(v, vault)
	if err != nil {
		return "", err
	}

	vault.mu.Lock()
	if existing, exists := vault.tokens.forward[v.Matched]; exists {
		vault.mu.Unlock()
		return existing, nil
	}
	vault.tokens.forward[v.Matched] = token
	vault.tokens.reverse[token] = v.Matched
	vault.mu.Unlock()

	return token, nil
}

func (t *Tokenizer) generatorForViolation(v detector.Violation) TokenGenerator {
	if generator, ok := t.generators[v.Pattern]; ok {
		return generator
	}
	if strings.HasSuffix(v.Pattern, "_phone") {
		return t.phoneGenerator
	}
	return t.defaultGenerator
}

type EmailGenerator struct{}

type NumericFixedLengthGenerator struct{}

type PhoneGenerator struct{}

type IBANLikeGenerator struct{}

type DefaultGenerator struct{}

func (g *EmailGenerator) Generate(v detector.Violation, vault *Vault) (string, error) {
	counter := nextCounter(vault, "email")
	return generateEmailToken(v.Matched, counter)
}

func (g *NumericFixedLengthGenerator) Generate(v detector.Violation, vault *Vault) (string, error) {
	counter := nextCounter(vault, "numeric")
	length := len(v.Matched)
	if length == 0 {
		return "", ErrTokenLengthExceeded
	}

	chars := make([]rune, length)
	digitPositions := make([]int, length)
	for i := 0; i < length; i++ {
		chars[i] = '0'
		digitPositions[i] = i
	}

	if err := replaceDigitsWithCounter(chars, digitPositions, counter); err != nil {
		return "", err
	}

	return string(chars), nil
}

func (g *PhoneGenerator) Generate(v detector.Violation, vault *Vault) (string, error) {
	counter := nextCounter(vault, "phone")
	original := []rune(v.Matched)
	preserved := phonePreservedPositions(original)

	chars := make([]rune, len(original))
	digitPositions := make([]int, 0, len(original))
	for i, ch := range original {
		switch {
		case unicode.IsDigit(ch):
			if preserved[i] {
				chars[i] = ch
			} else {
				chars[i] = '0'
				digitPositions = append(digitPositions, i)
			}
		default:
			chars[i] = ch
		}
	}

	if len(digitPositions) == 0 {
		return string(chars), nil
	}

	if err := replaceDigitsWithCounter(chars, digitPositions, counter); err != nil {
		return "", err
	}

	return string(chars), nil
}

func (g *IBANLikeGenerator) Generate(v detector.Violation, vault *Vault) (string, error) {
	counter := nextCounter(vault, "iban")
	original := []rune(v.Matched)
	chars := make([]rune, len(original))
	digitPositions := make([]int, 0, len(original))

	for i, ch := range original {
		if i < 2 && unicode.IsLetter(ch) {
			chars[i] = ch
			continue
		}

		switch {
		case unicode.IsDigit(ch):
			chars[i] = '0'
			digitPositions = append(digitPositions, i)
		case unicode.IsLetter(ch):
			if unicode.IsUpper(ch) {
				chars[i] = 'X'
			} else {
				chars[i] = 'x'
			}
		default:
			chars[i] = ch
		}
	}

	if err := replaceDigitsWithCounter(chars, digitPositions, counter); err != nil {
		return "", err
	}

	return string(chars), nil
}

func (g *DefaultGenerator) Generate(v detector.Violation, vault *Vault) (string, error) {
	counter := nextCounter(vault, "default")
	return generateClassPreservingToken(v.Matched, counter)
}

func nextCounter(vault *Vault, key string) uint64 {
	vault.mu.Lock()
	defer vault.mu.Unlock()

	vault.tokens.counters[key]++
	return vault.tokens.counters[key]
}

func generateEmailToken(original string, counter uint64) (string, error) {
	const domain = "example.com"
	atIndex := strings.Index(original, "@")
	if atIndex <= 0 || atIndex == len(original)-1 {
		return generateClassPreservingToken(original, counter)
	}

	localOriginal := original[:atIndex]
	domainOriginal := original[atIndex+1:]
	if localOriginal == "" || domainOriginal == "" {
		return generateClassPreservingToken(original, counter)
	}

	counterStr := strings.ToLower(strconv.FormatUint(counter, 36))
	baseLocal := buildPaddedPart("user_"+counterStr, len(localOriginal))
	baseDomain := buildPaddedPart(domain, len(domainOriginal))

	local := adjustPartForClass(localOriginal, baseLocal)
	domainPart := adjustPartForClass(domainOriginal, baseDomain)

	localWithCounter, err := applyCounterToPart(localOriginal, local, counter)
	if err != nil {
		return "", err
	}

	return localWithCounter + "@" + domainPart, nil
}

func generateClassPreservingToken(original string, counter uint64) (string, error) {
	originalRunes := []rune(original)
	chars, digitPositions, letterPositions := classPreservingTemplate(originalRunes)
	if len(digitPositions) > 0 {
		if err := replaceDigitsWithCounter(chars, digitPositions, counter); err != nil {
			return "", err
		}
		return string(chars), nil
	}

	if len(letterPositions) > 0 {
		if err := replaceLettersWithCounter(chars, originalRunes, letterPositions, counter); err != nil {
			return "", err
		}
		return string(chars), nil
	}

	return string(chars), nil
}

func classPreservingTemplate(original []rune) ([]rune, []int, []int) {
	chars := make([]rune, len(original))
	digitPositions := make([]int, 0, len(original))
	letterPositions := make([]int, 0, len(original))

	for i, ch := range original {
		switch {
		case unicode.IsDigit(ch):
			chars[i] = '0'
			digitPositions = append(digitPositions, i)
		case unicode.IsLetter(ch):
			if unicode.IsUpper(ch) {
				chars[i] = 'X'
			} else {
				chars[i] = 'x'
			}
			letterPositions = append(letterPositions, i)
		default:
			chars[i] = ch
		}
	}

	return chars, digitPositions, letterPositions
}

func replaceDigitsWithCounter(chars []rune, digitPositions []int, counter uint64) error {
	counterStr := strconv.FormatUint(counter, 10)
	if len(counterStr) > len(digitPositions) {
		return ErrTokenLengthExceeded
	}

	posIdx := len(digitPositions) - 1
	for i := len(counterStr) - 1; i >= 0; i-- {
		pos := digitPositions[posIdx]
		chars[pos] = rune(counterStr[i])
		posIdx--
	}

	return nil
}

func replaceLettersWithCounter(chars []rune, original []rune, letterPositions []int, counter uint64) error {
	letterDigits := base26Digits(counter)
	if len(letterDigits) > len(letterPositions) {
		return ErrTokenLengthExceeded
	}

	posIdx := len(letterPositions) - 1
	for i := len(letterDigits) - 1; i >= 0; i-- {
		pos := letterPositions[posIdx]
		if unicode.IsUpper(original[pos]) {
			chars[pos] = rune('A' + letterDigits[i])
		} else {
			chars[pos] = rune('a' + letterDigits[i])
		}
		posIdx--
	}

	return nil
}

func base26Digits(counter uint64) []int {
	if counter == 0 {
		return []int{0}
	}

	value := counter - 1
	var digits []int
	for {
		digits = append(digits, int(value%26))
		value /= 26
		if value == 0 {
			break
		}
	}

	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}

	return digits
}

func buildPaddedPart(base string, length int) string {
	if length <= len(base) {
		return base[:length]
	}
	return base + strings.Repeat("a", length-len(base))
}

func adjustPartForClass(original, candidate string) string {
	origRunes := []rune(original)
	candRunes := []rune(candidate)
	if len(origRunes) != len(candRunes) {
		return candidate
	}

	for i, ch := range origRunes {
		got := candRunes[i]
		switch {
		case unicode.IsDigit(ch):
			if !unicode.IsDigit(got) {
				candRunes[i] = '0'
			}
		case unicode.IsLetter(ch):
			if !unicode.IsLetter(got) {
				if unicode.IsUpper(ch) {
					candRunes[i] = 'A'
				} else {
					candRunes[i] = 'a'
				}
				continue
			}
			if unicode.IsUpper(ch) && unicode.IsLower(got) {
				candRunes[i] = unicode.ToUpper(got)
			} else if unicode.IsLower(ch) && unicode.IsUpper(got) {
				candRunes[i] = unicode.ToLower(got)
			}
		default:
			if unicode.IsDigit(got) || unicode.IsLetter(got) {
				candRunes[i] = ch
			}
		}
	}

	return string(candRunes)
}

func applyCounterToPart(original, candidate string, counter uint64) (string, error) {
	origRunes := []rune(original)
	candRunes := []rune(candidate)
	if len(origRunes) != len(candRunes) {
		return candidate, nil
	}

	_, digitPositions, letterPositions := classPreservingTemplate(origRunes)
	if len(digitPositions) > 0 {
		if err := replaceDigitsWithCounter(candRunes, digitPositions, counter); err != nil {
			return "", err
		}
		return string(candRunes), nil
	}
	if len(letterPositions) > 0 {
		if err := replaceLettersWithCounter(candRunes, origRunes, letterPositions, counter); err != nil {
			return "", err
		}
		return string(candRunes), nil
	}

	return string(candRunes), nil
}

func phonePreservedPositions(original []rune) map[int]bool {
	preserved := map[int]bool{}
	if len(original) == 0 {
		return preserved
	}

	if original[0] == '+' {
		rest := string(original[1:])
		codes := []string{"234", "254", "233", "27"}
		for _, code := range codes {
			if strings.HasPrefix(rest, code) {
				for i := 0; i < len(code); i++ {
					preserved[1+i] = true
				}
				break
			}
		}
		return preserved
	}

	if original[0] == '0' {
		preserved[0] = true
	}

	return preserved
}
