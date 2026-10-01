// Package devtools holds repo-level invariants about this project's development
// tooling — properties of how the suite is invoked and reported, which belong to no
// product package. It is test-only on purpose: there is nothing here to import.
package devtools

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

const gotestsumBinary = "gotestsum"

const hideSummaryFlag = "--hide-summary="

const bannedSummarySection = "output"

func TestNoGotestsumInvocationHidesTheOutputSummary(t *testing.T) {
	root := repoRoot(t)

	var invocations, offenders int
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && (srccensus.ExcludedDir(path) || d.Name() == ".tools") {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || !mayHoldInvocations(path) {
			return nil
		}
		body, readErr := os.ReadFile(path) // #nosec G304 -- files walked from this repo's own tree.
		if readErr != nil {
			return readErr
		}
		if !scannableForInvocations(path, string(body)) {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		found, offences := scanGotestsumCommands(string(body))
		invocations += found
		for _, offence := range offences {
			offenders++
			t.Errorf("%s:%d hides the gotestsum output summary, so a failure there prints a bare test name with the cause nowhere in the log; the rule is one-sided — hiding other sections is fine, hiding output is not:\n\t%s",
				rel, offence.line, offence.command)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("cannot scan the repo, so this guard checks nothing: %v", err)
	}

	if invocations == 0 {
		t.Fatalf("found no gotestsum invocation anywhere under %s; a rename or a move has made this guard vacuous — re-point it at whatever runs the suite rather than deleting it", root)
	}
	if offenders == 0 && invocations < 2 {
		t.Errorf("found only %d gotestsum invocation; both the CI workflow and the test script are expected to run one, so the scan is probably missing a file type", invocations)
	}
}

// A command, not a line. The guard reads source text while gotestsum reads argv, and the
// shell sits between them: quotes are stripped before the tool sees a value, and a
// trailing backslash means the flag lives on a line that never mentions the binary. Both
// were live evasions of a per-line scan. Everything below therefore works on a LOGICAL
// command — comments dropped, continuations joined, quotes removed — so the ban asks a
// question about a command rather than about a line of text.
//
// WHAT THIS CANNOT SEE, stated because the previous version of this comment claimed a
// completeness it did not have: it described the spellings gotestsum ACCEPTS, which is not
// the same as what a source scan COVERS. A scan cannot follow indirection.
// --hide-summary=$SECTIONS, a value arriving from a workflow matrix input, or an
// invocation inside a composite action are all unreachable by any grep or AST over this
// repo. That is inherent to scanning source rather than argv, not a gap left unclosed.
// One narrower shape is also out: --hide-summary="skipped, output", where the space after
// the comma is part of a quoted value, reads as a value ending at the space because quotes
// are removed before the value is cut. Named rather than fixed — see the stop rule.
//
// STOP RULE. Three rounds have gone on this one guard — flag order, then quotes and
// continuations. The hazard is a person editing a CI flag, so redding on the spellings a
// person writes plus naming the residual is proportionate. The next evasion is met by
// accepting the residual, not by a fourth predicate.
type gotestsumOffence struct {
	line    int
	command string
}

type logicalCommand struct {
	line int
	text string
}

// scanGotestsumCommands returns how many gotestsum invocations the body contains and
// which of them hide the output summary. Both counts come from the same normalisation, so
// the invocation floor cannot pass while the ban silently scans nothing.
func scanGotestsumCommands(body string) (invocations int, offences []gotestsumOffence) {
	for _, command := range logicalCommands(body) {
		for _, segment := range shellSegments(command.text) {
			if !isGotestsumInvocation(segment) {
				continue
			}
			invocations++
			if hidesOutputSummary(segment) {
				offences = append(offences, gotestsumOffence{line: command.line, command: segment})
			}
		}
	}
	return invocations, offences
}

// shellSegments cuts a logical command at the separators that end one command and begin
// another — ; && || | & — and leaves separators inside quotes alone, because the value of a
// quoted mention is text rather than syntax.
//
// The ban is asked per SEGMENT, which is what keeps the echo/printf exclusion below from
// swallowing the invocation beside it: a mention governs the segment it introduces and
// nothing past the separator. Judging the whole command instead cost coverage the per-line
// scan had — `echo running; gotestsum --hide-summary=output` read as a mention and was
// neither banned nor counted.
func shellSegments(command string) []string {
	var segments []string
	var current strings.Builder
	quote := byte(0)
	flush := func() {
		if segment := strings.TrimSpace(current.String()); segment != "" {
			segments = append(segments, segment)
		}
		current.Reset()
	}

	for i := 0; i < len(command); i++ {
		c := command[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
			current.WriteByte(c)
		case c == '\'' || c == '"':
			quote = c
			current.WriteByte(c)
		case c == '&' && ((i > 0 && command[i-1] == '>') || (i+1 < len(command) && command[i+1] == '>')):
			// An & adjacent to a > on either side is part of a redirect (2>&1,
			// >&, &>, &>>), not a command separator — cutting there lands
			// between the binary and its flags, and neither half fires the ban.
			current.WriteByte(c)
		case c == ';' || c == '|' || c == '&':
			if (c == '|' || c == '&') && i+1 < len(command) && command[i+1] == c {
				i++
			}
			flush()
		default:
			current.WriteByte(c)
		}
	}
	flush()
	return segments
}

// logicalCommands joins trailing-backslash continuations into one command and reports the
// line the command STARTS on, so a failure points at something the reader can open.
//
// Comments are dropped BEFORE joining, and a dropped comment also ends any pending
// continuation. Joining first would splice a commented-out invocation into a live command;
// gluing across a comment would build a command that never existed.
func logicalCommands(body string) []logicalCommand {
	var commands []logicalCommand
	pending := ""
	startLine := 0
	flush := func() {
		if pending != "" {
			commands = append(commands, logicalCommand{line: startLine, text: strings.TrimSpace(pending)})
		}
		pending = ""
		startLine = 0
	}

	for i, raw := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(raw)
		if strings.HasPrefix(trimmed, "#") {
			flush()
			continue
		}
		if pending == "" {
			startLine = i + 1
		}
		if continued := strings.HasSuffix(trimmed, "\\"); continued {
			pending += strings.TrimSuffix(trimmed, "\\") + " "
			continue
		}
		pending += trimmed
		flush()
	}
	flush()
	return commands
}

// hidesOutputSummary parses --hide-summary as the comma-separated list gotestsum accepts,
// so "skipped,output" is caught as surely as "output". Quote characters are stripped first
// because the shell removes them before the tool sees the value: without that, the last
// field of --hide-summary="skipped,output" carries a trailing quote and the compare fails.
func hidesOutputSummary(command string) bool {
	rest := unquote(command)
	for {
		at := strings.Index(rest, hideSummaryFlag)
		if at < 0 {
			return false
		}
		rest = rest[at+len(hideSummaryFlag):]
		value := rest
		if end := strings.IndexAny(value, " \t"); end >= 0 {
			value = value[:end]
		}
		for section := range strings.SplitSeq(value, ",") {
			if strings.TrimSpace(section) == bannedSummarySection {
				return true
			}
		}
	}
}

func unquote(s string) string {
	return strings.NewReplacer(`"`, "", `'`, "").Replace(s)
}

// isGotestsumInvocation asks whether the SEGMENT runs the tool. A segment that merely
// mentions the flag — prose, or an echo explaining it — looks identical once quotes are
// stripped, so a mention is excluded explicitly. The exclusion is safe only because it is
// asked of a segment: within one there is no separator, so an echo at the front does govern
// everything after it.
func isGotestsumInvocation(command string) bool {
	normalised := unquote(command)
	// Case-insensitive: scripts/test.sh invokes the tool through "$GOTESTSUM_BIN", which a
	// lowercase-only match misses entirely.
	if !strings.Contains(strings.ToLower(normalised), gotestsumBinary) {
		return false
	}
	if !strings.Contains(normalised, "--format=") && !strings.Contains(normalised, hideSummaryFlag) {
		return false
	}
	return !printsRatherThanRuns(normalised)
}

func printsRatherThanRuns(command string) bool {
	for field := range strings.FieldsSeq(command) {
		switch field {
		case "echo", "printf":
			return true
		case gotestsumBinary:
			return false
		}
		if strings.Contains(strings.ToLower(field), gotestsumBinary) {
			return false
		}
	}
	return false
}

// scannableForInvocations accepts a file by SHAPE rather than by name: the shell and
// workflow extensions, plus anything carrying a shebang. The previous version special-cased
// one extensionless runner by base name, which is itself the evidence that a second one is
// writable — a Makefile-style runner or scripts/citest would have been invisible.
func scannableForInvocations(path, body string) bool {
	if hasScannableExtension(path) {
		return true
	}
	return strings.HasPrefix(body, "#!")
}

func hasScannableExtension(path string) bool {
	switch filepath.Ext(path) {
	case ".yml", ".yaml", ".sh":
		return true
	}
	return false
}

// mayHoldInvocations keeps the walk from reading every file in the tree: a shebang can only
// be found by reading, so extensionless files are read, and everything with an extension we
// do not scan is skipped unread.
func mayHoldInvocations(path string) bool {
	return hasScannableExtension(path) || filepath.Ext(path) == ""
}

// The walk stays hand-rolled because its subject is scripts and workflow files, which
// srccensus.Tree (a Go-source enumerator) cannot see — but the directory EXCLUSIONS,
// including the nested-checkout skip whose absence let a worktree's copy of the test
// script be counted as a module invocation, are inherited via srccensus.ExcludedDir
// rather than copied. .tools is a local extra: vendored tool checkouts hold no
// invocation of this repo's suite.
//
// repoRoot walks up to the directory holding go.mod rather than counting "../" hops, so
// moving this package does not silently point the scan at a subtree.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod found above the working directory; cannot locate the repo root")
		}
		dir = parent
	}
}

// TestTheGotestsumGuardReadsCommandsNotLines is the acceptance for the two evasions that
// ran correctly while the per-line scan stayed green — a quoted value and a flag on a
// backslash continuation — plus the shapes that must NOT red, which is the risk of asking
// the question at a coarser level. The already-caught forms are here as controls, not as
// evidence: they passed before this change too.
func TestTheGotestsumGuardReadsCommandsNotLines(t *testing.T) {
	for _, tc := range []struct {
		name      string
		offending bool
		wantLine  int
		body      string
	}{
		{"double-quoted value", true, 1, `gotestsum --format=pkgname --hide-summary="skipped,output" -- ./...`},
		{"single-quoted value", true, 1, `gotestsum --format=pkgname --hide-summary='output' -- ./...`},
		{"backslash continuation in a YAML run block", true, 6, `jobs:
  test:
    steps:
      - name: Test
        run: |
          gotestsum --format=pkgname \
            --hide-summary=output \
            -- -p 1 ./...`},
		{"continuation in a shell script", true, 2, `#!/usr/bin/env bash
"$GOTESTSUM_BIN" --format=pkgname \
  --hide-summary=output \
  --jsonfile "$UNIT_JSON" -- ./...`},
		{"bare output", true, 1, `gotestsum --format=pkgname --hide-summary=output -- ./...`},
		{"reordered list", true, 1, `gotestsum --format=pkgname --hide-summary=output,skipped -- ./...`},
		{"two flags on one command", true, 1, `gotestsum --format=pkgname --hide-summary=skipped --hide-summary=output -- ./...`},
		{"a legal invocation", false, 0, `gotestsum --format=pkgname --hide-summary=skipped -- ./...`},
		{"no hide-summary at all", false, 0, `gotestsum --format=pkgname -- -p 1 ./...`},
		{"commented-out invocation", false, 0, `# gotestsum --format=pkgname --hide-summary=output -- ./...`},
		{"commented-out invocation spanning a continuation", false, 0, `# gotestsum --format=pkgname \
#   --hide-summary=output \
#   -- ./...`},
		{"a comment ends a pending continuation", false, 0, `gotestsum --format=pkgname \
# --hide-summary=output
`},
		{"an echoed mention", false, 0, `echo "gotestsum --format=pkgname --hide-summary=output is banned"`},
		{"a printf mention", false, 0, `printf 'run gotestsum --hide-summary=output to hide it\n'`},
		{"a mention whose quoted text carries a pipe", false, 0, `echo "banned | gotestsum --hide-summary=output must never be added"`},
		{"a real invocation after an echoed mention", true, 1, `echo "running tests"; gotestsum --format=pkgname --hide-summary=output -- ./...`},
		{"a real invocation after &&", true, 1, `echo running && gotestsum --format=pkgname --hide-summary=output`},
		{"a real invocation piped into tee", true, 1, `gotestsum --format=pkgname --hide-summary=output -- ./... | tee unit.log`},
		{"a 2>&1 redirect between the binary and the flag", true, 1, `gotestsum --format=pkgname 2>&1 --hide-summary=output ./...`},
		{"an &> redirect between the binary and the flag", true, 1, `gotestsum --format=pkgname &>log --hide-summary=output ./...`},
		{"an &>> redirect between the binary and the flag", true, 1, `gotestsum --format=pkgname &>>log --hide-summary=output ./...`},
		{"a trailing 2>&1 piped into tee", true, 1, `gotestsum --format=pkgname --hide-summary=output -- ./... 2>&1 | tee unit.log`},
		{"a trailing file redirect with 2>&1", true, 1, `gotestsum --format=pkgname --hide-summary=output -- ./... >>unit.log 2>&1`},
		{"a command substitution in an argument", true, 1, `gotestsum --format=pkgname --hide-summary=output --jsonfile "$(mktemp -t unit)" -- ./...`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, offences := scanGotestsumCommands(tc.body)
			if tc.offending {
				if len(offences) != 1 {
					t.Fatalf("offences = %v, want exactly one: this form runs correctly and hides the failure cause", offences)
				}
				if offences[0].line != tc.wantLine {
					t.Errorf("reported line %d, want %d — the report must point at the line the invocation STARTS on, not an index into the joined text", offences[0].line, tc.wantLine)
				}
				return
			}
			if len(offences) != 0 {
				t.Fatalf("offences = %v, want none", offences)
			}
		})
	}
}

// The redirect correction must not stop splitting where splitting is right: a genuine
// background & — not adjacent to a > — still separates two commands.
func TestABackgroundAmpersandStillSeparates(t *testing.T) {
	segments := shellSegments(`gotestsum --format=pkgname & echo done`)
	if len(segments) != 2 {
		t.Fatalf("segments = %q, want 2 — a bare background & must keep separating", segments)
	}
	if segments[0] != "gotestsum --format=pkgname" || segments[1] != "echo done" {
		t.Fatalf("segments = %q, want the cut exactly at the background &", segments)
	}
}

// The invocation count feeds the vacuity floor, so a shape that stops being recognised as
// an invocation would let the ban scan nothing while the floor still passed.
func TestTheGotestsumGuardCountsBothRealInvocationShapes(t *testing.T) {
	for _, tc := range []struct {
		name string
		want int
		body string
	}{
		{"workflow run:", 1, `        run: gotestsum --format=pkgname -- -p 1 ./...`},
		{"script through an uppercase variable", 1, `  if ! "$GOTESTSUM_BIN" --format=pkgname --hide-summary=skipped -- ./...; then`},
		{"a mention is not an invocation", 0, `  echo "gotestsum --format=pkgname not found"`},
		{"resolution helper without flags", 0, `  if command -v gotestsum >/dev/null 2>&1; then`},
		{"an invocation following a mention still counts", 1, `  echo "running tests"; gotestsum --format=pkgname -- ./...`},
		{"a mention and an invocation are counted once each", 1, `  echo "gotestsum runs next" && gotestsum --format=pkgname -- ./...`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invocations, _ := scanGotestsumCommands(tc.body)
			if invocations != tc.want {
				t.Errorf("invocations = %d, want %d", invocations, tc.want)
			}
		})
	}
}

// The file-shape axis: a runner is recognised by carrying a shebang, not by being named
// "dev". A second extensionless runner used to be invisible.
func TestAnExtensionlessRunnerIsScannedByItsShebang(t *testing.T) {
	for _, tc := range []struct {
		name      string
		path      string
		body      string
		scannable bool
	}{
		{"the repo's own runner", "dev", "#!/usr/bin/env bash\n", true},
		{"a future extensionless runner", "scripts/citest", "#!/bin/sh\n", true},
		{"a Makefile-style runner with a shebang", "Makefile", "#!/usr/bin/make -f\n", true},
		{"an extensionless file with no shebang", "LICENSE", "MIT License\n", false},
		{"a shell script", "scripts/test.sh", "set -e\n", true},
		{"a workflow", ".github/workflows/go.yml", "name: go\n", true},
		{"documentation", "docs/guides/contributing.md", "# gotestsum --hide-summary=output\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := scannableForInvocations(tc.path, tc.body); got != tc.scannable {
				t.Errorf("scannableForInvocations(%q) = %v, want %v", tc.path, got, tc.scannable)
			}
		})
	}
}
