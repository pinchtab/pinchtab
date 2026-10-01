// Package remedy owns details.remedy, the one field in an error response an agent may act
// on without reading English.
//
// THE PROPERTY. A remedy is ONE line a shell accepts as a single pinchtab invocation, or
// several joined with &&. Prose connectives ("then:", "or", a parenthetical tail), pipes,
// semicolons and redirections are not remedies: `pinchtab dialog accept|dismiss` is not two
// suggestions to a shell, it is a pipeline into a command named dismiss. A free slot is a
// <name> placeholder using the same angle-bracket convention the CLI's own help uses, and
// nothing else. Where the value is known at the point of production it is interpolated
// rather than left as a slot. When there is no single command the field is ABSENT: absence
// says truthfully that no command exists, which a client can handle and a sentence cannot.
//
// Prose belongs in details.hint, which is emitted beside remedy at every site.
//
// WHY AN OWNER. The field previously meant four different things across eight producers —
// executable line, command with a prose tail, bare verb, pure prose — while the CLI printed
// all of them into one "Remedy:" slot. A caller that learned the field is executable from
// one producer mis-executed the rest. Declare is the only way to build one, so a ninth
// producer cannot land in a ninth shape: the line is validated where it is written, and
// every declared line is walked by the guards (its commands resolve against the real
// command tree, and no site assembles the field by hand).
package remedy

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
)

// Remedy is a validated remedy line, or None.
type Remedy string

// None is the absence of a remedy: no single command fixes this refusal.
const None Remedy = ""

func (r Remedy) String() string { return string(r) }

// Empty reports whether there is no remedy to publish.
func (r Remedy) Empty() bool { return r == None }

// Template is a declared remedy line, which may carry <name> slots. Declaring it registers
// it, and that registry is what lets a guard walk the producers instead of listing them.
type Template struct{ line string }

var (
	mu        sync.Mutex
	templates []string
)

// Declare validates a remedy line and registers it. It panics on a line that violates the
// property, which is deliberate: every declaration in this repo is a constant, so the
// violation is a programming error that must surface at process start rather than reach a
// caller as guidance it cannot run.
func Declare(line string) Template {
	if err := Validate(line); err != nil {
		panic(fmt.Sprintf("remedy: %v", err))
	}
	mu.Lock()
	templates = append(templates, line)
	mu.Unlock()
	return Template{line: line}
}

// Remedy returns the declared line, slots intact. Use it where no value is known at the
// point of production: <ref> in the click remedy cannot be interpolated because the guard
// that produces it never receives the ref.
func (t Template) Remedy() Remedy { return Remedy(t.line) }

func (t Template) String() string { return t.line }

// Fill substitutes the leading slots, left to right, with values known at the point of
// production. A value that would break the property yields None rather than a line the
// caller cannot run — a URL with a quote in it has no remedy, and saying so is honest.
func (t Template) Fill(values ...string) Remedy {
	slots := placeholder.FindAllString(t.line, -1)
	if len(values) > len(slots) {
		panic(fmt.Sprintf("remedy: %d values for %d slots in %q", len(values), len(slots), t.line))
	}
	line := t.line
	for i, value := range values {
		line = strings.Replace(line, slots[i], value, 1)
	}
	if Validate(line) != nil {
		return None
	}
	return Remedy(line)
}

// Templates returns every declared line, for the guards that walk them.
func Templates() []string {
	mu.Lock()
	defer mu.Unlock()
	out := append([]string(nil), templates...)
	sort.Strings(out)
	return out
}

// Details is the guidance pair every refusal carries, built here so no site assembles the
// field by hand. An empty hint or remedy is omitted: the field's absence is the statement.
func Details(hint string, r Remedy) map[string]any {
	details := map[string]any{}
	if strings.TrimSpace(hint) != "" {
		details["hint"] = hint
	}
	if !r.Empty() {
		details["remedy"] = r.String()
	}
	return details
}

var placeholder = regexp.MustCompile(`<[a-z][a-z0-9-]*>`)

var forbidden = map[rune]string{
	'|':  "a pipe: `a|b` reads as a pipeline, not as alternatives",
	';':  "a command separator; join commands with &&",
	'`':  "a backquoted substitution; use $(...)",
	'#':  "a comment",
	'(':  "a parenthesis: a prose tail belongs in the hint",
	')':  "a parenthesis: a prose tail belongs in the hint",
	'\\': "an escape",
	'{':  "a brace expansion; a free slot is a <name> placeholder",
	'}':  "a brace expansion; a free slot is a <name> placeholder",
}

// Validate reports why line is not a remedy. It checks the shape a shell reads; whether the
// commands EXIST is the command-tree guard's question, and the two are deliberately
// separate so this package needs no dependency on the CLI.
func Validate(line string) error {
	if line == "" {
		return fmt.Errorf("remedy is empty; omit the field instead")
	}
	if strings.TrimSpace(line) != line {
		return fmt.Errorf("remedy %q has surrounding whitespace", line)
	}
	if strings.ContainsAny(line, "\n\r\t") {
		return fmt.Errorf("remedy %q is not one line", line)
	}
	if err := checkPlaceholders(line); err != nil {
		return err
	}
	segments, err := scan(line)
	if err != nil {
		return err
	}
	for _, words := range segments {
		if err := checkInvocation(line, words); err != nil {
			return err
		}
	}
	return nil
}

// Segments splits a remedy into the commands the shell would run, each already split into
// words the way a shell would: quotes and $(...) hold together. The command-tree guard reads
// it from here, because a second split written there would let the two disagree about what
// the shell sees, which is the whole subject of this package. An invalid line has no
// segments.
func Segments(line string) [][]string {
	if Validate(line) != nil {
		return nil
	}
	segments, _ := scan(line)
	return segments
}

func checkPlaceholders(line string) error {
	stripped := placeholder.ReplaceAllString(line, "")
	if idx := strings.IndexAny(stripped, "<>"); idx >= 0 {
		return fmt.Errorf("remedy %q has a free slot that is not a <name> placeholder", line)
	}
	return nil
}

func checkInvocation(line string, words []string) error {
	if len(words) < 2 {
		return fmt.Errorf("remedy %q has a command that is not a pinchtab invocation: %v", line, words)
	}
	if words[0] != "pinchtab" {
		return fmt.Errorf("remedy %q starts a command with %q rather than pinchtab, so it is prose", line, words[0])
	}
	if slices.Contains(words[1:], "pinchtab") {
		return fmt.Errorf("remedy %q runs two commands in one segment, so they are joined by prose rather than &&", line)
	}
	return nil
}

// scan splits line into &&-joined segments of shell words, rejecting anything that would
// give the line a meaning other than "run these commands in order".
func scan(line string) ([][]string, error) {
	var (
		segments [][]string
		words    []string
		word     strings.Builder
		quoted   bool
		subst    int
	)
	flushWord := func() {
		if word.Len() > 0 {
			words = append(words, word.String())
			word.Reset()
		}
	}
	flushSegment := func() {
		flushWord()
		segments = append(segments, words)
		words = nil
	}

	runes := []rune(line)
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		switch {
		case quoted:
			if c == '"' {
				quoted = false
			}
			word.WriteRune(c)
		case subst > 0:
			if c == '(' {
				subst++
			}
			if c == ')' {
				subst--
			}
			word.WriteRune(c)
		case c == '"':
			quoted = true
			word.WriteRune(c)
		case c == '$' && i+1 < len(runes) && runes[i+1] == '(':
			subst = 1
			word.WriteString("$(")
			i++
		case c == ' ':
			flushWord()
		case c == '&':
			if i+1 >= len(runes) || runes[i+1] != '&' {
				return nil, fmt.Errorf("remedy %q backgrounds a command with a bare &", line)
			}
			flushSegment()
			i++
		default:
			if reason, bad := forbidden[c]; bad {
				return nil, fmt.Errorf("remedy %q contains %s", line, reason)
			}
			word.WriteRune(c)
		}
	}
	if quoted {
		return nil, fmt.Errorf("remedy %q has an unclosed quote", line)
	}
	if subst > 0 {
		return nil, fmt.Errorf("remedy %q has an unclosed $( substitution", line)
	}
	flushSegment()
	return segments, nil
}
