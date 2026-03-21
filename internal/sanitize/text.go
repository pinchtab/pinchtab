package sanitize

import (
	"regexp"
	"strings"
	"unicode"
)

const TruncationSuffix = "..."

var (
	ansiCSI = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
	unixAbs = regexp.MustCompile(`(^|[\s"'(=:])((?:/Users|/home|/var|/tmp|/private|/opt|/etc|/Volumes)(?:/[^\s"'():;<>{}\[\]]+)+)`)
	winAbs  = regexp.MustCompile(`(^|[\s"'(=:])([A-Za-z]:\\(?:[^\s"'():;<>{}\[\]]+\\?)+)`)
)

func TruncateUTF8Bytes(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	if maxBytes <= len(TruncationSuffix) {
		return TruncationSuffix[:maxBytes]
	}

	limit := maxBytes - len(TruncationSuffix)
	cut := 0
	for i := range s {
		if i > limit {
			break
		}
		cut = i
	}
	if cut == 0 && limit > 0 {
		return TruncationSuffix
	}
	return s[:cut] + TruncationSuffix
}

func StripANSI(s string) string {
	return ansiCSI.ReplaceAllString(s, "")
}

func StripControlChars(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	lastWasSpace := false
	for _, r := range s {
		if unicode.IsControl(r) {
			if !lastWasSpace {
				b.WriteByte(' ')
				lastWasSpace = true
			}
			continue
		}
		b.WriteRune(r)
		lastWasSpace = r == ' '
	}
	return strings.TrimSpace(b.String())
}

func RedactAbsolutePaths(s string) string {
	s = unixAbs.ReplaceAllString(s, `$1[path]`)
	s = winAbs.ReplaceAllString(s, `$1[path]`)
	return s
}

func CleanForLog(s string, maxBytes int) string {
	return TruncateUTF8Bytes(StripControlChars(StripANSI(s)), maxBytes)
}

func CleanError(s string, maxBytes int) string {
	return TruncateUTF8Bytes(RedactAbsolutePaths(StripControlChars(StripANSI(s))), maxBytes)
}
