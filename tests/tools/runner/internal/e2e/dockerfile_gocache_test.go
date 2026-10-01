package e2e

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

const goBuildCacheMount = "--mount=type=cache,target=/root/.cache/go-build"

var dockerfileWalkSkips = map[string]bool{".tools": true, "tmp": true}

func isDockerfileName(name string) bool {
	return name == "Dockerfile" || strings.HasPrefix(name, "Dockerfile.") || strings.HasSuffix(name, ".Dockerfile")
}

func dockerfileInstructions(text string) []string {
	var out []string
	var cur strings.Builder
	for line := range strings.SplitSeq(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if cur.Len() == 0 && (trimmed == "" || strings.HasPrefix(trimmed, "#")) {
			continue
		}
		if cont, ok := strings.CutSuffix(trimmed, "\\"); ok {
			cur.WriteString(cont + " ")
			continue
		}
		cur.WriteString(trimmed)
		out = append(out, cur.String())
		cur.Reset()
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func uncachedGoBuilds(text string) []string {
	var bad []string
	for _, ins := range dockerfileInstructions(text) {
		if !strings.HasPrefix(strings.ToUpper(ins), "RUN ") {
			continue
		}
		if (strings.Contains(ins, "go build") || strings.Contains(ins, "go install")) && !strings.Contains(ins, goBuildCacheMount) {
			bad = append(bad, ins)
		}
	}
	return bad
}

func TestEveryDockerfileGoBuildKeepsItsCompileCacheAcrossBuilds(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "..")
	withGoBuild := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && (srccensus.ExcludedDir(path) || dockerfileWalkSkips[d.Name()]) {
				return fs.SkipDir
			}
			return nil
		}
		if !isDockerfileName(d.Name()) {
			return nil
		}
		data, err := os.ReadFile(path) // #nosec G304 -- walking the repo's own Dockerfiles
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "go build") || strings.Contains(string(data), "go install") {
			withGoBuild++
		}
		for _, ins := range uncachedGoBuilds(string(data)) {
			t.Errorf("%s compiles without %s, so every image build recompiles from scratch: %s", path, goBuildCacheMount, ins)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if withGoBuild < 4 {
		t.Fatalf("found %d Dockerfiles that run go build, want at least the 4 known; the walk would pass vacuously", withGoBuild)
	}
}

func TestTheGoBuildCacheCensusSeesAContinuedRunAndIgnoresOtherSteps(t *testing.T) {
	const planted = `FROM golang:1.26 AS build
# RUN go build ./commented-out
RUN apk add --no-cache \
    git
RUN CGO_ENABLED=0 \
    go build -o /out/x ./cmd/x
RUN ` + goBuildCacheMount + ` go build -o /out/y ./cmd/y
RUN go install example.com/tool@latest
`
	bad := uncachedGoBuilds(planted)

	if len(bad) != 2 || !strings.Contains(bad[0], "./cmd/x") || !strings.Contains(bad[1], "go install") {
		t.Fatalf("census flagged %q, want the continued go build and the go install only", bad)
	}
}
