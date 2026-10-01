package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
	"github.com/spf13/cobra"
)

// TestMain installs the guard the way Execute does, so every test in this package drives the
// command tree production actually ships rather than the pre-guard one.
func TestMain(m *testing.M) {
	installUnknownSubcommandGuard(rootCmd)
	os.Exit(m.Run())
}

// operandNotAVerb records the parents whose first argument is DATA, not a subcommand, with
// what that data is. They opt out by declaring their own Args — this table is the reason,
// and it fails both ways: a parent that stops taking an operand must leave, and a parent
// that starts accepting an unknown verb must be added deliberately.
var operandNotAVerb = map[string]string{
	"pinchtab tab":     "the argument is a tab ID to focus, so an unknown token is a 404 from the server rather than a typo'd verb",
	"pinchtab network": "the argument is a URL filter for the network log",
}

// A typo'd verb must not read as success: these are state-changing commands that live in
// setup and teardown scripts, so exit 0 means `set -e` does not trip and the state the
// script believed it reset was never reset. The census walks the command TREE rather than a
// list of group names, because a hand-written list is how eleven groups came to be missing
// this in the first place.
func TestEveryCommandGroupRejectsAnUnknownSubcommand(t *testing.T) {
	groups := commandGroups(rootCmd)
	if len(groups) < 15 {
		t.Fatalf("found only %d command groups, so this census is not walking the command tree", len(groups))
	}

	for _, group := range groups {
		path := group.CommandPath()
		// A nil validator is cobra's default, which accepts anything below the root — the very
		// state this guard exists to leave behind, so it must read as acceptance, not a panic.
		var err error
		if group.Args != nil {
			err = group.Args(group, []string{"zzz-not-a-subcommand"})
		}

		if reason, exempt := operandNotAVerb[path]; exempt {
			if err != nil {
				t.Errorf("%s rejected its operand; it takes one because %s", path, reason)
			}
			continue
		}

		if err == nil {
			t.Errorf("%s accepts an unknown subcommand, so a typo there exits 0 and a script reads it as success", path)
			continue
		}
		if got := commandExitCode(err); got != unknownSubcommandExitCode {
			t.Errorf("%s unknown-subcommand exit code = %d, want %d everywhere so callers can branch on it", path, got, unknownSubcommandExitCode)
		}
		if !strings.Contains(err.Error(), "zzz-not-a-subcommand") {
			t.Errorf("%s refusal = %q, want it to name the token that was not understood", path, err)
		}
		for _, name := range subcommandNames(group) {
			if !strings.Contains(err.Error(), name) {
				t.Errorf("%s refusal = %q, want it to name the valid subcommand %q", path, err, name)
			}
		}
		// The argument check is only reached on a runnable command: cobra answers "print the
		// help" for an unrunnable one before validating anything, which is why setting Args
		// alone left the groups with no action of their own at exit 0.
		if !group.Runnable() {
			t.Errorf("%s is not runnable, so cobra never reaches its argument check", path)
		}
	}

	for path := range operandNotAVerb {
		if findGroup(groups, path) == nil {
			t.Errorf("%s is recorded as taking an operand but is no longer a group; drop the entry", path)
		}
	}
}

// The guard must not shadow the verbs it protects: every registered subcommand still has to
// resolve to itself rather than to its parent's refusal.
func TestEveryValidSubcommandStillResolves(t *testing.T) {
	checked := 0
	for _, group := range commandGroups(rootCmd) {
		for _, sub := range group.Commands() {
			if !sub.IsAvailableCommand() {
				continue
			}
			path := append(commandPathArgs(group), sub.Name())
			found, _, err := rootCmd.Find(path)
			if err != nil {
				t.Errorf("Find(%v) error = %v", path, err)
				continue
			}
			if found != sub {
				t.Errorf("Find(%v) resolved to %s, want the subcommand itself", path, found.CommandPath())
			}
			checked++
		}
	}
	if checked < 40 {
		t.Fatalf("only checked %d subcommands, so this absence assertion is not covering the tree", checked)
	}
}

// Zero arguments is not a typo: a group invoked bare still prints its help and exits 0, the
// behaviour a human relies on to discover the verbs.
func TestABareGroupStillPrintsItsHelpAndSucceeds(t *testing.T) {
	out, err := runRootArgs(t, "cache")
	if err != nil {
		t.Fatalf("`pinchtab cache` error = %v, want the group help and exit 0", err)
	}
	if !strings.Contains(out, "clear") || !strings.Contains(out, "status") {
		t.Errorf("`pinchtab cache` output = %q, want the group help listing its subcommands", out)
	}
}

// One exit code for the whole CLI, measured at all three places that produce it: cobra's
// top-level error, a group refusal, and daemon's hand-rolled dispatch, which used to answer
// 2 and is the one this card changed.
func TestUnknownSubcommandExitsWithOneCodeEverywhere(t *testing.T) {
	if _, err := runRootArgs(t, "zzz-not-a-command"); err == nil {
		t.Fatal("a top-level unknown command must be an error")
	} else if got := commandExitCode(err); got != unknownSubcommandExitCode {
		t.Errorf("top-level unknown command exit code = %d, want %d", got, unknownSubcommandExitCode)
	}

	if _, err := runRootArgs(t, "cache", "clera"); err == nil {
		t.Fatal("a group's unknown subcommand must be an error")
	} else if got := commandExitCode(err); got != unknownSubcommandExitCode {
		t.Errorf("group unknown subcommand exit code = %d, want %d", got, unknownSubcommandExitCode)
	}

	if got := dispatchDaemonCommand("zzz-not-a-subcommand", false); got != unknownSubcommandExitCode {
		t.Errorf("daemon unknown subcommand exit code = %d, want %d", got, unknownSubcommandExitCode)
	}
}

// A refusal is two lines, the shape daemon already had: the token and the valid verbs. Cobra
// adds the parent's whole usage block and prints the error a second time unless the command
// silences both, so a refusal without those flags names the typo three times over.
func TestARefusalRendersOnceWithoutTheUsageBlock(t *testing.T) {
	for _, args := range [][]string{
		{"cache", "zzz-clera"},
		{"config", "zzz-st", "server.port", "18899"},
		{"mcp", "zzz-bogus"},
	} {
		invocation := "pinchtab " + strings.Join(args, " ")
		rendered := renderedRefusal(t, args...)

		if got := strings.Count(rendered, args[1]); got != 1 {
			t.Errorf("`%s` named the token %d times, want once:\n%s", invocation, got, rendered)
		}
		if strings.Contains(rendered, "Usage:") {
			t.Errorf("`%s` printed the usage block, so the valid verbs appear twice over:\n%s", invocation, rendered)
		}
	}
}

// The guard lives at Execute because the command tree is only complete there. A test that
// installs it itself proves the walk works, not that the binary runs it.
func TestExecuteInstallsTheGuard(t *testing.T) {
	pkg := srccensus.Load(t, ".", 40)
	body := pkg.Source(pkg.MustFunc(t, "Execute"))
	install := strings.Index(body, "installUnknownSubcommandGuard(rootCmd)")
	// Prefix match: the call has been both Execute() and ExecuteC(), and a
	// rename must not read as an ordering failure.
	execute := strings.Index(body, "rootCmd.Execute")
	if install < 0 {
		t.Fatal("Execute no longer installs the unknown-subcommand guard, so every group is back to exit 0")
	}
	if execute < 0 {
		t.Fatal("Execute no longer runs the command tree through rootCmd.Execute/ExecuteC; re-point this ordering guard at whatever runs it rather than deleting it")
	}
	if execute < install {
		t.Error("the guard is installed after the command tree runs, which is too late to matter")
	}
}

func commandGroups(root *cobra.Command) []*cobra.Command {
	var groups []*cobra.Command
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd.HasSubCommands() && cmd.HasParent() {
			groups = append(groups, cmd)
		}
		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}
	walk(root)
	return groups
}

func findGroup(groups []*cobra.Command, path string) *cobra.Command {
	for _, group := range groups {
		if group.CommandPath() == path {
			return group
		}
	}
	return nil
}

// commandPathArgs is the argv that reaches a command, which is its command path without the
// binary name — Find wants the arguments, not the display string.
func commandPathArgs(cmd *cobra.Command) []string {
	return strings.Fields(cmd.CommandPath())[1:]
}

func runRootArgs(t *testing.T, args ...string) (string, error) {
	t.Helper()

	var out bytes.Buffer
	err := runRoot(t, &out, io.Discard, args...)
	return out.String(), err
}

func runRoot(t *testing.T, out, errOut io.Writer, args ...string) error {
	t.Helper()

	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})
	rootCmd.SetArgs(args)
	rootCmd.SetOut(out)
	rootCmd.SetErr(errOut)

	return rootCmd.Execute()
}

// renderedRefusal is everything a user sees on a refusal: what cobra printed plus the line
// Execute prints itself. Asserting on the error VALUE instead is what let the refusal grow to
// eighteen lines unnoticed.
func renderedRefusal(t *testing.T, args ...string) string {
	t.Helper()

	var stream bytes.Buffer
	err := runRoot(t, &stream, &stream, args...)
	if err == nil {
		t.Fatalf("`pinchtab %s` succeeded, want a refusal", strings.Join(args, " "))
	}
	return stream.String() + fmt.Sprintln(err)
}

// operandTakingLeaves records the leaves whose first argument is DATA, with what that data
// is. They opt out by declaring a validator that permits a positional — this table is the
// reason, and like operandNotAVerb it fails both ways: a leaf that stops taking an operand
// must leave it, and a leaf that starts accepting one must be added deliberately.
//
// Only the ones a reader would question are listed. The census below covers every leaf; this
// table is the record for the handful whose Use string alone does not settle it.
var operandTakingLeaves = map[string]string{
	"pinchtab daemon": "the argument is the lifecycle verb (status, install, start…), read from args[0]",
}

// Cobra defaults a leaf with no Args to ArbitraryArgs: it accepts any positional and hands it
// to Run, which drops it. `pinchtab screenshot shot.png` therefore exited 0 having written an
// auto-named file elsewhere, which an agent cannot detect. Walks the command TREE, never a
// list of names — a hand-written list is how eleven groups came to be missing the sibling fix.
func TestEveryLeafRefusesAStrayArgument(t *testing.T) {
	installUnknownSubcommandGuard(rootCmd)

	leaves := commandLeaves(rootCmd)
	if len(leaves) < 100 {
		t.Fatalf("found only %d leaf commands, so this census is not walking the command tree", len(leaves))
	}

	refusing := 0
	for _, leaf := range leaves {
		path := leaf.CommandPath()
		if leaf.Args == nil {
			t.Errorf("%s has no argument validator, so cobra accepts any positional and Run drops it", path)
			continue
		}
		takesOperand := leaf.Args(leaf, []string{"zzz-not-an-argument"}) == nil

		if reason, exempt := operandTakingLeaves[path]; exempt {
			if !takesOperand {
				t.Errorf("%s refuses a positional, but it is recorded as taking one because %s", path, reason)
			}
			continue
		}
		if takesOperand {
			continue
		}
		refusing++
	}
	if refusing < 50 {
		t.Fatalf("only %d leaves refuse a stray argument; the sweep covered less than the tree it was measured against", refusing)
	}
	t.Logf("leaf census: %d leaves, %d refuse a stray argument, %d take an operand", len(leaves), refusing, len(leaves)-refusing)
}

// The refusal a leaf gives has the same two-line shape as a group's: the token and what to do.
// Asserted on the RENDERED output, because every assertion on the error VALUE is blind to the
// usage block and to cobra printing the message a second time — which is how `version bogus`
// grew to fourteen lines unnoticed.
func TestALeafRefusalRendersOnceWithoutTheUsageBlock(t *testing.T) {
	for _, tc := range []struct {
		args  []string
		wants []string
	}{
		// The card's headline case: the stray token IS the path, so the remedy names the flag.
		{args: []string{"screenshot", "shot.png"}, wants: []string{`unexpected argument "shot.png"`, "-o shot.png"}},
		{args: []string{"pdf", "out.pdf"}, wants: []string{"-o out.pdf"}},
		// A leaf with no output flag is pointed at its own help rather than a guessed flag.
		{args: []string{"title", "zzz-stray"}, wants: []string{`unexpected argument "zzz-stray"`, `"pinchtab title --help"`}},
		// Already declared NoArgs before this card, and rendered fourteen lines because it
		// lacked the silence flags: the guard covers it too.
		{args: []string{"version", "zzz-stray"}, wants: []string{"zzz-stray"}},
		// The one leaf that takes an operand still refuses a SECOND one.
		{args: []string{"daemon", "status", "zzz-stray"}, wants: []string{`unexpected argument "zzz-stray"`}},
	} {
		invocation := "pinchtab " + strings.Join(tc.args, " ")
		rendered := renderedRefusal(t, tc.args...)

		lines := strings.Split(strings.TrimSpace(rendered), "\n")
		// The message itself must appear once. Counting the TOKEN instead would be wrong
		// here: a remedy that says `did you mean -o shot.png?` names it a second time on
		// purpose, and that is the sentence's whole value.
		if got := strings.Count(rendered, lines[0]); got != 1 {
			t.Errorf("`%s` printed its first line %d times, want once:\n%s", invocation, got, rendered)
		}
		if strings.Contains(rendered, "Usage:") {
			t.Errorf("`%s` printed the usage block:\n%s", invocation, rendered)
		}
		if len(lines) > 2 {
			t.Errorf("`%s` rendered %d lines, want at most the token and what to do:\n%s", invocation, len(lines), rendered)
		}
		for _, want := range tc.wants {
			if !strings.Contains(rendered, want) {
				t.Errorf("`%s` does not say %q:\n%s", invocation, want, rendered)
			}
		}
	}
}

// A leaf that takes an operand must be left alone entirely — the guard classifies by asking
// the validator, so a command whose Args permits a positional keeps accepting it.
func TestOperandTakingLeavesStillAcceptTheirArgument(t *testing.T) {
	installUnknownSubcommandGuard(rootCmd)

	for _, tc := range []struct{ path, arg string }{
		{path: "pinchtab nav", arg: "https://example.com"},
		{path: "pinchtab click", arg: "e5"},
		{path: "pinchtab close", arg: "tab-1"},
		{path: "pinchtab daemon", arg: "status"},
	} {
		leaf := findLeaf(rootCmd, tc.path)
		if leaf == nil {
			t.Errorf("%s is no longer a leaf command; re-point this row rather than deleting it", tc.path)
			continue
		}
		if err := leaf.Args(leaf, []string{tc.arg}); err != nil {
			t.Errorf("%s refused its own argument %q: %v", tc.path, tc.arg, err)
		}
	}
}

func commandLeaves(root *cobra.Command) []*cobra.Command {
	var leaves []*cobra.Command
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		if !cmd.HasSubCommands() && cmd.HasParent() {
			leaves = append(leaves, cmd)
		}
		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}
	walk(root)
	return leaves
}

func findLeaf(root *cobra.Command, path string) *cobra.Command {
	for _, leaf := range commandLeaves(root) {
		if leaf.CommandPath() == path {
			return leaf
		}
	}
	return nil
}
