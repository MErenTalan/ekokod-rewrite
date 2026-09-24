package auth

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Policy violation codes (R146), in the order CheckPolicy reports them.
const (
	PolicyTooShort   = "password_too_short"
	PolicyTooLong    = "password_too_long"
	PolicyComplexity = "password_complexity"
	PolicyRepeat     = "password_repeat"
	PolicyCommon     = "password_common"
	PolicyPersonal   = "password_personal"
	PolicyReused     = "password_reused"
)

const (
	minPasswordRunes = 10
	maxPasswordRunes = 128
	minFragmentRunes = 3
)

const specialChars = " !\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~"

var commonSequences = []string{"1234", "abcd", "qwer", "asdf", "zxcv", "password", "passw0rd", "1111", "0000"}

// PolicyInput is everything the policy needs about a candidate password.
type PolicyInput struct {
	Password, Name, Email, CompanyName string
	CurrentHash                        string
	History                            []string
}

// CheckPolicy returns every violated rule, or nil when the password is acceptable.
func CheckPolicy(h Hasher, in PolicyInput) []string {
	var out []string
	pw := in.Password
	n := utf8.RuneCountInString(pw)
	if n < minPasswordRunes {
		out = append(out, PolicyTooShort)
	}
	if n > maxPasswordRunes {
		out = append(out, PolicyTooLong)
	}
	if !meetsComplexity(pw) {
		out = append(out, PolicyComplexity)
	}
	if hasRun(pw, 4) {
		out = append(out, PolicyRepeat)
	}
	lower := turkishLower(pw)
	for _, seq := range commonSequences {
		if strings.Contains(lower, seq) {
			out = append(out, PolicyCommon)
			break
		}
	}
	for _, frag := range personalFragments(in) {
		if strings.Contains(lower, frag) {
			out = append(out, PolicyPersonal)
			break
		}
	}
	if n <= maxPasswordRunes && reused(h, in) {
		out = append(out, PolicyReused)
	}
	return out
}

func meetsComplexity(pw string) bool {
	var lower, upper, digit, special bool
	for _, r := range pw {
		switch {
		case unicode.IsLower(r):
			lower = true
		case unicode.IsUpper(r):
			upper = true
		case r >= '0' && r <= '9':
			digit = true
		case strings.ContainsRune(specialChars, r):
			special = true
		}
	}
	return lower && upper && digit && special
}

func hasRun(pw string, length int) bool {
	var prev rune
	count := 0
	for i, r := range pw {
		if i > 0 && r == prev {
			count++
		} else {
			count = 1
		}
		if count >= length {
			return true
		}
		prev = r
	}
	return false
}

func turkishLower(s string) string { return strings.ToLowerSpecial(unicode.TurkishCase, s) }

func personalFragments(in PolicyInput) []string {
	var words []string
	words = append(words, strings.Fields(in.Name)...)
	words = append(words, strings.Fields(in.CompanyName)...)
	if local, _, ok := strings.Cut(in.Email, "@"); ok {
		words = append(words, strings.FieldsFunc(local, func(r rune) bool { return strings.ContainsRune("._-+", r) })...)
	}
	out := make([]string, 0, len(words))
	for _, w := range words {
		if utf8.RuneCountInString(w) >= minFragmentRunes {
			out = append(out, turkishLower(w))
		}
	}
	return out
}

func reused(h Hasher, in PolicyInput) bool {
	candidates := append([]string{}, in.History...)
	if in.CurrentHash != "" {
		candidates = append(candidates, in.CurrentHash)
	}
	for _, stored := range candidates {
		if ok, _ := h.Verify(stored, in.Password); ok {
			return true
		}
	}
	return false
}
