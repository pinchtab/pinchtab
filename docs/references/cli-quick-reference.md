# PinchTab CLI Quick Reference

**Legend:**
- `inst_abc123` — Instance ID
- `tab_xyz789` — Tab ID
- `e5` — Element reference (from snapshot)

## Instance Management

### Launch an instance
```bash
# Headless (default)
pinchtab instance launch

# Headed (with window)
pinchtab instance launch --mode headed

# On specific port
pinchtab instance launch --port 9868

# Get instance ID
INST=$(pinchtab instance launch --mode headed | jq -r .id)
echo $INST  # inst_abc123
```

### List running instances
```bash
pinchtab instances

# Get just IDs
pinchtab instances | jq -r '.[] | .id'

# Get specific instance
pinchtab instances | jq '.[] | select(.id == "inst_abc123")'
```

### Instance logs
```bash
pinchtab instance inst_abc123 logs

# Follow logs (continuous)
pinchtab instance inst_abc123 logs | tail -f

# Last 100 lines
pinchtab instance inst_abc123 logs | tail -100
```

### Stop instance
```bash
pinchtab instance inst_abc123 stop
```

---

## Browser Control (Single Instance)

### Navigate
```bash
# Default instance
pinchtab nav https://example.com

# Specific instance
pinchtab --instance inst_abc123 nav https://example.com

# Open in new tab
pinchtab --instance inst_abc123 nav https://example.com --new-tab

# Without images (faster)
pinchtab --instance inst_abc123 nav https://example.com --block-images
```

### Snapshot page
```bash
# Full page
pinchtab --instance inst_abc123 snap

# Interactive elements only
pinchtab --instance inst_abc123 snap -i

# Compact (token-efficient)
pinchtab --instance inst_abc123 snap -c

# Interactive + compact (best for AI)
pinchtab --instance inst_abc123 snap -i -c

# Only changes since last snapshot
pinchtab --instance inst_abc123 snap -d

# Save to file
pinchtab --instance inst_abc123 snap > page.json

# Parse in script
pinchtab --instance inst_abc123 snap -c | jq '.elements[] | .ref' | head -5
```

### Click element
```bash
pinchtab --instance inst_abc123 click e5
```

### Type text
```bash
pinchtab --instance inst_abc123 type e12 "hello world"
```

### Fill input (directly, no events)
```bash
pinchtab --instance inst_abc123 fill e12 "value"
```

### Press key
```bash
pinchtab --instance inst_abc123 press Enter
pinchtab --instance inst_abc123 press Tab
pinchtab --instance inst_abc123 press Escape
```

### Scroll
```bash
pinchtab --instance inst_abc123 scroll down
pinchtab --instance inst_abc123 scroll up
pinchtab --instance inst_abc123 scroll 500  # pixels
```

### Get page text
```bash
pinchtab --instance inst_abc123 text

# Raw text (no JSON wrapper)
pinchtab --instance inst_abc123 text --raw
```

### Screenshot
```bash
# To stdout (PNG)
pinchtab --instance inst_abc123 ss > screenshot.png

# To file
pinchtab --instance inst_abc123 ss -o out.png

# JPEG with quality
pinchtab --instance inst_abc123 ss -o out.jpg -q 85
```

### PDF export
```bash
# Default (A4 portrait)
pinchtab --instance inst_abc123 pdf -o out.pdf

# Landscape
pinchtab --instance inst_abc123 pdf -o out.pdf --landscape

# Letter size
pinchtab --instance inst_abc123 pdf -o out.pdf --paper-width 8.5 --paper-height 11

# Specific pages
pinchtab --instance inst_abc123 pdf -o out.pdf --page-ranges "1-3,5"
```

### Run JavaScript
```bash
pinchtab --instance inst_abc123 eval "document.title"

# JSON result
pinchtab --instance inst_abc123 eval "document.querySelectorAll('a').length"

# Complex script
pinchtab --instance inst_abc123 eval '
  JSON.stringify({
    title: document.title,
    url: location.href,
    links: document.querySelectorAll("a").length
  })
'
```

---

## Tab Management

### List tabs
```bash
pinchtab --instance inst_abc123 tabs

# Get tab IDs
pinchtab --instance inst_abc123 tabs | jq -r '.tabs[] | .id'

# Count tabs
pinchtab --instance inst_abc123 tabs | jq '.tabs | length'
```

### Create tab
```bash
# Create and get ID
TAB=$(pinchtab --instance inst_abc123 tab create https://example.com | jq -r .id)
echo $TAB  # tab_xyz789
```

### Navigate specific tab
```bash
pinchtab --instance inst_abc123 tab tab_xyz789 navigate https://google.com
```

### Close tab
```bash
pinchtab --instance inst_abc123 tab tab_xyz789 close
```

### Lock tab (prevent concurrent access)
```bash
# Lock for 60 seconds
pinchtab --instance inst_abc123 tab tab_xyz789 lock --owner my-agent --ttl 60

# After work, unlock
pinchtab --instance inst_abc123 tab tab_xyz789 unlock --owner my-agent
```

---

## Complex Actions

### Multi-step workflow (JSON stdin)
```bash
cat << 'EOF' | pinchtab --instance inst_abc123 action
{
  "kind": "actions",
  "actions": [
    {"kind": "click", "ref": "e1"},
    {"kind": "type", "ref": "e2", "text": "search query"},
    {"kind": "press", "key": "Enter"},
    {"kind": "wait", "time": 2000},
    {"kind": "click", "ref": "e5"}
  ]
}
EOF
```

### From file
```bash
# Create actions file
cat > actions.json << 'EOF'
{
  "kind": "actions",
  "actions": [
    {"kind": "click", "ref": "e1"},
    {"kind": "type", "ref": "e2", "text": "hello"},
    {"kind": "press", "key": "Enter"}
  ]
}
EOF

# Run it
pinchtab --instance inst_abc123 action -f actions.json
```

### From inline JSON
```bash
pinchtab --instance inst_abc123 action --json '{"kind":"click","ref":"e5"}'
```

---

## Typical Workflow

### 1. Start orchestrator
```bash
# Terminal 1: Start the dashboard/orchestrator
pinchtab
# Now listening on http://localhost:9867
```

### 2. Launch instance
```bash
# Terminal 2: Launch a headed instance
INST=$(pinchtab instance launch --mode headed | jq -r .id)
echo "Instance: $INST"
```

### 3. Navigate and interact
```bash
# Navigate to website
pinchtab --instance $INST nav https://github.com/pinchtab/pinchtab

# See page structure
pinchtab --instance $INST snap -i -c | jq .

# Click button (find e5 from snapshot)
pinchtab --instance $INST click e5

# See result
pinchtab --instance $INST snap -i -c | jq '.elements[] | select(.ref == "e5")'
```

### 4. Extract data
```bash
# Get all visible text
pinchtab --instance $INST text --raw

# Count links
pinchtab --instance $INST eval 'document.querySelectorAll("a").length'

# Export page
pinchtab --instance $INST pdf -o page.pdf
```

### 5. Cleanup
```bash
# Stop instance
pinchtab instance $INST stop

# Verify stopped
pinchtab instances
```

---

## Scripting Examples

### Batch instances
```bash
# Launch 3 instances
for i in {1..3}; do
  PORT=$((9868 + i))
  INST=$(pinchtab instance launch --mode headed --port $PORT | jq -r .id)
  echo "Instance $i: $INST"
done
```

### Parallel navigation
```bash
# Navigate multiple instances concurrently
for inst in $(pinchtab instances | jq -r '.[] | .id'); do
  (pinchtab --instance $inst nav https://example.com) &
done
wait

echo "All instances navigated"
```

### Monitor instances
```bash
# Watch instance status
while true; do
  clear
  pinchtab instances | jq -r '.[] | "\(.id) (\(.mode)): \(.status)"'
  sleep 2
done
```

### Cleanup all instances
```bash
# Stop all instances
pinchtab instances | jq -r '.[] | .id' | xargs -I {} pinchtab instance {} stop
```

---

## Troubleshooting

### Check server status
```bash
pinchtab health
# Should print: {"status": "ok"}
```

### View server logs
```bash
# If running in foreground, Ctrl+C to see logs
# If running in background:
jobs
fg  # bring to foreground
```

### Instance not starting?
```bash
# Check logs
pinchtab instance inst_abc123 logs | tail -50

# Check port availability
lsof -i :9868
```

### Can't connect to instance?
```bash
# Verify instance is running
pinchtab instances | jq '.[] | select(.id == "inst_abc123")'

# Check status
pinchtab instances | jq '.[] | select(.id == "inst_abc123") | .status'
# Should be "running"
```

### Need to specify server address?
```bash
# For remote server
export PINCHTAB_URL=http://192.168.1.100:9867

# Or per-command (coming soon)
pinchtab --server http://192.168.1.100:9867 instances
```

---

## Environment Variables

```bash
# Server address (if not on localhost:9867)
export PINCHTAB_URL=http://localhost:9867

# Server port (alternative to PINCHTAB_URL)
export BRIDGE_PORT=9868

# Default instance (skip --instance flag)
export PINCHTAB_INSTANCE=inst_abc123

# Auth token
export PINCHTAB_TOKEN=sk_xxx

# Request timeout
export PINCHTAB_TIMEOUT=30

# Output format
export PINCHTAB_FORMAT=json  # json, text (coming soon)

# Disable colors
export PINCHTAB_NO_COLOR=1
```

---

## Common Patterns

### Wait for page load, then interact
```bash
pinchtab --instance inst_abc123 nav https://example.com
sleep 2  # Wait for page
pinchtab --instance inst_abc123 snap -i
```

### Click, wait, screenshot
```bash
pinchtab --instance inst_abc123 click e5
sleep 1
pinchtab --instance inst_abc123 ss -o result.png
```

### Form fill
```bash
pinchtab --instance inst_abc123 fill e1 "John Doe"
pinchtab --instance inst_abc123 fill e2 "john@example.com"
pinchtab --instance inst_abc123 click e3  # Submit button
sleep 2
pinchtab --instance inst_abc123 snap
```

### Search and verify
```bash
pinchtab --instance inst_abc123 nav https://google.com
pinchtab --instance inst_abc123 fill e1 "golang"
pinchtab --instance inst_abc123 press Enter
sleep 2
pinchtab --instance inst_abc123 text | grep -q "golang"
echo "Search results found"
```

---

## Exit Codes

```bash
pinchtab instance inst_abc123 logs
echo $?  # 0 = success

pinchtab --instance nonexistent snap
echo $?  # 4 = not found

pinchtab instance launch --invalid-flag
echo $?  # 1 = user error

curl http://localhost:9867/health > /dev/null || {
  echo "Server down"  # 2 = server error
}
```
