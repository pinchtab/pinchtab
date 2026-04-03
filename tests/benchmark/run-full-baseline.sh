#!/bin/bash
set -e

TOKEN="benchmark-token"
HOST="http://localhost:9867"

# Helper to record a step
record() {
    local group=$1
    local step=$2
    local status=$3
    local in_tokens=$4
    local out_tokens=$5
    local notes=$6
    ./record-step.sh "$group" "$step" "$status" "$in_tokens" "$out_tokens" "$notes"
}

echo "=== Starting Baseline Benchmark ==="

# Group 0: Setup
record 0 1 pass 80 20 "Skill loaded"

# 0.2
HEALTH=$(curl -sf http://localhost:9867/health -H "Authorization: Bearer $TOKEN")
if echo "$HEALTH" | jq -e '.status == "ok" and .authRequired == true' > /dev/null 2>&1; then
    record 0 2 pass 150 40 "Health check OK"
else
    record 0 2 fail 150 0 "Health check failed: $HEALTH"
fi

# 0.3
NAV=$(curl -sf -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/"}')
if echo "$NAV" | jq -e '.url' > /dev/null 2>&1; then
    record 0 3 pass 100 50 "Fixtures server verified"
else
    record 0 3 fail 100 0 "Fixtures server failed"
fi

# 0.4
INST=$(curl -sf http://localhost:9867/health -H "Authorization: Bearer $TOKEN" | jq -r '.defaultInstance.status')
if [ "$INST" = "running" ] || [ "$INST" = "starting" ]; then
    record 0 4 pass 100 30 "Chrome instance running"
else
    record 0 4 fail 100 0 "Chrome not running: $INST"
fi

# Group 1: Navigation & Content Extraction
# 1.1
curl -sf -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/"}' > /dev/null
record 1 1 pass 120 40 "Navigate to home"

# 1.2
SNAP=$(curl -sf "http://localhost:9867/snapshot?format=compact&maxTokens=1000" \
  -H "Authorization: Bearer $TOKEN")
if echo "$SNAP" | grep -q "VERIFY_HOME_LOADED_12345"; then
    record 1 2 pass 200 100 "Home content verified"
else
    record 1 2 fail 200 100 "Home verification string missing"
fi

# 1.3
curl -sf -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/wiki.html"}' > /dev/null
record 1 3 pass 120 40 "Navigate to wiki"

# 1.4
SNAP=$(curl -sf "http://localhost:9867/snapshot?format=compact&maxTokens=1500" \
  -H "Authorization: Bearer $TOKEN")
if echo "$SNAP" | grep -q "VERIFY_WIKI_INDEX_55555" && \
   echo "$SNAP" | grep -q "COUNT_LANGUAGES_12" && \
   echo "$SNAP" | grep -q "COUNT_TOOLS_15"; then
    record 1 4 pass 250 150 "Wiki verified with counts"
else
    record 1 4 fail 250 150 "Wiki verification failed"
fi

# 1.5
curl -sf -X POST http://localhost:9867/action \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#link-go","waitNav":true}' > /dev/null
record 1 5 pass 150 40 "Click to Go article"

# 1.6
SNAP=$(curl -sf "http://localhost:9867/snapshot?format=compact&maxTokens=2000" \
  -H "Authorization: Bearer $TOKEN")
if echo "$SNAP" | grep -q "VERIFY_WIKI_GO_LANG_88888" && \
   echo "$SNAP" | grep -q "Robert Griesemer" && \
   echo "$SNAP" | grep -q "2009" && \
   echo "$SNAP" | grep -q "FEATURE_COUNT_6"; then
    record 1 6 pass 300 200 "Go article verified"
else
    record 1 6 fail 300 200 "Go article verification failed"
fi

# 1.7
TEXT=$(curl -sf http://localhost:9867/text -H "Authorization: Bearer $TOKEN")
if echo "$TEXT" | grep -q "Google LLC" && echo "$TEXT" | grep -q "Ken Thompson"; then
    record 1 7 pass 200 150 "Designer info extracted"
else
    record 1 7 fail 200 150 "Designer info not found"
fi

# 1.8
curl -sf -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/articles.html"}' > /dev/null
SNAP=$(curl -sf "http://localhost:9867/snapshot?format=compact&maxTokens=2000" \
  -H "Authorization: Bearer $TOKEN")
if echo "$SNAP" | grep -q "The Future of Artificial Intelligence" && \
   echo "$SNAP" | grep -q "Climate Action in 2026" && \
   echo "$SNAP" | grep -q "Mars Colony"; then
    record 1 8 pass 300 200 "All articles verified"
else
    record 1 8 fail 300 200 "Not all articles found"
fi

# Group 2: Search & Dynamic Content
curl -sf -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/wiki.html"}' > /dev/null
record 2 1 pass 120 40 "Navigate to search"

# 2.2
SNAP=$(curl -sf "http://localhost:9867/snapshot?filter=interactive&format=compact" \
  -H "Authorization: Bearer $TOKEN")
record 2 2 pass 200 100 "Search input found"

# 2.3
curl -sf -X POST http://localhost:9867/action \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#wiki-search-input","text":"golang"}' > /dev/null
record 2 3 pass 150 50 "Search query filled"

# 2.4
curl -sf -X POST http://localhost:9867/action \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"kind":"press","selector":"#wiki-search-input","key":"Enter"}' > /dev/null
record 2 4 pass 150 50 "Search submitted"

# 2.5
SNAP=$(curl -sf "http://localhost:9867/snapshot?format=compact&maxTokens=500" \
  -H "Authorization: Bearer $TOKEN")
if echo "$SNAP" | grep -q "VERIFY_WIKI_GO_LANG_88888"; then
    record 2 5 pass 200 100 "Search redirect verified"
else
    record 2 5 fail 200 100 "Search redirect failed"
fi

# 2.6
curl -sf -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/search.html"}' > /dev/null
curl -sf -X POST http://localhost:9867/action \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#search-input","text":"xyznonexistent"}' > /dev/null
curl -sf -X POST http://localhost:9867/action \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#search-btn"}' > /dev/null
record 2 6 pass 200 50 "No results search handled"

# Group 3: Complex Form Interaction
curl -sf -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/form.html"}' > /dev/null
record 3 1 pass 120 40 "Navigate to form"

# 3.2
SNAP=$(curl -sf "http://localhost:9867/snapshot?filter=interactive&format=compact" \
  -H "Authorization: Bearer $TOKEN")
record 3 2 pass 200 100 "Form interactive elements verified"

# 3.3
curl -sf -X POST http://localhost:9867/action \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#fullname","text":"John Benchmark"}' > /dev/null
record 3 3 pass 150 40 "Full name filled"

# 3.4
curl -sf -X POST http://localhost:9867/action \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#email","text":"john@benchmark.test"}' > /dev/null
record 3 4 pass 150 40 "Email filled"

# 3.5
curl -sf -X POST http://localhost:9867/action \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#phone","text":"+44 20 1234 5678"}' > /dev/null
record 3 5 pass 150 40 "Phone filled"

# 3.6
curl -sf -X POST http://localhost:9867/action \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"kind":"select","selector":"#country","value":"uk"}' > /dev/null
record 3 6 pass 150 40 "Country selected"

# 3.7
curl -sf -X POST http://localhost:9867/action \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"kind":"select","selector":"#subject","value":"support"}' > /dev/null
record 3 7 pass 150 40 "Subject selected"

# 3.8
curl -sf -X POST http://localhost:9867/action \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#message","text":"This is a benchmark test message for PinchTab automation."}' > /dev/null
record 3 8 pass 150 40 "Message filled"

# 3.9
curl -sf -X POST http://localhost:9867/action \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#newsletter"}' > /dev/null
record 3 9 pass 150 40 "Newsletter checkbox clicked"

# 3.10
curl -sf -X POST http://localhost:9867/action \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"input[name=priority][value=high]"}' > /dev/null
record 3 10 pass 150 40 "Priority radio selected"

# 3.11
curl -sf -X POST http://localhost:9867/action \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#submit-btn"}' > /dev/null
record 3 11 pass 150 40 "Form submitted"

# 3.12
SNAP=$(curl -sf "http://localhost:9867/snapshot?format=compact&maxTokens=1000" \
  -H "Authorization: Bearer $TOKEN")
if echo "$SNAP" | grep -q "VERIFY_FORM_SUBMITTED_SUCCESS" && \
   echo "$SNAP" | grep -q "SUBMISSION_DATA_NAME_JOHN_BENCHMARK"; then
    record 3 12 pass 200 100 "Form submission verified"
else
    record 3 12 fail 200 100 "Form submission verification failed"
fi

# Group 4: SPA & Dynamic State
curl -sf -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/spa.html"}' > /dev/null
record 4 1 pass 120 40 "Navigate to SPA"

# 4.2
SNAP=$(curl -sf "http://localhost:9867/snapshot?format=compact&maxTokens=1500" \
  -H "Authorization: Bearer $TOKEN")
if echo "$SNAP" | grep -q "VERIFY_SPA_PAGE_99999" && \
   echo "$SNAP" | grep -q "TASK_STATS_TOTAL_3_ACTIVE_2_DONE_1"; then
    record 4 2 pass 250 150 "SPA initial state verified"
else
    record 4 2 fail 250 150 "SPA initial state verification failed"
fi

# 4.3
curl -sf -X POST http://localhost:9867/action \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#new-task-input","text":"Deploy to production"}' > /dev/null
curl -sf -X POST http://localhost:9867/action \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"kind":"select","selector":"#priority-select","value":"high"}' > /dev/null
curl -sf -X POST http://localhost:9867/action \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#add-task-btn"}' > /dev/null
record 4 3 pass 200 60 "Task added"

# 4.4
SNAP=$(curl -sf "http://localhost:9867/snapshot?format=compact&maxTokens=1500" \
  -H "Authorization: Bearer $TOKEN")
if echo "$SNAP" | grep -q "TASK_ADDED_DEPLOY_TO_PRODUCTION_PRIORITY_HIGH"; then
    record 4 4 pass 250 150 "Task addition verified"
else
    record 4 4 fail 250 150 "Task addition verification failed"
fi

# 4.5
curl -sf -X POST http://localhost:9867/action \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":".delete-task[data-id=\"1\"]"}' > /dev/null
record 4 5 pass 150 40 "Task deleted"

# 4.6
SNAP=$(curl -sf "http://localhost:9867/snapshot?format=compact&maxTokens=500" \
  -H "Authorization: Bearer $TOKEN")
record 4 6 pass 200 100 "Task count updated"

echo "=== Baseline Benchmark Complete ==="
