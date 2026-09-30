package apiclient

import (
	"fmt"
	"os"
)

// SuggestNextAction provides helpful suggestions based on the current command and state
func SuggestNextAction(cmd string, result map[string]any) {
	switch cmd {
	case "nav", "navigate":
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Next steps")
		fmt.Fprintln(os.Stderr, "  pinchtab snap # See page structure")
		fmt.Fprintln(os.Stderr, "  pinchtab screenshot # Capture visual")
		fmt.Fprintln(os.Stderr, "  pinchtab click <ref> # Click an element")
		fmt.Fprintln(os.Stderr, "  pinchtab pdf --tab <id> -o output.pdf # Save tab as PDF")

	case "snap", "snapshot":
		refs := extractRefs(result)
		if len(refs) > 0 {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintf(os.Stderr, "Found %d interactive elements\n", len(refs))
			for i, ref := range refs[:min(3, len(refs))] {
				fmt.Fprintf(os.Stderr, "  pinchtab click %s # %s\n", ref.id, ref.desc)
				if i >= 2 {
					break
				}
			}
			if len(refs) > 3 {
				fmt.Fprintf(os.Stderr, "  ... and %d more\n", len(refs)-3)
			}
		}

	case "click", "type", "fill":
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Action completed")
		fmt.Fprintln(os.Stderr, "  pinchtab snap # See updated page")
		fmt.Fprintln(os.Stderr, "  pinchtab screenshot # Visual confirmation")
	}
}

type refInfo struct {
	id   string
	desc string
}

func extractRefs(data map[string]any) []refInfo {
	var refs []refInfo

	if elements, ok := data["elements"].([]any); ok {
		for _, elem := range elements {
			if m, ok := elem.(map[string]any); ok {
				if ref, ok := m["ref"].(string); ok && ref != "" {
					desc := ""
					if role, ok := m["role"].(string); ok {
						desc = role
					}
					if name, ok := m["name"].(string); ok && name != "" {
						desc += ": " + name
					}
					// Only include interactive elements
					if role, ok := m["role"].(string); ok {
						if role == "button" || role == "link" || role == "textbox" ||
							role == "checkbox" || role == "radio" || role == "combobox" {
							refs = append(refs, refInfo{id: ref, desc: desc})
						}
					}
				}
			}
		}
	}

	return refs
}
