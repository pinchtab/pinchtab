package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/scroll"
)

func resetScrollFlags(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		for _, name := range []string{"dy", "dx"} {
			flag := scrollCmd.Flags().Lookup(name)
			if flag == nil {
				continue
			}
			_ = flag.Value.Set("0")
			flag.Changed = false
		}
	})
}

// A second positional used to be accepted and dropped, which is how a scroll ran on the
// WRONG tab and still reported OK: `scroll -- -300 --tab <id>` put --tab and its value in
// args[1:], and MinimumNArgs(1) was happy. Refusing the count is the whole guard — it needs
// no special case for --tab.
func TestScrollRefusesArgumentsItCannotHonour(t *testing.T) {
	resetScrollFlags(t)

	for _, tc := range []struct {
		name    string
		args    []string
		flags   map[string]string
		wantErr string
	}{
		{
			name:    "the swallowed tab",
			args:    []string{"-300", "--tab", "zzz-not-a-tab"},
			wantErr: "at most 1 positional",
		},
		{
			name:    "any stray second positional",
			args:    []string{"800", "junk"},
			wantErr: "at most 1 positional",
		},
		{
			name:    "nothing to scroll by",
			args:    nil,
			wantErr: "--dy",
		},
		{
			name:    "two spellings of one argument",
			args:    []string{"800"},
			flags:   map[string]string{"dy": "-300"},
			wantErr: "not both",
		},
		{
			name:    "an explicit zero delta",
			args:    nil,
			flags:   map[string]string{"dy": "0"},
			wantErr: "zero delta",
		},
		{
			name:    "zero on both axes",
			args:    nil,
			flags:   map[string]string{"dy": "0", "dx": "0"},
			wantErr: "zero delta",
		},
		{
			name:  "an explicit zero on ONE axis is a real scroll",
			args:  nil,
			flags: map[string]string{"dy": "0", "dx": "500"},
		},
		{
			// The same request as --dy 0, differently spelled. Keying the refusal to the
			// flag rather than to the delta let this one through to the default step.
			name:    "a zero positional",
			args:    []string{"0"},
			wantErr: "zero delta",
		},
		{
			name:    "a signed zero positional",
			args:    []string{"-0"},
			wantErr: "zero delta",
		},
		{
			name:  "a positional alone",
			args:  []string{"800"},
			flags: nil,
		},
		{
			name:  "a selector alone",
			args:  []string{"e12"},
			flags: nil,
		},
		{
			name:  "the pixel flag alone",
			args:  nil,
			flags: map[string]string{"dy": "-300"},
		},
		{
			name:  "the horizontal flag alone",
			args:  nil,
			flags: map[string]string{"dx": "-120"},
		},
	} {
		for name, value := range tc.flags {
			if err := scrollCmd.Flags().Set(name, value); err != nil {
				t.Fatal(err)
			}
		}

		err := scrollCmd.Args(scrollCmd, tc.args)

		switch {
		case tc.wantErr == "" && err != nil:
			t.Errorf("%s: scroll %v rejected with %v, want accepted", tc.name, tc.args, err)
		case tc.wantErr != "" && err == nil:
			t.Errorf("%s: scroll %v was accepted; anything it cannot honour must be refused rather than dropped", tc.name, tc.args)
		case tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr):
			t.Errorf("%s: error = %v, want it to mention %q", tc.name, err, tc.wantErr)
		}

		for name := range tc.flags {
			flag := scrollCmd.Flags().Lookup(name)
			_ = flag.Value.Set("0")
			flag.Changed = false
		}
	}
}

// The negative count is only reachable through a flag, so the flags have to exist: a help
// text promising --dy over a command that never registered it is the same defect one layer up.
func TestScrollRegistersThePixelFlags(t *testing.T) {
	for _, name := range []string{"dy", "dx"} {
		flag := scrollCmd.Flags().Lookup(name)
		if flag == nil {
			t.Fatalf("scroll has no --%s, so a negative pixel count has no spelling that parses", name)
		}
		if flag.Value.Type() != "int" {
			t.Errorf("--%s is %s, want int: a signed count must be accepted as a flag VALUE", name, flag.Value.Type())
		}
	}
}

// directionsEnumeratedAfter extracts the keyword list a line presents after marker, so the
// prose can keep its own separators and order while the SET is what gets compared.
func directionsEnumeratedAfter(text, marker string) ([]string, bool) {
	for line := range strings.SplitSeq(text, "\n") {
		_, list, ok := strings.Cut(line, marker)
		if !ok {
			continue
		}
		if cut := strings.Index(list, "("); cut >= 0 {
			list = list[:cut]
		}
		var found []string
		for _, token := range strings.FieldsFunc(list, func(r rune) bool {
			return r < 'a' || r > 'z'
		}) {
			if len(token) > 1 {
				found = append(found, token)
			}
		}
		sort.Strings(found)
		return found, true
	}
	return nil, false
}

// Every prose enumeration of the direction keywords is asserted against the code's own
// vocabulary, in BOTH directions: the agent-facing reference had invented "top" and
// "bottom", which fall through to CSS selectors and match nothing, while omitting "left"
// and "right", which work. A guard pinning one known-bad example would not have caught
// either half — only comparing the whole set does.
func TestDocumentedScrollDirectionsAreExactlyTheSupportedOnes(t *testing.T) {
	want := scroll.DirectionKeywords()

	commands, err := os.ReadFile(filepath.Join("..", "..", "skills", "pinchtab", "references", "commands.md"))
	if err != nil {
		t.Fatalf("cannot read the agent-facing reference, so this guard would not cover it: %v", err)
	}

	for _, site := range []struct {
		name   string
		text   string
		marker string
	}{
		{name: "scroll --help", text: scrollCmd.Long, marker: "Direction keyword:"},
		{name: "skills/pinchtab/references/commands.md", text: string(commands), marker: "named direction:"},
	} {
		got, ok := directionsEnumeratedAfter(site.text, site.marker)
		if !ok {
			t.Errorf("%s no longer enumerates the directions after %q, so drift there is now unguarded", site.name, site.marker)
			continue
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s documents directions %v, want exactly %v — a keyword it invents falls through to a CSS selector and matches nothing, and one it omits is invisible to the reader", site.name, got, want)
		}
	}
}
