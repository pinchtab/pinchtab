package report

import (
	"os"

	"github.com/pinchtab/pinchtab/internal/cli/termstyle"
)

var (
	headingStyle = termstyle.Heading
	commandStyle = termstyle.Command
	mutedStyle   = termstyle.Muted
	successStyle = termstyle.Success
	warningStyle = termstyle.Warning
	valueStyle   = termstyle.Value
)

func styleStdout(style termstyle.Style, text string) string {
	return termstyle.RenderToFile(os.Stdout, style, text)
}
