package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/remedy"
	"github.com/pinchtab/pinchtab/internal/routes"
	"github.com/spf13/cobra"
)

// capabilitySettings are the config paths a capability refusal can name, derived
// from the route catalogue. Clipboard is appended because it gates endpoints
// without a catalogue entry, so nothing else would carry it here.
func capabilitySettings(t *testing.T) []string {
	t.Helper()

	settings := []string{"security.allowClipboard"}
	for capability := range routes.CapabilityEndpoints() {
		meta, ok := routes.Meta(capability)
		if !ok {
			t.Errorf("capability %q gates endpoints but has no metadata", capability)
			continue
		}
		settings = append(settings, meta.Setting)
	}
	if len(settings) < 2 {
		t.Fatal("no capability settings found; this test would prove nothing")
	}
	return settings
}

// A remedy an agent cannot execute is not a remedy. This resolves each command in EVERY
// declared remedy against the REAL command tree — the same rootCmd the binary runs — rather
// than eyeballing the strings, because the failure mode being closed is a remedy that reads
// plausibly and dead-ends when run.
//
// It walks remedy.Templates() rather than a list of producers, which is why it covers a
// producer added after it was written: declaring a remedy anywhere in the binary's import
// graph registers it, and this test links the whole binary. The guard started life covering
// the capability refusal alone, and every other producer of the field was free to mean
// something else; the population it walks is now the population that exists.
func TestEveryDeclaredRemedyResolvesInTheCLI(t *testing.T) {
	declared := remedy.Templates()
	// The floor is a vacuity check, not a census: the producers are enumerated by the
	// registry above. It must fail if linking stops pulling the producer packages in,
	// because an empty walk passes for the wrong reason.
	if len(declared) < 8 {
		t.Fatalf("only %d remedies are declared in the whole binary (%v); the walk has lost the producer packages and would pass vacuously", len(declared), declared)
	}
	for _, line := range declared {
		t.Run(line, func(t *testing.T) {
			assertRemedyRuns(t, line)
		})
	}
}

// assertRemedyRuns is the check itself, separated so a test can show it RED against a line
// the property forbids.
func assertRemedyRuns(t *testing.T, line string) {
	t.Helper()

	segments := remedy.Segments(line)
	if len(segments) == 0 {
		t.Fatalf("remedy %q is not a shell command line", line)
	}
	for _, words := range segments {
		if words[0] != "pinchtab" {
			t.Errorf("remedy command %v does not invoke pinchtab", words)
			continue
		}

		// The command path is the words before the first flag; everything after is
		// split by the RESOLVED command's own flag table rather than by the shape of
		// the token, because a flag's separate value does not start with a dash and
		// would otherwise be counted as a positional argument — which rejects every
		// remedy of the form `--flag <value>` however correct it is.
		path := words[1:]
		for i, word := range path {
			if strings.HasPrefix(word, "-") {
				path = path[:i]
				break
			}
		}
		found, rest, err := rootCmd.Find(path)
		if err != nil {
			t.Errorf("remedy command %v does not resolve: %v", words, err)
			continue
		}

		args := append([]string(nil), rest...)
		var flags []string
		tail := words[1+len(path):]
		for i := 0; i < len(tail); i++ {
			word := tail[i]
			if !strings.HasPrefix(word, "-") {
				args = append(args, word)
				continue
			}
			flags = append(flags, word)
			if i+1 < len(tail) && !strings.HasPrefix(tail[i+1], "-") && remedyFlagTakesAValue(found, word) {
				i++
			}
		}
		rest = args
		if !found.Runnable() || printsGroupHelp(found) {
			t.Errorf("remedy command %v resolves to %q, which is a command group and only prints help when run",
				words, found.CommandPath())
			continue
		}
		if found.Args != nil {
			if err := found.Args(found, rest); err != nil {
				t.Errorf("remedy command %v resolves to %q but its arguments %v are rejected: %v",
					words, found.CommandPath(), rest, err)
			}
		}
		// A flag the resolved command does not define is the same dead end as a missing
		// verb, and it is the likelier one: a remedy naming --wait-nav outlives a rename.
		// The name is cut at "=" the way remedyFlagTakesAValue cuts it, or the day a
		// remedy is written --flag=value this reports the flag as undefined.
		for _, flag := range flags {
			name, _, _ := strings.Cut(strings.TrimLeft(flag, "-"), "=")
			if found.Flags().Lookup(name) == nil && found.InheritedFlags().Lookup(name) == nil {
				t.Errorf("remedy command %v names flag %q, which %q does not define", words, flag, found.CommandPath())
			}
		}
	}
}

// remedyFlagTakesAValue mirrors cobra's own binding rule rather than a hand list of
// value-taking flags: a flag with no implicit value (everything but a bool) consumes
// the next word. An unknown flag consumes nothing and is reported by the check below.
func remedyFlagTakesAValue(cmd *cobra.Command, word string) bool {
	name, _, _ := strings.Cut(strings.TrimLeft(word, "-"), "=")
	flag := cmd.Flags().Lookup(name)
	if flag == nil {
		flag = cmd.InheritedFlags().Lookup(name)
	}
	return flag != nil && flag.NoOptDefVal == ""
}

// printsGroupHelp reports whether the command's only action is the help the unknown-subcommand
// guard installs on groups. Runnable() alone stopped answering this question when that guard
// landed: every group is now runnable, and a remedy naming one still does nothing.
func printsGroupHelp(cmd *cobra.Command) bool {
	if cmd.Run != nil || cmd.RunE == nil {
		return false
	}
	return reflect.ValueOf(cmd.RunE).Pointer() == reflect.ValueOf(printGroupHelp).Pointer()
}

// The guard has to be able to fail, and these are the shapes it must fail on: a verb that
// does not exist, a group that does nothing when run, and a flag the command never defined.
// Each is a remedy that reads plausibly and dead-ends when an agent runs it.
func TestTheRemedyGuardRedsOnACommandThatCannotRun(t *testing.T) {
	for _, line := range []string{
		"pinchtab unclog",
		"pinchtab config get security.allowedDomains && pinchtab unclog",
		"pinchtab session",
		"pinchtab back --wait-nav",
		// Splitting a flag's value off the positional list must not blind the
		// argument check: revoke takes exactly one id, and this line hands it two
		// past a flag that consumes neither.
		"pinchtab session revoke --json ses_one ses_two",
	} {
		fake := &testing.T{}
		assertRemedyRuns(fake, line)
		if !fake.Failed() {
			t.Errorf("the guard accepts %q, so it would accept a remedy nobody can run", line)
		}
	}
}

// The joined flag form, which no remedy uses today and which the undefined-flag check
// would have reported as undefined: the name has to be cut at "=" or every --flag=value
// reads as a flag the command never defined. This is a POSITIVE shape on purpose — the
// broken version reds a line that runs, and a reds-list row cannot see that, since an
// undefined flag is reported either way.
func TestTheRemedyGuardAcceptsADefinedFlagWrittenWithAnEquals(t *testing.T) {
	fake := &testing.T{}
	assertRemedyRuns(fake, "pinchtab session create --agent-id=a")
	if fake.Failed() {
		t.Error("the guard rejects `pinchtab session create --agent-id=a`, a line that runs; a remedy written in the joined form would be unpublishable")
	}
}

// The other half of executable: `config set <path> true` is only real if the
// config editor accepts that path. A setting present in the schema but missing
// from the editor's field table dead-ends every message that cites it.
func TestCapabilityRemedySettingsAreAcceptedByTheConfigEditor(t *testing.T) {
	for _, setting := range capabilitySettings(t) {
		fc := config.FileConfig{}
		if err := config.SetConfigValue(&fc, setting, "true"); err != nil {
			t.Errorf("the remedy tells the caller to run `pinchtab config set %s true`, which the editor rejects: %v", setting, err)
		}
	}
}

// The restart half must name a command that exists at that exact path. Finding it
// through the tree rather than asserting the literal means a renamed or moved
// verb reds here instead of shipping a remedy nobody can run.
func TestCapabilityRemedyRestartCommandExists(t *testing.T) {
	line, _ := httpx.DisabledEndpointDetails("security.allowCookies")["remedy"].(string)

	const restart = "pinchtab server restart"
	if !strings.Contains(line, restart) {
		t.Fatalf("remedy = %q, want it to name %q", line, restart)
	}
	found, _, err := rootCmd.Find([]string{"server", "restart"})
	if err != nil || found.CommandPath() != "pinchtab server restart" {
		t.Fatalf("`%s` does not resolve to itself (got %q, err %v); the remedy names a command that no longer exists",
			restart, found.CommandPath(), err)
	}
}
