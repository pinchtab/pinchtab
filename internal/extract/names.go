package extract

import (
	"strings"
	"unicode"
)

func headerKey(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '_' || r == '-' || unicode.IsSpace(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, s)
}

func splitCaseWords(name string) string {
	runes := []rune(name)
	var b strings.Builder
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) && startsCaseWord(runes, i) {
			b.WriteRune(' ')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

func startsCaseWord(runes []rune, i int) bool {
	prev := runes[i-1]
	if unicode.IsLower(prev) || unicode.IsDigit(prev) {
		return true
	}
	return unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1])
}
