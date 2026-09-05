package readiness

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

func TestDeadlinePollsRunThroughWaitUntil(t *testing.T) {
	const owner = "internal/readiness/readiness.go"
	ownerSeen := false
	for _, file := range srccensus.Tree(t, filepath.Join("..", ".."), 200) {
		if strings.HasSuffix(file.Name, "_test.go") {
			continue
		}
		if file.Name == owner && strings.Contains(file.Text, "func WaitUntil[") {
			ownerSeen = true
		}
		if strings.Contains(file.Text, "for time.Now().Before(deadline)") {
			t.Errorf("%s polls with its own deadline loop; use readiness.WaitUntil or readiness.Poll so every wait honours the context, the interval and the timeout the same way", file.Name)
		}
	}
	if !ownerSeen {
		t.Fatalf("%s no longer declares WaitUntil; re-point this census at its new home", owner)
	}
}
