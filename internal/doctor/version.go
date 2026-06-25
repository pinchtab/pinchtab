package doctor

import "github.com/pinchtab/pinchtab/internal/browserprobe"

func compareSemver(a, b string) int { return browserprobe.CompareSemver(a, b) }

func extractVersionToken(s string) string { return browserprobe.ExtractVersionToken(s) }
