package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const (
	minScenarioFilesScanned = 80
	minHelperFunctions      = 100
	minScenarioFunctions    = 10
)

var (
	funcOpensAtColumnZero = regexp.MustCompile(`^(?:function\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*\(\)\s*\{`)
	assignsE2EServer      = regexp.MustCompile(`^\s*(?:export\s+)?E2E_SERVER=`)
	installsTrap          = regexp.MustCompile(`^\s*trap\s`)
)

type scenarioFile struct {
	Name string
	Body string
}

func loadScenarioFiles(t *testing.T) []scenarioFile {
	t.Helper()

	var files []scenarioFile
	err := filepath.WalkDir("scenarios", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".sh") {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		files = append(files, scenarioFile{Name: path, Body: string(body)})
		return nil
	})
	if err != nil {
		t.Fatalf("cannot walk scenarios, so these rules would check nothing: %v", err)
	}
	if len(files) < minScenarioFilesScanned {
		t.Fatalf("scanned %d scenario files, want at least %d; the walk matched almost nothing and these rules would pass vacuously", len(files), minScenarioFilesScanned)
	}
	return files
}

func functionsDefinedIn(body string) map[string]bool {
	defined := map[string]bool{}
	for line := range strings.SplitSeq(body, "\n") {
		if m := funcOpensAtColumnZero.FindStringSubmatch(line); m != nil {
			defined[m[1]] = true
		}
	}
	return defined
}

func lineIsInsideAFunction(body string) []bool {
	lines := strings.Split(body, "\n")
	inside := make([]bool, len(lines))
	open := false
	for i, line := range lines {
		if !open && funcOpensAtColumnZero.MatchString(line) {
			open = true
			inside[i] = true
			continue
		}
		inside[i] = open
		if open && strings.TrimRight(line, " \t") == "}" {
			open = false
		}
	}
	return inside
}

func TestNoScenarioAssignsE2EServerAtFileScope(t *testing.T) {
	var offenders []string
	guarded := 0
	for _, f := range loadScenarioFiles(t) {
		inside := lineIsInsideAFunction(f.Body)
		for i, line := range strings.Split(f.Body, "\n") {
			if !assignsE2EServer.MatchString(line) {
				continue
			}
			if inside[i] {
				guarded++
				continue
			}
			offenders = append(offenders, fmt.Sprintf("%s:%d: %s", f.Name, i+1, strings.TrimSpace(line)))
		}
	}
	if guarded == 0 && len(offenders) == 0 {
		t.Fatal("no scenario assigns E2E_SERVER anywhere; this rule has nothing to guard and would pass vacuously — re-point it at whatever now retargets the server")
	}
	if len(offenders) > 0 {
		t.Errorf("these scenarios retarget E2E_SERVER at file scope, so an early exit above the restore leaves the rest of the file talking to the wrong server; wrap the block in a function and call it through with_server:\n%s",
			strings.Join(offenders, "\n"))
	}
}

func TestNoScenarioInstallsATrap(t *testing.T) {
	var offenders []string
	for _, f := range loadScenarioFiles(t) {
		for i, line := range strings.Split(f.Body, "\n") {
			if installsTrap.MatchString(line) {
				offenders = append(offenders, fmt.Sprintf("%s:%d: %s", f.Name, i+1, strings.TrimSpace(line)))
			}
		}
	}
	if len(offenders) > 0 {
		t.Errorf("bash keeps exactly one EXIT trap and the executor owns it to decide the scenario's verdict, so a scenario installing its own silently replaces it; define scenario_cleanup instead:\n%s",
			strings.Join(offenders, "\n"))
	}
}

func TestNoScenarioDependsOnAFunctionDefinedInAnotherScenario(t *testing.T) {
	files := loadScenarioFiles(t)

	helperFuncs := map[string]bool{}
	for _, path := range append(helperPaths(t), "run.sh") {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("cannot read %s, so shared functions would look scenario-local: %v", path, err)
		}
		for name := range functionsDefinedIn(string(body)) {
			helperFuncs[name] = true
		}
	}
	if len(helperFuncs) < minHelperFunctions {
		t.Fatalf("found %d helper functions, want at least %d; the helper scan matched almost nothing and every shared call would read as a cross-file dependency", len(helperFuncs), minHelperFunctions)
	}

	ownerOf := map[string][]string{}
	for _, f := range files {
		for name := range functionsDefinedIn(f.Body) {
			ownerOf[name] = append(ownerOf[name], f.Name)
		}
	}
	if len(ownerOf) < minScenarioFunctions {
		t.Fatalf("found %d scenario-defined functions, want at least %d; this rule would have nothing to catch", len(ownerOf), minScenarioFunctions)
	}

	var offenders []string
	for _, f := range files {
		own := functionsDefinedIn(f.Body)
		for i, line := range strings.Split(f.Body, "\n") {
			call := strings.Fields(strings.TrimSpace(line))
			if len(call) == 0 {
				continue
			}
			name := call[0]
			if own[name] || helperFuncs[name] || ownerOf[name] == nil {
				continue
			}
			offenders = append(offenders, fmt.Sprintf("%s:%d calls %s(), defined only in %s", f.Name, i+1, name, strings.Join(ownerOf[name], ", ")))
		}
	}
	if len(offenders) > 0 {
		t.Errorf("each scenario file runs in its own subshell, so a function defined in another scenario is not in scope; move it to tests/e2e/helpers:\n%s",
			strings.Join(offenders, "\n"))
	}
}

func helperPaths(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir("helpers")
	if err != nil {
		t.Fatalf("cannot read helpers dir: %v", err)
	}
	var paths []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sh") {
			paths = append(paths, filepath.Join("helpers", e.Name()))
		}
	}
	return paths
}
