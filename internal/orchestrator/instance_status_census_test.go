package orchestrator

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestInstanceStatusIsNeverABareLiteral(t *testing.T) {
	bare := regexp.MustCompile(`Status\s*(==|!=|=)\s*"`)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	constUses := 0
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		constUses += strings.Count(string(src), "bridge.InstanceStatus")
		for i, line := range strings.Split(string(src), "\n") {
			if bare.MatchString(line) {
				t.Errorf("%s:%d compares or assigns an instance status by bare literal; use bridge.InstanceStatus*", file, i+1)
			}
		}
	}
	if constUses < 20 {
		t.Fatalf("only %d bridge.InstanceStatus uses in the package; the census would prove little", constUses)
	}
}
