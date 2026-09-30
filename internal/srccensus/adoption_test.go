package srccensus

import (
	"path/filepath"
	"strings"
	"testing"
)

// awaitingPort lists AST-parsing tests that still enumerate or name their sources by hand.
// It is a ratchet, not a licence: an entry leaves when its file is ported, and a new
// hand-rolled census cannot join it.
var awaitingPort = map[string]string{
	"internal/api/types/generated_parity_test.go":            "hand-rolled Glob over the package",
	"internal/audit/types_test.go":                           "hand-rolled ReadDir over the package",
	"internal/bridge/action_resolve_scope_test.go":           "reads action_resolve.go by name",
	"internal/bridge/cdpops/frame_dom_test.go":               "reads frame_dom.go by name and hand-rolls a Glob",
	"internal/bridge/cdpops/pointer_test.go":                 "reads pointer.go by name",
	"internal/bridge/isolated_world_batch_test.go":           "reads action_resolve.go by name",
	"internal/cli/apiclient/apierror_test.go":                "hand-rolled Glob over the package",
	"internal/config/file_config_from_runtime_alias_test.go": "reads config_file_marshal.go by name",
	"internal/handlers/action_unknown_kind_test.go":          "reads actions.go by name",
	"internal/handlers/action_vocab_test.go":                 "hand-rolled Glob over the package",
	"internal/handlers/actions_test.go":                      "parses a named handler file",
	"internal/handlers/capability_refusal_test.go":           "hand-rolled ReadDir over the package",
	"internal/handlers/snapshot_params_test.go":              "reads the snapshot handler by name",
	"internal/mcp/action_payload_contract_test.go":           "hand-rolled ReadDir over the package",
	"internal/mcp/argcheck_test.go":                          "hand-rolled ReadDir and reads handlers.go by name",
	"internal/mcp/tool_result_test.go":                       "hand-rolled Glob over the package",
	"internal/mcp/tools_params_test.go":                      "hand-rolled ReadDir and reads handlers.go and tools.go by name",
	"internal/stealth/ua_test.go":                            "hand-rolled Glob over the package",
}

func TestEveryASTParsingTestUnderInternalBuildsOnSrccensus(t *testing.T) {
	files := TestTree(t, filepath.Join("..", ".."), 100)

	parsers := 0
	handRolled := map[string]bool{}
	for _, file := range files {
		if !strings.HasPrefix(file.Name, "internal/") || strings.HasPrefix(file.Name, "internal/srccensus/") {
			continue
		}
		if !containsAnyMarker(file.Text, []string{"parser.ParseFile(", "parser.ParseDir("}) {
			continue
		}
		parsers++

		consults, err := callsSrccensus(file.Name, file.Text)
		if err != nil {
			t.Errorf("%s: cannot parse, so this guard can neither clear nor flag it: %v", file.Name, err)
			continue
		}
		if consults {
			continue
		}
		handRolled[file.Name] = true
		if _, listed := awaitingPort[file.Name]; !listed {
			t.Errorf("%s parses Go source without srccensus; obtain the files from srccensus.Load (Syntax for raw AST) or srccensus.Tree, so its scope is derived rather than hand-listed", file.Name)
		}
	}
	for name := range awaitingPort {
		if !handRolled[name] {
			t.Errorf("awaitingPort names %s, which no longer parses Go source by hand; delete the entry", name)
		}
	}
	if parsers < 20 {
		t.Fatalf("found only %d AST-parsing tests under internal/; the detector matched almost nothing and would pass vacuously", parsers)
	}
}
