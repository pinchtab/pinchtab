package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

// The four commands measured as broken at HEAD, each with the coordinate or delta that
// could not be expressed. Every one failed with "unknown shorthand flag" on the first
// digit of its own argument.
func TestNegativeArgumentsAreMovedBehindTheFlags(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "a southern latitude and a western longitude in one command",
			args: []string{"set", "geo", "-33.8", "-151.2", "--tab", "T"},
			want: []string{"set", "geo", "--tab", "T", "--", "-33.8", "-151.2"},
		},
		{
			name: "the house ordering, flag last",
			args: []string{"set", "geo", "51.5", "-0.12", "--tab", "T"},
			want: []string{"set", "geo", "--tab", "T", "--", "51.5", "-0.12"},
		},
		{
			name: "scroll up by pixels",
			args: []string{"scroll", "-300", "--tab", "T"},
			want: []string{"scroll", "--tab", "T", "--", "-300"},
		},
		{
			name: "a negative wheel delta",
			args: []string{"mouse", "wheel", "-200"},
			want: []string{"mouse", "wheel", "--", "-200"},
		},
		{
			name: "two negative viewport coordinates",
			args: []string{"mouse", "move", "-5", "-5"},
			want: []string{"mouse", "move", "--", "-5", "-5"},
		},
		{
			name: "a flag before the subcommand still parses as a flag",
			args: []string{"set", "geo", "-33.8", "151.2", "--json"},
			want: []string{"set", "geo", "--json", "--", "-33.8", "151.2"},
		},
		{
			name: "a persistent --server before the subcommand keeps its value and the path",
			args: []string{"--server", "http://127.0.0.1:19227", "set", "geo", "51.5", "-0.12"},
			want: []string{"set", "geo", "--server", "http://127.0.0.1:19227", "--", "51.5", "-0.12"},
		},
		{
			name: "a persistent --agent-id before the subcommand keeps its value and the path",
			args: []string{"--agent-id", "agent-7", "mouse", "move", "-5", "-5"},
			want: []string{"mouse", "move", "--agent-id", "agent-7", "--", "-5", "-5"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := rewriteNegativeNumberArgs(rootCmd, tc.args)
			if strings.Join(got, " ") != strings.Join(tc.want, " ") {
				t.Errorf("rewrite = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPersistentFlagValuesStayWithTheirFlagOnAnUnmergedTree(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "a persistent --server before the subcommand",
			args: []string{"--server", "http://127.0.0.1:19227", "move", "-5", "-5"},
			want: []string{"move", "--server", "http://127.0.0.1:19227", "--", "-5", "-5"},
		},
		{
			name: "a persistent --agent-id before the subcommand",
			args: []string{"--agent-id", "agent-7", "move", "-5", "-5"},
			want: []string{"move", "--agent-id", "agent-7", "--", "-5", "-5"},
		},
		{
			name: "a persistent shorthand before the subcommand",
			args: []string{"-p", "work", "move", "-5"},
			want: []string{"move", "-p", "work", "--", "-5"},
		},
		{
			name: "a persistent flag after the subcommand resolves through the ancestor",
			args: []string{"move", "-5", "--server", "http://127.0.0.1:19227"},
			want: []string{"move", "--server", "http://127.0.0.1:19227", "--", "-5"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := rewriteNegativeNumberArgs(unmergedPersistentFlagTree(), tc.args)
			if strings.Join(got, " ") != strings.Join(tc.want, " ") {
				t.Errorf("rewrite = %v, want %v", got, tc.want)
			}
		})
	}
}

func unmergedPersistentFlagTree() *cobra.Command {
	root := &cobra.Command{Use: "root"}
	root.PersistentFlags().String("server", "", "")
	root.PersistentFlags().String("agent-id", "", "")
	root.PersistentFlags().StringP("profile", "p", "", "")
	root.AddCommand(&cobra.Command{Use: "move", Run: func(*cobra.Command, []string) {}})
	return root
}

// The safety property that makes a rewrite this broad acceptable: an invocation with no
// negative-number positional comes back byte-identical, so nothing else in the CLI can
// change shape because of this.
func TestEveryOtherInvocationIsUntouched(t *testing.T) {
	for _, args := range [][]string{
		{"set", "geo", "51.5", "0.12", "--tab", "T"},
		{"set", "viewport", "800", "600"},
		{"scroll", "800"},
		{"scroll", "down"},
		{"scroll", "--dy", "-300", "--tab", "T"},
		{"mouse", "wheel", "--dy", "-200"},
		{"config", "get", "server.port"},
		{"config", "set", "browser.proxy.username", "bob"},
		{"screenshot", "-o", "shot.png"},
		{"text", "--tab", "T"},
		{"scroll", "--", "-300"},
		// PIN-136's trap, which must stay refused: after a hand-written `--` the tab flag is
		// a positional, so scroll sees three and says so. Rewriting this form would silently
		// turn it into a working command aimed at that tab, changing a landed refusal.
		{"scroll", "--", "-300", "--tab", "T"},
		{"set", "geo", "--tab", "T", "--", "51.5", "-0.12"},
		{"--help"},
		{},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			got := rewriteNegativeNumberArgs(rootCmd, args)
			if strings.Join(got, "\x00") != strings.Join(args, "\x00") {
				t.Errorf("rewrite changed an invocation it has no business touching:\n in  %v\n out %v", args, got)
			}
		})
	}
}

// A negative number that is a flag's VALUE must stay with its flag. Moving it would hand
// --dy nothing and turn the delta into a positional the command never asked for.
func TestAFlagValueThatLooksNegativeStaysWithItsFlag(t *testing.T) {
	got := rewriteNegativeNumberArgs(rootCmd, []string{"scroll", "--dy", "-300", "--tab", "T"})
	if strings.Join(got, " ") != "scroll --dy -300 --tab T" {
		t.Fatalf("rewrite = %v, want the input unchanged — -300 belongs to --dy", got)
	}
}

// The end of the claim: the rewritten form actually parses, on the real commands, through
// the real pflag parser. The original form is asserted to FAIL in the same test, so this
// cannot pass on a build where the defect was never there.
func TestTheRewrittenFormParsesWhereTheOriginalDoesNot(t *testing.T) {
	for _, tc := range []struct {
		path  []string
		args  []string
		want  []string
		flag  string
		value string
	}{
		{path: []string{"set", "geo"}, args: []string{"-33.8", "151.2", "--tab", "T"}, want: []string{"-33.8", "151.2"}, flag: "tab", value: "T"},
		{path: []string{"scroll"}, args: []string{"-300", "--tab", "T"}, want: []string{"-300"}, flag: "tab", value: "T"},
		{path: []string{"mouse", "wheel"}, args: []string{"-200"}, want: []string{"-200"}},
		{path: []string{"mouse", "move"}, args: []string{"-5", "-5"}, want: []string{"-5", "-5"}},
	} {
		t.Run(strings.Join(tc.path, " "), func(t *testing.T) {
			cmd := commandAt(t, tc.path...)

			if err := freshFlagSet(t, cmd).Parse(tc.args); err == nil {
				t.Fatalf("%v parsed as written, so this build never had the defect and the rewrite proves nothing", tc.args)
			}

			full := append(append([]string{}, tc.path...), tc.args...)
			rewritten := rewriteNegativeNumberArgs(rootCmd, full)
			flags := freshFlagSet(t, cmd)
			if err := flags.Parse(rewritten[len(tc.path):]); err != nil {
				t.Fatalf("the rewritten form still does not parse: %v (%v)", err, rewritten)
			}
			if strings.Join(flags.Args(), " ") != strings.Join(tc.want, " ") {
				t.Errorf("positionals = %v, want %v", flags.Args(), tc.want)
			}
			if tc.flag != "" {
				if got := flags.Lookup(tc.flag).Value.String(); got != tc.value {
					t.Errorf("--%s = %q, want %q — a flag after the positional must still reach the command", tc.flag, got, tc.value)
				}
			}
		})
	}
}

// The whole rewrite rests on a token starting -<digit> being unambiguously a number. A
// digit shorthand would make that false, silently, for every command at once — so the
// assumption is asserted over the tree rather than written down and hoped for.
func TestNoShorthandFlagIsADigit(t *testing.T) {
	checked := 0
	visitAllCommands(rootCmd, func(cmd *cobra.Command) {
		inspect := func(flag *pflag.Flag) {
			checked++
			if flag.Shorthand == "" {
				return
			}
			if flag.Shorthand[0] >= '0' && flag.Shorthand[0] <= '9' {
				t.Errorf("%s registers -%s as a shorthand for --%s; a digit shorthand makes a negative number ambiguous and breaks the rewrite in negative_positionals.go for EVERY command — pick a letter",
					cmd.CommandPath(), flag.Shorthand, flag.Name)
			}
		}
		cmd.Flags().VisitAll(inspect)
		cmd.PersistentFlags().VisitAll(inspect)
	})
	if checked == 0 {
		t.Fatal("no flag was inspected; this guard would pass vacuously")
	}
}

// The census the card corrected: the predicate is a positional whose value range includes
// negatives, not an argument-count shape. These are the four it found, and each must
// accept its negative without the caller writing `--`.
func TestTheFourMeasuredCommandsAllAcceptANegativePositional(t *testing.T) {
	for _, args := range [][]string{
		{"set", "geo", "-33.8", "151.2"},
		{"scroll", "-300"},
		{"mouse", "wheel", "-200"},
		{"mouse", "move", "-5", "-5"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			rewritten := rewriteNegativeNumberArgs(rootCmd, args)
			if !containsToken(rewritten, "--") {
				t.Fatalf("%v was left as written, so it still fails on its own first digit", args)
			}
		})
	}
}

func commandAt(t *testing.T, path ...string) *cobra.Command {
	t.Helper()
	cmd, _, err := rootCmd.Find(path)
	if err != nil || cmd.CommandPath() != "pinchtab "+strings.Join(path, " ") {
		t.Fatalf("cannot find %q in the command tree (got %q, err %v)", strings.Join(path, " "), cmd.CommandPath(), err)
	}
	return cmd
}

// freshFlagSet is a copy of the command's own flags, so parsing here cannot leave values
// behind on the shared command tree for the next test to read.
func freshFlagSet(t *testing.T, cmd *cobra.Command) *pflag.FlagSet {
	t.Helper()
	set := pflag.NewFlagSet(cmd.Name(), pflag.ContinueOnError)
	set.SetOutput(discardWriter{})
	copyFlag := func(flag *pflag.Flag) {
		clone := *flag
		value := stringValue(flag.Value.String())
		clone.Value = &value
		set.AddFlag(&clone)
	}
	cmd.Flags().VisitAll(copyFlag)
	cmd.InheritedFlags().VisitAll(func(flag *pflag.Flag) {
		if set.Lookup(flag.Name) == nil {
			copyFlag(flag)
		}
	})
	return set
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

type stringValue string

func (s *stringValue) String() string     { return string(*s) }
func (s *stringValue) Set(v string) error { *s = stringValue(v); return nil }
func (s *stringValue) Type() string       { return "string" }

func visitAllCommands(cmd *cobra.Command, visit func(*cobra.Command)) {
	visit(cmd)
	for _, sub := range cmd.Commands() {
		visitAllCommands(sub, visit)
	}
}

func containsToken(args []string, want string) bool {
	return slices.Contains(args, want)
}

// The doc sites an agent reads, plus every command's own help. This replaces the
// scroll-local ban that stood here before: that guard forbade documenting a negative
// positional because it could not parse, which the rewrite above makes false. The
// property worth guarding is the same one either way — a documented example must work —
// so it is now asserted by RUNNING each documented example through the rewrite and the
// real parser, rather than by banning a spelling.
var documentedExampleSites = []string{
	"skills/pinchtab/references/commands.md",
	"docs/reference/scroll.md",
}

func TestEveryDocumentedNegativeExampleParses(t *testing.T) {
	examples := documentedPinchtabExamples(t)
	if len(examples) == 0 {
		t.Fatal("no pinchtab example was found in any doc site or help text; this guard would pass vacuously")
	}

	negatives := 0
	for _, example := range examples {
		if !holdsNegativeNumber(example.args) {
			continue
		}
		negatives++
		t.Run(strings.Join(example.args, "_"), func(t *testing.T) {
			rewritten := rewriteNegativeNumberArgs(rootCmd, example.args)
			path, _, positionals, ok := classifyArgs(rootCmd, rewritten)
			if !ok {
				// The example wrote its own `--`; cobra handles it and nothing here applies.
				return
			}
			cmd := commandAt(t, path...)
			flags := freshFlagSet(t, cmd)
			if err := flags.Parse(rewritten[len(path):]); err != nil {
				t.Errorf("%s documents %q, which does not parse: %v", example.site, strings.Join(example.args, " "), err)
				return
			}
			if strings.Join(flags.Args(), " ") != strings.Join(positionals, " ") {
				t.Errorf("%s documents %q, whose positionals reach the command as %v rather than %v", example.site, strings.Join(example.args, " "), flags.Args(), positionals)
			}
		})
	}
	if negatives == 0 {
		t.Error("no documented example uses a negative number any more; the working spelling stopped being taught, which is how the last reader learned it was impossible")
	}
}

type documentedExample struct {
	site string
	args []string
}

// documentedPinchtabExamples collects every `pinchtab …` line from the agent-facing docs
// and from every command's own Long text, so a command that gains an example is covered
// without editing this list.
func documentedPinchtabExamples(t *testing.T) []documentedExample {
	t.Helper()

	texts := map[string]string{}
	for _, site := range documentedExampleSites {
		raw, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(site)))
		if err != nil {
			t.Fatalf("cannot read %s, so this guard would not cover the doc site it names: %v", site, err)
		}
		texts[site] = string(raw)
	}
	visitAllCommands(rootCmd, func(cmd *cobra.Command) {
		if cmd.Long != "" {
			texts[cmd.CommandPath()+" --help"] = cmd.Long
		}
	})

	var examples []documentedExample
	for site, text := range texts {
		for line := range strings.SplitSeq(text, "\n") {
			fields := strings.Fields(strings.TrimPrefix(strings.TrimSpace(line), "$ "))
			if len(fields) < 2 || fields[0] != "pinchtab" {
				continue
			}
			examples = append(examples, documentedExample{site: site, args: stripComment(fields[1:])})
		}
	}
	return examples
}

// stripComment drops the trailing `# what this does` every example in these docs carries.
func stripComment(fields []string) []string {
	for i, field := range fields {
		if strings.HasPrefix(field, "#") {
			return fields[:i]
		}
	}
	return fields
}

// The rewrite is only worth anything if Execute installs it. Nothing else in this file
// can see that wiring — every other test calls the function directly — so it is pinned
// where the call is made.
func TestExecuteInstallsTheRewrite(t *testing.T) {
	pkg := srccensus.Load(t, ".", cliSourceFileFloor)

	execute, ok := pkg.Func("Execute")
	if !ok {
		t.Fatalf("Execute is not declared anywhere in %s; this guard is pinned to a function that no longer exists — re-point it at whatever runs the CLI rather than deleting it", pkg.Dir())
	}
	for _, site := range pkg.Calls(t, "rewriteNegativeNumberArgs") {
		if pkg.Contains(execute, site) {
			return
		}
	}
	t.Errorf("%s does not call rewriteNegativeNumberArgs, so every negative positional fails on its own first digit again no matter what the tests above prove", execute.Name)
}

// Well under the real count, so ordinary growth does not trip it while a scan that lost
// the package still fails.
const cliSourceFileFloor = 40
