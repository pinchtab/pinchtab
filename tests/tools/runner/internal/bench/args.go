// Package main implements the benchmark runner.
//
// This file defines the command-line interface. Two flags, --dry-run and
// --index-file, allow exercising the plan/prompt assembly without needing
// network access or the default index.md location.
package bench

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// Lane is the benchmark lane (pinchtab or agent-browser).
type Lane string

const (
	LanePinchtab     Lane = "pinchtab"
	LaneAgentBrowser Lane = "agent-browser"
)

// Provider identifies which API the runner talks to.
type Provider string

const (
	ProviderAnthropic Provider = "anthropic"
	ProviderOpenAI    Provider = "openai"
	ProviderFake      Provider = "fake"
	ProviderUnset     Provider = ""
)

// Args is the resolved plan for a single invocation. It is intentionally a
// plain-data struct so tests can construct one without touching os.Args.
type Args struct {
	Lane            Lane
	Provider        Provider
	Model           string
	Groups          []int
	Profile         string
	MaxTokens       int
	Temperature     float64
	MaxTurns        int
	MaxIdleTurns    int
	TimeoutSeconds  int
	TurnDelayMs     int
	ReportFile      string
	SkipInit        bool
	NoPromptCaching bool
	Finalize        bool
	DryRun          bool
	Verbose         bool
	IndexFile       string
	MaxInputTokens  int
	MaxOutputTokens int
	TerseSummary    bool
}

// defaultArgs holds the resolved defaults for the benchmark loop. Values that
// also apply to BrowserBench live as shared constants in shared_flags.go so the
// two entrypoints can't drift; loop-only defaults stay inline here.
func defaultArgs() Args {
	return Args{
		MaxTokens:      defaultMaxTokens,
		Temperature:    defaultTemperature,
		MaxTurns:       300,
		MaxIdleTurns:   25,
		TimeoutSeconds: defaultTimeoutSeconds,
		TurnDelayMs:    1500,
	}
}

const usageText = `Usage:
  runner --lane pinchtab [options]     Run benchmark loop
  runner --lane agent-browser [options]
  runner step-end [options] <group> <step> <status> <answer> <verify-status> <notes>
  runner record-step [options] <group> <step> <status> [answer] [notes]
  runner verify-step [options] <group> <step> <status> [notes]

Subcommand options:
  --type baseline|pinchtab|agent-browser
  --report-file PATH

Benchmark loop options:
` + usageLineProvider +
	usageLineModel +
	`  --groups 0,1,2,3
  --profile common10
` + usageLineMaxTokens +
	usageLineTemperature +
	usageLineMaxTurns +
	`  --max-idle-turns N
` + usageLineTimeoutSeconds +
	usageLineTurnDelayMs +
	`  --report-file PATH
  --skip-init
  --no-prompt-caching
  --finalize
  --dry-run                 Print the resolved plan and exit 0 without network access
  --index-file PATH         Override path to tests/benchmark/index.md
  --max-input-tokens N      Stop when cumulative input tokens exceed N (exit code 4)
  --max-output-tokens N     Stop when cumulative output tokens exceed N (exit code 4)
  --terse-summary           Tell the agent to end with "done" (no prose summary); use for benchmark runs
  --verbose, -v             Show detailed progress with spinners
`

// ParseArgs walks argv manually (like the TS runner) so the flag surface and
// error messages stay byte-identical. The stdlib `flag` package would reorder
// output and reject `--groups 0,1,2` style values in some edge cases.
func ParseArgs(argv []string) (Args, error) {
	a := defaultArgs()

	next := func(i *int, name string) (string, error) {
		*i++
		if *i >= len(argv) {
			return "", fmt.Errorf("%s requires a value", name)
		}
		return argv[*i], nil
	}

	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		switch arg {
		case "--lane":
			v, err := next(&i, arg)
			if err != nil {
				return a, err
			}
			a.Lane = Lane(v)
		case "--provider":
			if err := parseStringFlag(next, &i, arg, func(v string) { a.Provider = Provider(v) }); err != nil {
				return a, err
			}
		case "--model":
			if err := parseStringFlag(next, &i, arg, func(v string) { a.Model = v }); err != nil {
				return a, err
			}
		case "--groups":
			v, err := next(&i, arg)
			if err != nil {
				return a, err
			}
			groups, perr := parseGroups(v)
			if perr != nil {
				return a, perr
			}
			a.Groups = groups
		case "--profile":
			if err := parseStringFlag(next, &i, arg, func(v string) { a.Profile = v }); err != nil {
				return a, err
			}
		case "--max-tokens":
			if err := parseIntFlag(next, &i, arg, func(n int) { a.MaxTokens = n }); err != nil {
				return a, err
			}
		case "--temperature":
			if err := parseFloatFlag(next, &i, arg, func(f float64) { a.Temperature = f }); err != nil {
				return a, err
			}
		case "--max-turns":
			if err := parseIntFlag(next, &i, arg, func(n int) { a.MaxTurns = n }); err != nil {
				return a, err
			}
		case "--max-idle-turns":
			if err := parseIntFlag(next, &i, arg, func(n int) { a.MaxIdleTurns = n }); err != nil {
				return a, err
			}
		case "--timeout-seconds":
			if err := parseIntFlag(next, &i, arg, func(n int) { a.TimeoutSeconds = n }); err != nil {
				return a, err
			}
		case "--turn-delay-ms":
			if err := parseIntFlag(next, &i, arg, func(n int) { a.TurnDelayMs = n }); err != nil {
				return a, err
			}
		case "--report-file":
			if err := parseStringFlag(next, &i, arg, func(v string) { a.ReportFile = v }); err != nil {
				return a, err
			}
		case "--skip-init":
			a.SkipInit = true
		case "--no-prompt-caching":
			a.NoPromptCaching = true
		case "--finalize":
			a.Finalize = true
		case "--terse-summary":
			a.TerseSummary = true
		case "--dry-run":
			a.DryRun = true
		case "--verbose", "-v":
			a.Verbose = true
		case "--index-file":
			if err := parseStringFlag(next, &i, arg, func(v string) { a.IndexFile = v }); err != nil {
				return a, err
			}
		case "--max-input-tokens":
			if err := parseIntFlag(next, &i, arg, func(n int) { a.MaxInputTokens = n }); err != nil {
				return a, err
			}
		case "--max-output-tokens":
			if err := parseIntFlag(next, &i, arg, func(n int) { a.MaxOutputTokens = n }); err != nil {
				return a, err
			}
		case "-h", "--help", "help":
			return a, errHelp
		default:
			return a, fmt.Errorf("unknown argument: %s", arg)
		}
	}

	if a.Lane != LanePinchtab && a.Lane != LaneAgentBrowser {
		return a, errors.New("--lane must be 'pinchtab' or 'agent-browser'")
	}

	return a, nil
}

// errHelp is a sentinel the caller uses to distinguish "user asked for help"
// (exit 0) from "bad flags" (exit 1). It matches the TS runner's usage(0) vs
// usage(1) behaviour.
var errHelp = errors.New("help requested")

// parseGroups splits "0,1,2,3" into sorted unique ints, matching the TS
// implementation's `.map(Number).filter(Number.isInteger)` semantics.
func parseGroups(raw string) ([]int, error) {
	seen := make(map[int]struct{})
	var out []int
	for _, piece := range strings.Split(raw, ",") {
		piece = strings.TrimSpace(piece)
		if piece == "" {
			continue
		}
		n, err := strconv.Atoi(piece)
		if err != nil {
			// TS filters silently; we do too, to preserve parity.
			continue
		}
		if _, dup := seen[n]; dup {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	sort.Ints(out)
	return out, nil
}

// WriteUsage prints the help block to the given writer. Kept as a helper so
// tests can capture output without redirecting os.Stderr.
func WriteUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, usageText)
}
