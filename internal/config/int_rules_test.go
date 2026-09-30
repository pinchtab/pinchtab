package config

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
)

func TestIntRulesRefuseBadValuesThroughSetAndFileValidation(t *testing.T) {
	bad := func(rule intRule) int {
		for _, n := range []int{-2, 0, 1 << 30} {
			if rule(n) != "" {
				return n
			}
		}
		t.Fatal("rule accepts every probe value")
		return 0
	}
	for path, rule := range intRules {
		t.Run(path, func(t *testing.T) {
			n := bad(rule)

			var setErr ValidationError
			if err := SetConfigValue(&FileConfig{}, path, strconv.Itoa(n)); !errors.As(err, &setErr) || setErr.Field != path {
				t.Fatalf("config set %s=%d: err = %v, want a ValidationError on %s", path, n, err, path)
			}

			fc := &FileConfig{}
			if err := json.Unmarshal(nestedJSON(path, n), fc); err != nil {
				t.Fatalf("decode %s: %v", path, err)
			}
			for _, err := range ValidateFileConfig(fc) {
				var ve ValidationError
				if errors.As(err, &ve) && ve.Field == path && ve.Message == setErr.Message {
					return
				}
			}
			t.Fatalf("ValidateFileConfig did not refuse %s=%d with the rule set used (%q)", path, n, setErr.Message)
		})
	}
}

func nestedJSON(path string, n int) []byte {
	var v any = n
	parts := strings.Split(path, ".")
	for i := len(parts) - 1; i >= 0; i-- {
		v = map[string]any{parts[i]: v}
	}
	raw, _ := json.Marshal(v)
	return raw
}
