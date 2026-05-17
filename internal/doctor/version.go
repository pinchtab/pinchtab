package doctor

import "strings"

// compareSemver returns -1/0/1 comparing major.minor.patch of a and b; it is
// lenient about "v" prefixes, build/prerelease suffixes, and non-numeric noise.
func compareSemver(a, b string) int {
	ap := splitDoctorVersion(a)
	bp := splitDoctorVersion(b)
	for i := 0; i < 3; i++ {
		switch {
		case ap[i] < bp[i]:
			return -1
		case ap[i] > bp[i]:
			return 1
		}
	}
	return 0
}

func splitDoctorVersion(v string) [3]int {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := [3]int{}
	segs := strings.SplitN(v, ".", 3)
	for i, s := range segs {
		if i >= 3 {
			break
		}
		n := 0
		for _, c := range s {
			if c >= '0' && c <= '9' {
				n = n*10 + int(c-'0')
				continue
			}
			break
		}
		parts[i] = n
	}
	return parts
}

// extractVersionToken returns the first dotted numeric token in s, or "".
func extractVersionToken(s string) string {
	fields := strings.Fields(s)
	for _, f := range fields {
		f = strings.Trim(f, "()[],;")
		if hasDottedDigits(f) {
			return f
		}
	}
	return ""
}

func hasDottedDigits(s string) bool {
	sawDigit := false
	sawDot := false
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
			sawDigit = true
		case c == '.':
			sawDot = true
		default:
			return false
		}
	}
	return sawDigit && sawDot
}
