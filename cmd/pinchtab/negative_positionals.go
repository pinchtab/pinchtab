package main

import (
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// A leading-minus positional is parsed by pflag as a bundle of shorthand flags, so
// `pinchtab set geo -33.8 151.2` failed with "unknown shorthand flag: `3`" and the whole
// southern hemisphere was unreachable. The same one-line failure hit `scroll -300`,
// `mouse wheel -200` and `mouse move -5 -5`.
//
// The escape cobra offers is `--`, but it also stops flag parsing, so the caller has to
// write flags FIRST — `set geo --tab X -- -33.8 151.2` — which is undocumented and the
// reverse of the ordering every example teaches. This rewrites the caller's arguments
// into exactly that working order, once, for every command, rather than giving four
// commands signed flags and leaving the fifth to be forgotten.
//
// THE PROPERTY IT RESTS ON: no shorthand flag registered anywhere in this CLI is a digit,
// so a token whose first character after the minus is a digit or a decimal point is
// unambiguously a number and never a flag. negative_positionals_test.go asserts that over
// the whole command tree, because a future digit shorthand would silently make this
// rewrite wrong.
//
// It fires ONLY when such a token is present and would be read as a positional. Every
// other invocation is returned byte-identical, which is what keeps a rewrite this broad
// safe.
func rewriteNegativeNumberArgs(root *cobra.Command, args []string) []string {
	path, flags, positionals, ok := classifyArgs(root, args)
	if !ok || !holdsNegativeNumber(positionals) {
		return args
	}

	rewritten := make([]string, 0, len(args)+1)
	rewritten = append(rewritten, path...)
	rewritten = append(rewritten, flags...)
	rewritten = append(rewritten, "--")
	return append(rewritten, positionals...)
}

func subcommandNamed(cmd *cobra.Command, name string) *cobra.Command {
	for _, sub := range cmd.Commands() {
		if sub.Name() == name || sub.HasAlias(name) {
			return sub
		}
	}
	return nil
}

// classifyArgs sorts every token into the command path, the flags, and the positionals in
// ONE pass, descending as it goes — a flag may legitimately precede a subcommand
// (`pinchtab --json set geo …`), so the path and the flags have to be recognised together
// rather than in two phases.
//
// Tokens are classified the way pflag will classify them, so a negative number that is a
// flag's VALUE (`--dy -300`) stays with its flag and only a genuine positional moves. The
// first token that is neither a flag nor a subcommand ends the path: from there on every
// non-flag token is data, so a positional spelling some command's name is never mistaken
// for one.
//
// ok is false when the caller already wrote `--` — they have chosen the argument order
// themselves and nothing here should second-guess it.
func classifyArgs(root *cobra.Command, args []string) (path, flags, positionals []string, ok bool) {
	cmd := root
	pathOpen := true
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			return nil, nil, nil, false
		case strings.HasPrefix(arg, "-") && arg != "-" && !looksNumeric(arg):
			flags = append(flags, arg)
			if consumesNextToken(cmd, arg) && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		default:
			if sub := subcommandNamed(cmd, arg); pathOpen && sub != nil {
				path = append(path, arg)
				cmd = sub
				continue
			}
			pathOpen = false
			positionals = append(positionals, arg)
		}
	}
	return path, flags, positionals, true
}

// looksNumeric reports a token pflag would read as shorthand flags but a human wrote as a
// number. Only the leading minus matters: the digit-shorthand property above is what makes
// the first character decisive, so this does not need to parse the whole token — `-3abc`
// is not a number, but it is not a flag either, and letting it through as a positional
// gives a refusal that names it instead of one naming the character `3`.
func looksNumeric(arg string) bool {
	rest := strings.TrimPrefix(arg, "-")
	if rest == arg || rest == "" {
		return false
	}
	rest = strings.TrimPrefix(rest, ".")
	return rest != "" && rest[0] >= '0' && rest[0] <= '9'
}

// consumesNextToken reports whether a flag token takes its value from the FOLLOWING
// token, which is the only case where the next token must travel with it. A flag written
// --name=value or -nvalue carries its own, and a boolean carries none.
func consumesNextToken(cmd *cobra.Command, arg string) bool {
	if strings.HasPrefix(arg, "--") {
		name, attached := cutFlagValue(arg[2:])
		if attached {
			return false
		}
		return takesValue(findFlag(cmd, func(set *pflag.FlagSet) *pflag.Flag { return set.Lookup(name) }))
	}

	// A shorthand bundle: pflag walks the characters and gives the REST of the token to
	// the first one that wants a value, so only a value-taking flag in final position
	// reaches for the next token.
	shorthands := arg[1:]
	for i := 0; i < len(shorthands); i++ {
		c := shorthands[i : i+1]
		if c[0] >= 0x80 {
			// pflag only looks up single ASCII shorthands and panics on anything else, so a
			// non-ASCII byte is not a flag this walk can reason about.
			return false
		}
		if !takesValue(findFlag(cmd, func(set *pflag.FlagSet) *pflag.Flag { return set.ShorthandLookup(c) })) {
			continue
		}
		return i == len(shorthands)-1
	}
	return false
}

func findFlag(cmd *cobra.Command, lookup func(*pflag.FlagSet) *pflag.Flag) *pflag.Flag {
	if flag := lookup(cmd.Flags()); flag != nil {
		return flag
	}
	for owner := cmd; owner != nil; owner = owner.Parent() {
		if flag := lookup(owner.PersistentFlags()); flag != nil {
			return flag
		}
	}
	return nil
}

func takesValue(flag *pflag.Flag) bool {
	if flag != nil {
		return flag.NoOptDefVal == ""
	}
	// An unknown flag is about to be refused by cobra whatever we do here; assuming it
	// takes no value keeps the following token a positional, so the refusal names the
	// flag rather than an argument count.
	return false
}

func cutFlagValue(name string) (string, bool) {
	if before, _, ok := strings.Cut(name, "="); ok {
		return before, true
	}
	return name, false
}

func holdsNegativeNumber(args []string) bool {
	return slices.ContainsFunc(args, looksNumeric)
}
