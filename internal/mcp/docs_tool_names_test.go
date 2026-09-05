package mcp

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// toolNamePattern matches the way the docs write a tool: the prefix plus the
// lowercase name, whatever markup surrounds it.
var toolNamePattern = regexp.MustCompile(`pinchtab_[a-z0-9_]+`)

// notAToolName holds the strings that carry the tool prefix in prose without
// naming a tool, each with the reason it is not one. Recorded by VALUE so a new
// spelling has to be looked at rather than inherited by a file exemption.
var notAToolName = map[string]string{
	"pinchtab_mcp":        "the server binary's own name, written in client configuration examples",
	"pinchtab_benchmark_": "the prefix of the benchmark results filenames, written as a glob",
	"pinchtab_token":      "a Docker secret name in the compose example, not a tool",
}

// The MCP tool surface as documented has drifted from the surface as registered in
// both directions: a doc telling an integrator to call a tool family that does not
// exist, and a capability with no tool at all. This catches the first directly and
// the second from the other side — a name nobody documents is a name nobody uses.
func TestEveryToolNameInTheDocsIsRegistered(t *testing.T) {
	registered := map[string]bool{}
	for _, tool := range allTools() {
		registered[tool.Name] = true
	}
	if len(registered) < 30 {
		t.Fatalf("only %d tools registered; the census would prove little", len(registered))
	}

	docs := filepath.Join("..", "..", "docs")
	scanned, mentions := 0, 0
	var unknown []string
	err := filepath.WalkDir(docs, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		scanned++
		rel, _ := filepath.Rel(docs, path)
		for _, name := range toolNamePattern.FindAllString(string(body), -1) {
			mentions++
			if registered[name] {
				continue
			}
			if _, recorded := notAToolName[name]; recorded {
				continue
			}
			unknown = append(unknown, rel+": "+name)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk docs: %v", err)
	}
	if scanned < 5 || mentions < 20 {
		t.Fatalf("scanned %d docs finding %d tool mentions; the walk stopped seeing them", scanned, mentions)
	}

	sort.Strings(unknown)
	unknown = dedupe(unknown)
	if len(unknown) > 0 {
		t.Errorf("the docs name %d tool(s) that are not registered, so a reader is sent looking for something that does not exist:\n  %s\nfix the doc, register the tool, or record the spelling in notAToolName with its reason",
			len(unknown), strings.Join(unknown, "\n  "))
	}
}

func dedupe(values []string) []string {
	var out []string
	for i, value := range values {
		if i == 0 || value != values[i-1] {
			out = append(out, value)
		}
	}
	return out
}
