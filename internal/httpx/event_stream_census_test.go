package httpx

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

func TestServerSentEventsAreProducedOnlyByEventStream(t *testing.T) {
	const owner = "internal/httpx/event_stream.go"
	exempt := map[string]string{
		"internal/cli/actions/actions_network.go": "a CLIENT sending Accept: text/event-stream, not a producer",
	}
	spellings := []string{`"text/event-stream"`, `"event: `, `": keepalive`}
	ownerSeen := false
	for _, file := range srccensus.Tree(t, filepath.Join("..", ".."), 200) {
		if strings.HasSuffix(file.Name, "_test.go") {
			continue
		}
		hit := false
		for _, spelling := range spellings {
			if strings.Contains(file.Text, spelling) {
				hit = true
			}
		}
		if !hit {
			continue
		}
		if file.Name == owner {
			ownerSeen = true
			continue
		}
		if reason, ok := exempt[file.Name]; ok {
			if reason == "" {
				t.Errorf("%s is exempt with no reason recorded", file.Name)
			}
			continue
		}
		t.Errorf("%s writes SSE headers or frames by hand; produce them through httpx.EventStream so every stream sets the same headers and flushes the same way", file.Name)
	}
	if !ownerSeen {
		t.Fatalf("%s no longer spells the SSE frame; re-point this census at the new owner", owner)
	}
}
