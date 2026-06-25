package apiclient

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// CheckServerAndGuide checks if pinchtab server is running and provides guidance
func CheckServerAndGuide(client *http.Client, base, token string) bool {
	req, _ := http.NewRequest("GET", base+"/health", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "dial tcp") {
			fmt.Fprintf(os.Stderr, "Pinchtab server is not running on %s\n", base)
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "To start the server")
			fmt.Fprintln(os.Stderr, "  pinchtab # Run in foreground (recommended for beginners)")
			fmt.Fprintln(os.Stderr, "  pinchtab & # Run in background")
			fmt.Fprintln(os.Stderr, "  PINCHTAB_PORT=9868 pinchtab # Use different port")
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "Then try your command again")
			fmt.Fprintf(os.Stderr, "  %s\n", strings.Join(os.Args, " "))
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "Learn more: https://github.com/pinchtab/pinchtab#quick-start")
			return false
		}
		fmt.Fprintf(os.Stderr, "Cannot connect to Pinchtab server: %v\n", err)
		return false
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == 401 {
		fmt.Fprintln(os.Stderr, "Authentication required. Set PINCHTAB_TOKEN.")
		return false
	}

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Server error %d: %s\n", resp.StatusCode, string(body))
		return false
	}

	return true
}

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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
