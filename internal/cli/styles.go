package cli

import (
	"fmt"
	"os"

	"github.com/pinchtab/pinchtab/internal/cli/termstyle"
	"github.com/spf13/cobra"
)

var (
	HeadingStyle = termstyle.Heading
	CommandStyle = termstyle.Command
	MutedStyle   = termstyle.Muted
	SuccessStyle = termstyle.Success
	WarningStyle = termstyle.Warning
	ErrorStyle   = termstyle.Error
	ValueStyle   = termstyle.Value
)

func StyleStdout(style termstyle.Style, text string) string {
	return renderToWriter(os.Stdout, style, text)
}

func StyleStderr(style termstyle.Style, text string) string {
	return renderToWriter(os.Stderr, style, text)
}

const (
	ExitRuntime = 1
	ExitUsage   = 2
)

func Fatal(format string, args ...any) {
	fmt.Fprintln(os.Stderr, StyleStderr(ErrorStyle, fmt.Sprintf(format, args...)))
	os.Exit(ExitRuntime)
}

func SetupUsage(root *cobra.Command) {
	headerStyle := HeadingStyle.Render
	cmdStyle := CommandStyle.Render

	root.SetUsageTemplate(fmt.Sprintf(`%s:
{{if .HasParent}}  {{.UseLine}}
{{else}}  {{.CommandPath}} [command] [flags]
  {{.CommandPath}} server                # Starts the full server
{{end}}

{{if .HasAvailableSubCommands}}%s:
{{range .Groups}}{{$group := .ID}}
  {{.Title}}:
{{range $.Commands}}{{if eq .GroupID $group}}{{if not .Hidden}}    %s  {{.Short}}
{{end}}{{end}}{{end}}{{end}}{{range $.Commands}}{{if and (eq .GroupID "") .IsAvailableCommand}}    %s  {{.Short}}
{{end}}{{end}}
{{end}}{{if .HasAvailableLocalFlags}}
%s:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}
{{end}}
{{if .HasAvailableInheritedFlags}}
%s:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}
{{end}}
{{if .HasExample}}
%s:
{{.Example}}
{{end}}
`,
		headerStyle("Usage"),
		headerStyle("Commands"),
		cmdStyle("{{rpad .Name .NamePadding}}"),
		cmdStyle("{{rpad .Name .NamePadding}}"),
		headerStyle("Flags"),
		headerStyle("Global Flags"),
		headerStyle("Examples")))
}

func renderToWriter(w *os.File, style termstyle.Style, text string) string {
	return termstyle.RenderToFile(w, style, text)
}
