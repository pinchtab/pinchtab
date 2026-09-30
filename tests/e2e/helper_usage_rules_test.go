package e2e

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var inlineTabIDParse = regexp.MustCompile(`jq\s+-r\s+['"]\.tabId['"]`)

func TestScenariosReadTabIDThroughTheHelper(t *testing.T) {
	var offenders []string
	for _, f := range loadScenarioFiles(t) {
		for i, line := range strings.Split(f.Body, "\n") {
			if inlineTabIDParse.MatchString(line) {
				offenders = append(offenders, f.Name+":"+strconv.Itoa(i+1)+": "+strings.TrimSpace(line))
			}
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("read a tab id with get_tab_id (or get_tab_id \"$JSON\") from helpers/base.sh, not an inline jq:\n  %s",
			strings.Join(offenders, "\n  "))
	}
}
