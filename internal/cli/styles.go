package cli

import (
	"fmt"
	"os"

	"github.com/pinchtab/pinchtab/internal/cli/termstyle"
	"github.com/spf13/cobra"
)

var (
	ColorBorder      = termstyle.Color("#2b3345")
	ColorTextPrimary = termstyle.Color("#e2e8f0")
	ColorTextMuted   = termstyle.Color("#64748b")
	ColorAccent      = termstyle.Color("#60a5fa")
	ColorAccentLight = termstyle.Color("#93c5fd")
	ColorSuccess     = termstyle.Color("#22c55e")
	ColorWarning     = termstyle.Color("#fbbf24")
	ColorDanger      = termstyle.Color("#ef4444")
)

var (
	HeadingStyle = termstyle.NewStyle().Foreground(ColorAccent).Bold(true)
	CommandStyle = termstyle.NewStyle().Foreground(ColorAccentLight)
	MutedStyle   = termstyle.NewStyle().Foreground(ColorTextMuted)
	SuccessStyle = termstyle.NewStyle().Foreground(ColorSuccess).Bold(true)
	WarningStyle = termstyle.NewStyle().Foreground(ColorWarning).Bold(true)
	ErrorStyle   = termstyle.NewStyle().Foreground(ColorDanger).Bold(true)
	ValueStyle   = termstyle.NewStyle().Foreground(ColorTextPrimary)
)

func RenderStdout(style termstyle.Style, text string) string {
	return StyleStdout(style, text)
}

func RenderStderr(style termstyle.Style, text string) string {
	return StyleStderr(style, text)
}

func StyleStdout(style termstyle.Style, text string) string {
	return renderToWriter(os.Stdout, style, text)
}

func StyleStderr(style termstyle.Style, text string) string {
	return renderToWriter(os.Stderr, style, text)
}

func Fatal(format string, args ...any) {
	fmt.Fprintln(os.Stderr, StyleStderr(ErrorStyle, fmt.Sprintf(format, args...)))
	os.Exit(1)
}

func SetupUsage(root *cobra.Command) {
	// Custom template using the shared ANSI-ready styles.
	headerStyle := HeadingStyle.Render
	cmdStyle := CommandStyle.Render

	root.SetUsageTemplate(fmt.Sprintf(`%s:
{{if .HasParent}}  {{.UseLine}}
{{else}}  {{.CommandPath}} [command] [flags]
  {{.CommandPath}} server                # Starts the full server
{{end}}

%s:
{{range .Groups}}{{$group := .ID}}
  {{.Title}}:
{{range $.Commands}}{{if eq .GroupID $group}}{{if not .Hidden}}    %s  {{.Short}}
{{end}}{{end}}{{end}}{{end}}
{{if .HasAvailableLocalFlags}}
%s:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}
{{end}}
{{if .HasExample}}
%s:
{{.Example}}
{{end}}
`,
		headerStyle("Usage"),
		headerStyle("Commands"),
		cmdStyle("{{rpad .Name .NamePadding}}"),
		headerStyle("Flags"),
		headerStyle("Examples")))
}

func renderToWriter(w *os.File, style termstyle.Style, text string) string {
	return termstyle.RenderToFile(w, style, text)
}
