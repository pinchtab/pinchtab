package report

import (
	"os"

	"github.com/pinchtab/pinchtab/internal/cli/termstyle"
)

var (
	colorTextPrimary = termstyle.Color("#e2e8f0")
	colorTextMuted   = termstyle.Color("#64748b")
	colorAccent      = termstyle.Color("#60a5fa")
	colorAccentLight = termstyle.Color("#93c5fd")
	colorSuccess     = termstyle.Color("#22c55e")
	colorWarning     = termstyle.Color("#fbbf24")
	colorDanger      = termstyle.Color("#ef4444")
)

var (
	headingStyle = termstyle.NewStyle().Foreground(colorAccent).Bold(true)
	commandStyle = termstyle.NewStyle().Foreground(colorAccentLight)
	mutedStyle   = termstyle.NewStyle().Foreground(colorTextMuted)
	successStyle = termstyle.NewStyle().Foreground(colorSuccess).Bold(true)
	warningStyle = termstyle.NewStyle().Foreground(colorWarning).Bold(true)
	errorStyle   = termstyle.NewStyle().Foreground(colorDanger).Bold(true)
	valueStyle   = termstyle.NewStyle().Foreground(colorTextPrimary)
)

func styleStdout(style termstyle.Style, text string) string {
	return termstyle.RenderToFile(os.Stdout, style, text)
}
