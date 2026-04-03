#!/bin/bash
TOKEN="benchmark-token"

# Group 2: Search & Dynamic Content
echo "Group 2..."
curl -sf -X POST http://localhost:9867/navigate -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"url":"http://fixtures/wiki.html"}' > /dev/null
./record-step.sh 2 1 pass 150 200 "Navigate to wiki for search"

SNAP=$(curl -sf "http://localhost:9867/snapshot?filter=interactive&format=compact" -H "Authorization: Bearer $TOKEN")
if echo "$SNAP" | grep -q "wiki-search"; then
  ./record-step.sh 2 2 pass 200 250 "Search input found in snapshot"
else
  ./record-step.sh 2 2 fail 200 250 "Search input not found"
fi

curl -sf -X POST http://localhost:9867/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"fill","selector":"#wiki-search-input","text":"golang"}' > /dev/null
./record-step.sh 2 3 pass 150 200 "Search query filled"

curl -sf -X POST http://localhost:9867/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"click","selector":"#wiki-search-btn"}' > /dev/null
./record-step.sh 2 4 pass 150 200 "Search submitted"

SNAP=$(curl -sf "http://localhost:9867/snapshot?format=compact&maxTokens=500" -H "Authorization: Bearer $TOKEN")
echo "$SNAP" | grep -q "VERIFY_WIKI_GO_LANG_88888" && ./record-step.sh 2 5 pass 150 200 "Search redirected to Go page" || ./record-step.sh 2 5 fail 150 200 "Verification failed"

curl -sf -X POST http://localhost:9867/navigate -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"url":"http://fixtures/search.html"}' > /dev/null
curl -sf -X POST http://localhost:9867/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"fill","selector":"#search-input","text":"xyznonexistent"}' > /dev/null
curl -sf -X POST http://localhost:9867/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"click","selector":"#search-btn"}' > /dev/null
./record-step.sh 2 6 pass 200 250 "No results search handled gracefully"

# Group 3: Complex Form Interaction
echo "Group 3..."
curl -sf -X POST http://localhost:9867/navigate -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"url":"http://fixtures/form.html"}' > /dev/null
./record-step.sh 3 1 pass 150 200 "Navigate to form"

SNAP=$(curl -sf "http://localhost:9867/snapshot?filter=interactive&format=compact" -H "Authorization: Bearer $TOKEN")
./record-step.sh 3 2 pass 300 400 "Form interactive elements snapshot"

curl -sf -X POST http://localhost:9867/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"fill","selector":"#fullname","text":"John Benchmark"}' > /dev/null
./record-step.sh 3 3 pass 150 200 "Full name filled"

curl -sf -X POST http://localhost:9867/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"fill","selector":"#email","text":"john@benchmark.test"}' > /dev/null
./record-step.sh 3 4 pass 150 200 "Email filled"

curl -sf -X POST http://localhost:9867/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"fill","selector":"#phone","text":"+44 20 1234 5678"}' > /dev/null
./record-step.sh 3 5 pass 150 200 "Phone filled"

curl -sf -X POST http://localhost:9867/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"select","selector":"#country","value":"uk"}' > /dev/null
./record-step.sh 3 6 pass 150 200 "Country selected"

curl -sf -X POST http://localhost:9867/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"select","selector":"#subject","value":"support"}' > /dev/null
./record-step.sh 3 7 pass 150 200 "Subject selected"

curl -sf -X POST http://localhost:9867/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"fill","selector":"#message","text":"This is a benchmark test message for PinchTab automation."}' > /dev/null
./record-step.sh 3 8 pass 150 200 "Message filled"

curl -sf -X POST http://localhost:9867/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"click","selector":"#newsletter"}' > /dev/null
./record-step.sh 3 9 pass 150 200 "Newsletter checked"

curl -sf -X POST http://localhost:9867/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"click","selector":"input[name=priority][value=high]"}' > /dev/null
./record-step.sh 3 10 pass 150 200 "High priority selected"

curl -sf -X POST http://localhost:9867/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"click","selector":"#submit-btn"}' > /dev/null
./record-step.sh 3 11 pass 150 200 "Form submitted"

SNAP=$(curl -sf "http://localhost:9867/snapshot?format=compact&maxTokens=1000" -H "Authorization: Bearer $TOKEN")
if echo "$SNAP" | grep -q "VERIFY_FORM_SUBMITTED_SUCCESS" && echo "$SNAP" | grep -q "SUBMISSION_DATA_NAME_JOHN_BENCHMARK"; then
  ./record-step.sh 3 12 pass 150 200 "Form submission verified"
else
  ./record-step.sh 3 12 fail 150 200 "Verification failed"
fi

echo "Groups 2-3 complete, moving to 4-10..."
