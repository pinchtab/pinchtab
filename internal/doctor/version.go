package doctor

import "github.com/pinchtab/pinchtab/internal/browserprobe"

// compareSemver delegates to the shared browserprobe implementation.
func compareSemver(a, b string) int { return browserprobe.CompareSemver(a, b) }

// extractVersionToken delegates to the shared browserprobe implementation.
func extractVersionToken(s string) string { return browserprobe.ExtractVersionToken(s) }
