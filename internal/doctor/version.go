package doctor

import "github.com/pinchtab/pinchtab/internal/browserprobe"

func extractVersionToken(s string) string { return browserprobe.ExtractVersionToken(s) }
