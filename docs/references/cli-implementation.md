# PinchTab CLI Implementation Guide

## Payload Handling Patterns

### Pattern 1: Simple Flags → JSON Body

**CLI:**
```bash
pinchtab instance launch --mode headed --port 9869
```

**Implementation:**
```go
func cliInstanceLaunch(client *http.Client, base, token string, args []string) {
	body := map[string]any{}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--mode":
			if i+1 < len(args) {
				body["mode"] = args[i+1]
				i++
			}
		case "--port":
			if i+1 < len(args) {
				body["port"] = args[i+1]
				i++
			}
		}
	}

	doPost(client, base, token, "/instances/launch", body)
}
```

**Result:** `POST /instances/launch {"mode": "headed", "port": "9869"}`

---

### Pattern 2: Positional Arguments → Path Params

**CLI:**
```bash
pinchtab instance inst_abc123 stop
pinchtab instance inst_abc123 logs
```

**Implementation:**
```go
func cliInstanceStop(client *http.Client, base, token string, args []string) {
	if len(args) < 1 {
		fatal("Usage: pinchtab instance stop <instance-id>")
	}

	instID := args[0]
	doPost(client, base, token, fmt.Sprintf("/instances/%s/stop", instID), nil)
}
```

**Result:** `POST /instances/inst_abc123/stop`

---

### Pattern 3: Stdin JSON for Complex Payloads

**CLI:**
```bash
cat << 'EOF' | pinchtab --instance inst_abc123 action
{
  "kind": "actions",
  "actions": [
    {"kind": "click", "ref": "e5"},
    {"kind": "type", "ref": "e12", "text": "hello"}
  ]
}
EOF
```

**Implementation:**
```go
func cliAction(client *http.Client, base, token string, cmd string, args []string) {
	// Check if stdin has data
	info, _ := os.Stdin.Stat()
	if (info.Mode() & os.ModeCharDevice) == 0 {
		// Stdin is piped - read JSON from stdin
		body := readJSONFromStdin()
		instanceID := getInstanceID(args)
		doPost(client, base, token, fmt.Sprintf("/instances/%s/action", instanceID), body)
		return
	}

	// Otherwise, parse from flags/args
	body := parseActionFlags(cmd, args)
	instanceID := getInstanceID(args)
	doPost(client, base, token, fmt.Sprintf("/instances/%s/action", instanceID), body)
}

func readJSONFromStdin() map[string]any {
	decoder := json.NewDecoder(os.Stdin)
	var body map[string]any
	if err := decoder.Decode(&body); err != nil {
		fatal("Invalid JSON from stdin: %v", err)
	}
	return body
}
```

**Result:** `POST /instances/inst_abc123/action` with JSON body from stdin

---

### Pattern 4: File Input via Flag

**CLI:**
```bash
pinchtab --instance inst_abc123 action -f actions.json
pinchtab --instance inst_abc123 action --json '{"kind": "click", "ref": "e5"}'
```

**Implementation:**
```go
func cliAction(client *http.Client, base, token string, cmd string, args []string) {
	var body map[string]any

	// Check for file input
	for i := 0; i < len(args); i++ {
		if args[i] == "-f" || args[i] == "--file" {
			if i+1 < len(args) {
				data, err := os.ReadFile(args[i+1])
				if err != nil {
					fatal("Failed to read file: %v", err)
				}
				if err := json.Unmarshal(data, &body); err != nil {
					fatal("Invalid JSON in file: %v", err)
				}
				instanceID := getInstanceID(args)
				doPost(client, base, token, fmt.Sprintf("/instances/%s/action", instanceID), body)
				return
			}
		} else if args[i] == "--json" {
			if i+1 < len(args) {
				if err := json.Unmarshal([]byte(args[i+1]), &body); err != nil {
					fatal("Invalid JSON: %v", err)
				}
				instanceID := getInstanceID(args)
				doPost(client, base, token, fmt.Sprintf("/instances/%s/action", instanceID), body)
				return
			}
		}
	}

	// Fall back to regular flag parsing
	body = parseActionFlags(cmd, args)
	instanceID := getInstanceID(args)
	doPost(client, base, token, fmt.Sprintf("/instances/%s/action", instanceID), body)
}
```

**Result:** JSON loaded from file or string

---

### Pattern 5: Nested Resources (Tabs)

**CLI:**
```bash
# List tabs
pinchtab --instance inst_abc123 tabs

# Create tab
pinchtab --instance inst_abc123 tab create https://example.com

# Navigate specific tab
pinchtab --instance inst_abc123 tab tab_xyz navigate https://google.com

# Lock tab
pinchtab --instance inst_abc123 tab tab_xyz lock --owner agent1 --ttl 60
```

**Implementation:**
```go
func cliTabs(client *http.Client, base, token string, args []string) {
	if len(args) == 0 {
		// pinchtab --instance <id> tabs → List tabs
		instanceID := getInstanceID(args)
		doGet(client, base, token, fmt.Sprintf("/instances/%s/tabs", instanceID))
		return
	}

	action := args[0]
	subArgs := args[1:]
	instanceID := getInstanceID(args[2:]) // Skip 'tab' and action

	switch action {
	case "create":
		if len(subArgs) < 1 {
			fatal("Usage: pinchtab tab create <url>")
		}
		body := map[string]any{"url": subArgs[0]}
		doPost(client, base, token, fmt.Sprintf("/instances/%s/tab", instanceID), body)

	case "close":
		if len(subArgs) < 1 {
			fatal("Usage: pinchtab tab <tabId> close")
		}
		tabID := subArgs[0]
		body := map[string]any{"tabId": tabID}
		doPost(client, base, token, fmt.Sprintf("/instances/%s/tab/close", instanceID), body)

	case "navigate":
		if len(subArgs) < 2 {
			fatal("Usage: pinchtab tab <tabId> navigate <url>")
		}
		tabID := subArgs[0]
		url := subArgs[1]
		body := map[string]any{"tabId": tabID, "url": url}
		doPost(client, base, token, fmt.Sprintf("/instances/%s/tab/navigate", instanceID), body)

	case "lock":
		if len(subArgs) < 1 {
			fatal("Usage: pinchtab tab <tabId> lock [--owner name] [--ttl seconds]")
		}
		tabID := subArgs[0]
		body := map[string]any{"tabId": tabID}

		// Parse flags
		for i := 1; i < len(subArgs); i++ {
			switch subArgs[i] {
			case "--owner":
				if i+1 < len(subArgs) {
					body["owner"] = subArgs[i+1]
					i++
				}
			case "--ttl":
				if i+1 < len(subArgs) {
					ttl, _ := strconv.Atoi(subArgs[i+1])
					body["ttl"] = ttl
					i++
				}
			}
		}

		doPost(client, base, token, fmt.Sprintf("/instances/%s/tab/lock", instanceID), body)
	}
}
```

**Result:**
- `GET /instances/inst_abc123/tabs`
- `POST /instances/inst_abc123/tab {"url": "..."}`
- `POST /instances/inst_abc123/tab/navigate {"tabId": "...", "url": "..."}`
- `POST /instances/inst_abc123/tab/lock {"tabId": "...", "owner": "...", "ttl": 60}`

---

## Global Flag Handling

### Instance Selection

**Goals:**
1. Support explicit `--instance <id>` flag
2. Fallback to `PINCHTAB_INSTANCE` env var
3. Fallback to single running instance (if only one)

**Implementation:**
```go
func getInstanceID(args []string) string {
	// Check for explicit --instance flag
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--instance" {
			return args[i+1]
		}
	}

	// Check env var
	if id := os.Getenv("PINCHTAB_INSTANCE"); id != "" {
		return id
	}

	// For bridge mode (single instance), instances list will have exactly 1
	// and we can use it if no explicit instance was provided
	// This is optional - caller can still require --instance

	fatal("instance not specified: use --instance <id> or set PINCHTAB_INSTANCE")
	return ""
}

func removeInstanceFlag(args []string) []string {
	result := []string{}
	skip := false

	for i, arg := range args {
		if skip {
			skip = false
			continue
		}
		if arg == "--instance" && i+1 < len(args) {
			skip = true
			continue
		}
		result = append(result, arg)
	}

	return result
}
```

---

## Error Handling

**Standard Exit Codes:**
```go
const (
	ExitOK           = 0  // Success
	ExitUserError    = 1  // Bad arguments, file not found
	ExitServerError  = 2  // Server 5xx or connection refused
	ExitTimeout      = 3  // Request timed out
	ExitNotFound     = 4  // Resource not found (404)
	ExitUnauthorized = 5  // Auth failed (401)
	ExitConflict     = 6  // Conflict (409)
)

func doPost(client *http.Client, base, token, path string, body any) error {
	// ... make request ...

	resp, err := client.Do(req)
	if err != nil {
		// Network error
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(ExitServerError)
	}

	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(resp.Body)

	switch resp.StatusCode {
	case 200, 201:
		// Success - print response
		fmt.Println(string(responseBody))
	case 400:
		fmt.Fprintf(os.Stderr, "Error: invalid request\n")
		fmt.Fprintf(os.Stderr, "%s\n", string(responseBody))
		os.Exit(ExitUserError)
	case 404:
		fmt.Fprintf(os.Stderr, "Error: not found\n")
		os.Exit(ExitNotFound)
	case 409:
		fmt.Fprintf(os.Stderr, "Error: conflict (resource already exists or in use)\n")
		os.Exit(ExitConflict)
	case 500, 503:
		fmt.Fprintf(os.Stderr, "Error: server error (%d)\n", resp.StatusCode)
		fmt.Fprintf(os.Stderr, "%s\n", string(responseBody))
		os.Exit(ExitServerError)
	default:
		fmt.Fprintf(os.Stderr, "Error: HTTP %d\n", resp.StatusCode)
		os.Exit(ExitServerError)
	}

	return nil
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(ExitUserError)
}
```

---

## Testing CLI Commands

### Unit Test Example
```go
func TestCliInstanceLaunch(t *testing.T) {
	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/instances/launch" {
			t.Errorf("expected POST /instances/launch, got %s %s", r.Method, r.URL.Path)
		}

		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)

		if body["mode"] != "headed" || body["port"] != "9869" {
			t.Errorf("unexpected body: %v", body)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": "inst_123"})
	}))
	defer server.Close()

	client := &http.Client{}
	args := []string{"--mode", "headed", "--port", "9869"}

	// Should not panic
	cliInstanceLaunch(client, server.URL, "", args)
}
```

### Integration Test Example
```bash
#!/bin/bash
# test-cli.sh

set -e

# Start server
./pinchtab &
SERVER_PID=$!
sleep 2

trap "kill $SERVER_PID" EXIT

# Test instance launch
RESP=$(./pinchtab instance launch --mode headed --port 9999)
INST_ID=$(echo "$RESP" | jq -r .id)
echo "✓ Launched instance: $INST_ID"

# Test with --instance flag
./pinchtab --instance $INST_ID snap -c > /tmp/snap.json
echo "✓ Snapshot with --instance flag"

# Test JSON stdin
echo '{"kind": "click", "ref": "e5"}' | ./pinchtab --instance $INST_ID action
echo "✓ Action with stdin JSON"

# Test file input
echo '{"kind": "navigate", "url": "https://example.com"}' > /tmp/nav.json
./pinchtab --instance $INST_ID action -f /tmp/nav.json
echo "✓ Action with file input"

echo "All tests passed!"
```

---

## Documentation

Update help text:
```go
case "instance":
	if len(args) < 1 {
		fmt.Print(`Instance management:

Usage: pinchtab instance <command> [options]

Commands:
  launch [--mode headed|headless] [--port N]   Create new instance
  logs <id>                                      Show instance logs
  stop <id>                                      Stop instance
  <id> navigate <url>                            Navigate in instance
  <id> snap [flags]                              Snapshot instance
  <id> tab create <url>                          Create tab in instance
  <id> tab <tabId> navigate <url>               Navigate specific tab
  <id> tab <tabId> lock [--owner name] [--ttl N] Lock tab
  <id> tab <tabId> unlock [--owner name]        Unlock tab

Examples:
  pinchtab instance launch --mode headed
  pinchtab instance inst_abc123 logs
  pinchtab --instance inst_abc123 snap -i -c
  pinchtab --instance inst_abc123 tab create https://example.com
`)
		return
	}
```
