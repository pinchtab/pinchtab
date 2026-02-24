package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
)

func printHelp() {
	fmt.Printf(`pinchtab %s — Browser control for AI agents

MODES:
  pinchtab                  Start server (default port 9867)
  pinchtab dashboard        Start profile manager/orchestrator
  pinchtab connect <name>   Get URL for a running profile

CLI (requires running server):
  pinchtab nav <url>                    Navigate to URL
  pinchtab snap [-i] [-c] [-d]         Snapshot accessibility tree
  pinchtab click <ref>                  Click element
  pinchtab type <ref> <text>            Type into element
  pinchtab press <key>                  Press key (Enter, Tab, Escape...)
  pinchtab fill <ref|selector> <text>   Fill input directly
  pinchtab hover <ref>                  Hover element
  pinchtab scroll <ref|pixels>          Scroll to element or by pixels
  pinchtab select <ref> <value>         Select dropdown option
  pinchtab focus <ref>                  Focus element
  pinchtab text [--raw]                 Extract readable text
  pinchtab tabs [new <url>|close <id>]  Manage tabs
  pinchtab ss [-o file] [-q 80]         Screenshot
  pinchtab eval <expression>            Run JavaScript
  pinchtab pdf [-o file] [--landscape]  Export page as PDF
  pinchtab health                       Check server status

SNAPSHOT FLAGS:
  -i, --interactive    Interactive elements only (buttons, links, inputs)
  -c, --compact        Compact format (most token-efficient)
  -d, --diff           Only changes since last snapshot
  -s, --selector CSS   Scope to CSS selector
  --max-tokens N       Truncate to ~N tokens
  --depth N            Max tree depth
  --tab ID             Target specific tab

ENVIRONMENT:
  PINCHTAB_URL         Server URL (default: http://127.0.0.1:9867)
  PINCHTAB_TOKEN       Auth token (or BRIDGE_TOKEN for server)
  BRIDGE_PORT          Server port (default: 9867)
  BRIDGE_HEADLESS      Run Chrome headless (default: true)

Examples:
  pinchtab nav https://example.com
  pinchtab snap -i -c
  pinchtab click e5
  pinchtab type e12 hello world
  pinchtab press Enter
  pinchtab text | jq .text
  pinchtab eval "document.title"
`, version)
}

var cliCommands = map[string]bool{
	"nav": true, "navigate": true,
	"snap": true, "snapshot": true,
	"click": true, "type": true, "press": true, "fill": true,
	"hover": true, "scroll": true, "select": true, "focus": true,
	"text": true, "tabs": true, "tab": true,
	"screenshot": true, "ss": true,
	"eval": true, "evaluate": true,
	"pdf": true, "health": true,
	"help": true,
}

func isCLICommand(cmd string) bool {
	return cliCommands[cmd]
}

func runCLI(cfg *config.RuntimeConfig) {
	cmd := os.Args[1]
	args := os.Args[2:]

	base := fmt.Sprintf("http://%s:%s", cfg.Bind, cfg.Port)
	if envURL := os.Getenv("PINCHTAB_URL"); envURL != "" {
		base = strings.TrimRight(envURL, "/")
	}

	token := cfg.Token
	if envToken := os.Getenv("PINCHTAB_TOKEN"); envToken != "" {
		token = envToken
	}

	client := &http.Client{Timeout: 30 * time.Second}

	switch cmd {
	case "nav", "navigate":
		cliNavigate(client, base, token, args)
	case "snap", "snapshot":
		cliSnapshot(client, base, token, args)
	case "click", "type", "press", "fill", "hover", "scroll", "select", "focus":
		cliAction(client, base, token, cmd, args)
	case "text":
		cliText(client, base, token, args)
	case "tabs", "tab":
		cliTabs(client, base, token, args)
	case "screenshot", "ss":
		cliScreenshot(client, base, token, args)
	case "eval", "evaluate":
		cliEvaluate(client, base, token, args)
	case "pdf":
		cliPDF(client, base, token, args)
	case "health":
		cliHealth(client, base, token)
	case "help":
		cliHelp()
	}
}

func cliHelp() {
	fmt.Print(`Pinchtab CLI — browser control from the command line

Usage: pinchtab <command> [args] [flags]

Commands:
  nav, navigate <url>     Navigate to URL (--new-tab, --block-images)
  snap, snapshot          Accessibility tree snapshot (-i, -c, -d, --max-tokens N)
  click <ref>             Click element by ref
  type <ref> <text>       Type text into element
  fill <ref> <text>       Set input value (no key events)
  press <key>             Press a key (Enter, Tab, Escape, ...)
  hover <ref>             Hover over element
  scroll <direction>      Scroll page (up, down, left, right)
  select <ref> <value>    Select dropdown option
  focus <ref>             Focus element
  text                    Extract page text (--raw for innerText)
  tabs                    List open tabs
  tabs new <url>          Open new tab
  tabs close <tabId>      Close tab
  ss, screenshot          Take screenshot (-o file, -q quality)
  eval <expression>       Evaluate JavaScript
  pdf                     Export page as PDF (-o file, --landscape, --scale N)
  health                  Server health check
  help                    Show this help

Environment:
  PINCHTAB_URL            Server URL (default: http://localhost:9867)
  PINCHTAB_TOKEN          Auth token (sent as Bearer)

Pipe with jq:
  pinchtab snap -i | jq '.nodes[] | select(.role=="link")'
`)
	os.Exit(0)
}

// --- navigate ---

func cliNavigate(client *http.Client, base, token string, args []string) {
	if len(args) < 1 {
		fatal("Usage: pinchtab nav <url> [--new-tab] [--block-images]")
	}
	body := map[string]any{"url": args[0]}
	for _, a := range args[1:] {
		switch a {
		case "--new-tab":
			body["newTab"] = true
		case "--block-images":
			body["blockImages"] = true
		}
	}
	doPost(client, base, token, "/navigate", body)
}

// --- snapshot ---

func cliSnapshot(client *http.Client, base, token string, args []string) {
	params := url.Values{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--interactive", "-i":
			params.Set("filter", "interactive")
		case "--compact", "-c":
			params.Set("format", "compact")
		case "--text":
			params.Set("format", "text")
		case "--diff", "-d":
			params.Set("diff", "true")
		case "--selector", "-s":
			if i+1 < len(args) {
				i++
				params.Set("selector", args[i])
			}
		case "--max-tokens":
			if i+1 < len(args) {
				i++
				params.Set("maxTokens", args[i])
			}
		case "--depth":
			if i+1 < len(args) {
				i++
				params.Set("depth", args[i])
			}
		case "--tab":
			if i+1 < len(args) {
				i++
				params.Set("tabId", args[i])
			}
		}
	}
	doGet(client, base, token, "/snapshot", params)
}

// --- element actions ---

func cliAction(client *http.Client, base, token, kind string, args []string) {
	body := map[string]any{"kind": kind}

	switch kind {
	case "click", "hover", "focus":
		if len(args) < 1 {
			fatal("Usage: pinchtab %s <ref> [--wait-nav]", kind)
		}
		body["ref"] = args[0]
		for _, a := range args[1:] {
			if a == "--wait-nav" {
				body["waitNav"] = true
			}
		}
	case "type":
		if len(args) < 2 {
			fatal("Usage: pinchtab type <ref> <text>")
		}
		body["ref"] = args[0]
		body["text"] = strings.Join(args[1:], " ")
	case "fill":
		if len(args) < 2 {
			fatal("Usage: pinchtab fill <ref|selector> <text>")
		}
		if strings.HasPrefix(args[0], "e") {
			body["ref"] = args[0]
		} else {
			body["selector"] = args[0]
		}
		body["text"] = strings.Join(args[1:], " ")
	case "press":
		if len(args) < 1 {
			fatal("Usage: pinchtab press <key>  (e.g. Enter, Tab, Escape)")
		}
		body["key"] = args[0]
	case "scroll":
		if len(args) < 1 {
			fatal("Usage: pinchtab scroll <ref|pixels>  (e.g. e5 or 800)")
		}
		if strings.HasPrefix(args[0], "e") {
			body["ref"] = args[0]
		} else {
			body["scrollY"] = args[0]
		}
	case "select":
		if len(args) < 2 {
			fatal("Usage: pinchtab select <ref> <value>")
		}
		body["ref"] = args[0]
		body["value"] = args[1]
	}

	doPost(client, base, token, "/action", body)
}

// --- text ---

func cliText(client *http.Client, base, token string, args []string) {
	params := url.Values{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--raw":
			params.Set("mode", "raw")
		case "--tab":
			if i+1 < len(args) {
				i++
				params.Set("tabId", args[i])
			}
		}
	}
	doGet(client, base, token, "/text", params)
}

// --- tabs ---

func cliTabs(client *http.Client, base, token string, args []string) {
	if len(args) == 0 {
		doGet(client, base, token, "/tabs", nil)
		return
	}
	switch args[0] {
	case "new":
		body := map[string]any{"action": "new"}
		if len(args) > 1 {
			body["url"] = args[1]
		}
		doPost(client, base, token, "/tab", body)
	case "close":
		if len(args) < 2 {
			fatal("Usage: pinchtab tab close <tabId>")
		}
		doPost(client, base, token, "/tab", map[string]any{
			"action": "close",
			"tabId":  args[1],
		})
	default:
		fatal("Usage: pinchtab tabs [new <url>|close <tabId>]")
	}
}

// --- screenshot ---

func cliScreenshot(client *http.Client, base, token string, args []string) {
	params := url.Values{}
	params.Set("raw", "true")
	outFile := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o", "--output":
			if i+1 < len(args) {
				i++
				outFile = args[i]
			}
		case "--quality", "-q":
			if i+1 < len(args) {
				i++
				params.Set("quality", args[i])
			}
		case "--tab":
			if i+1 < len(args) {
				i++
				params.Set("tabId", args[i])
			}
		}
	}

	if outFile == "" {
		outFile = fmt.Sprintf("screenshot-%s.jpg", time.Now().Format("20060102-150405"))
	}

	data := doGetRaw(client, base, token, "/screenshot", params)
	if data == nil {
		return
	}
	if err := os.WriteFile(outFile, data, 0600); err != nil {
		fatal("Write failed: %v", err)
	}
	fmt.Printf("Saved %s (%d bytes)\n", outFile, len(data))
}

// --- evaluate ---

func cliEvaluate(client *http.Client, base, token string, args []string) {
	if len(args) < 1 {
		fatal("Usage: pinchtab eval <expression>")
	}
	expr := strings.Join(args, " ")
	doPost(client, base, token, "/evaluate", map[string]any{
		"expression": expr,
	})
}

// --- pdf ---

func cliPDF(client *http.Client, base, token string, args []string) {
	params := url.Values{}
	params.Set("raw", "true")
	outFile := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o", "--output":
			if i+1 < len(args) {
				i++
				outFile = args[i]
			}
		case "--landscape":
			params.Set("landscape", "true")
		case "--scale":
			if i+1 < len(args) {
				i++
				params.Set("scale", args[i])
			}
		case "--tab":
			if i+1 < len(args) {
				i++
				params.Set("tabId", args[i])
			}
		}
	}

	if outFile == "" {
		outFile = fmt.Sprintf("page-%s.pdf", time.Now().Format("20060102-150405"))
	}

	data := doGetRaw(client, base, token, "/pdf", params)
	if data == nil {
		return
	}
	if err := os.WriteFile(outFile, data, 0600); err != nil {
		fatal("Write failed: %v", err)
	}
	fmt.Printf("Saved %s (%d bytes)\n", outFile, len(data))
}

// --- health ---

func cliHealth(client *http.Client, base, token string) {
	doGet(client, base, token, "/health", nil)
}

// --- helpers ---

func doGet(client *http.Client, base, token, path string, params url.Values) {
	u := base + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, _ := http.NewRequest("GET", u, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		fatal("Request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		fmt.Fprintf(os.Stderr, "Error %d: %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	// Pretty-print JSON if possible
	var buf bytes.Buffer
	if json.Indent(&buf, body, "", "  ") == nil {
		fmt.Println(buf.String())
	} else {
		fmt.Println(string(body))
	}
}

func doGetRaw(client *http.Client, base, token, path string, params url.Values) []byte {
	u := base + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, _ := http.NewRequest("GET", u, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		fatal("Request failed: %v", err)
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		fmt.Fprintf(os.Stderr, "Error %d: %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}
	return body
}

func doPost(client *http.Client, base, token, path string, body map[string]any) {
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", base+path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		fatal("Request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		fmt.Fprintf(os.Stderr, "Error %d: %s\n", resp.StatusCode, string(respBody))
		os.Exit(1)
	}

	var buf bytes.Buffer
	if json.Indent(&buf, respBody, "", "  ") == nil {
		fmt.Println(buf.String())
	} else {
		fmt.Println(string(respBody))
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
