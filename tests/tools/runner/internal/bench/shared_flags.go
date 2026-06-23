package bench

import (
	"fmt"
	"strconv"
)

// valueReader pulls the next argv token for a flag that takes a value. Both
// ParseArgs and parseBrowserBenchArgs build one of these over their argv slice;
// keeping the signature shared lets the parse helpers below live in one place.
type valueReader func(i *int, name string) (string, error)

// parseIntFlag reads the next argv token as an int and stores it via set,
// wrapping conversion errors as "<flag>: <err>" to match the historical
// messages in both entrypoints.
func parseIntFlag(next valueReader, i *int, flag string, set func(int)) error {
	v, err := next(i, flag)
	if err != nil {
		return err
	}
	n, perr := strconv.Atoi(v)
	if perr != nil {
		return fmt.Errorf("%s: %w", flag, perr)
	}
	set(n)
	return nil
}

// parseFloatFlag reads the next argv token as a float64 and stores it via set.
func parseFloatFlag(next valueReader, i *int, flag string, set func(float64)) error {
	v, err := next(i, flag)
	if err != nil {
		return err
	}
	f, perr := strconv.ParseFloat(v, 64)
	if perr != nil {
		return fmt.Errorf("%s: %w", flag, perr)
	}
	set(f)
	return nil
}

// parseStringFlag reads the next argv token and stores it verbatim via set.
func parseStringFlag(next valueReader, i *int, flag string, set func(string)) error {
	v, err := next(i, flag)
	if err != nil {
		return err
	}
	set(v)
	return nil
}

// Shared default values for flags that mean the same thing in both the generic
// benchmark loop and BrowserBench. Flags whose defaults differ between the two
// entrypoints (e.g. --max-turns, --turn-delay-ms) are intentionally left out so
// each entrypoint keeps its own value.
const (
	defaultMaxTokens      = 4096
	defaultTemperature    = 0
	defaultTimeoutSeconds = 120
)

// Shared usage lines for the flags whose help text is byte-identical across both
// entrypoints. Both usage texts compose these instead of repeating the literal
// strings, so a wording change lands in both --help outputs at once. The
// generic loop interleaves entrypoint-specific flags (--groups, --profile,
// --max-idle-turns) between these, so the constants are referenced per line
// rather than as one block.
//
// --max-input-tokens, --max-output-tokens and --verbose carry entrypoint-
// specific trailing descriptions in the generic loop and so are not shared here.
const (
	usageLineProvider       = "  --provider anthropic|openai|fake\n"
	usageLineModel          = "  --model MODEL\n"
	usageLineMaxTokens      = "  --max-tokens N\n"
	usageLineTemperature    = "  --temperature N\n"
	usageLineMaxTurns       = "  --max-turns N\n"
	usageLineTimeoutSeconds = "  --timeout-seconds N\n"
	usageLineTurnDelayMs    = "  --turn-delay-ms N\n"
)
