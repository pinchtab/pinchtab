package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	scenarioRoot         = "tests/e2e/scenarios"
	scenarioManifestFile = "tests/e2e/scenarios/manifest.json"

	tierBasic    = "basic"
	tierExtended = "extended"
	tierSmoke    = "smoke"
)

var composeServiceOrder = []string{
	"pinchtab",
	"pinchtab-secure",
	"pinchtab-autoclose",
	"pinchtab-medium",
	"pinchtab-full",
	"pinchtab-retain",
	"pinchtab-ghostchrome",
	"pinchtab-bridge",
	"fixtures",
}

var readyTargetOrder = []string{
	"E2E_SERVER",
	"E2E_SECURE_SERVER",
	"E2E_AUTOCLOSE_SERVER",
	"E2E_MEDIUM_SERVER",
	"E2E_FULL_SERVER",
	"E2E_RETAIN_SERVER",
	"E2E_SERVER_GHOSTCHROME",
	"E2E_BRIDGE_URL|60|E2E_BRIDGE_TOKEN",
}

type scenarioManifest struct {
	Scenarios map[string]scenarioManifestEntry `json:"scenarios"`
}

type scenarioManifestEntry struct {
	Tier     string   `json:"tier,omitempty"`
	Helper   string   `json:"helper,omitempty"`
	Services []string `json:"services,omitempty"`
	Ready    []string `json:"ready,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

type scenarioMeta struct {
	Key      string
	Group    string
	File     string
	Tier     string
	Helper   string
	Services []string
	Ready    []string
	Tags     []string
}

type scenarioCatalog struct {
	byGroup map[string][]scenarioMeta
	byKey   map[string]scenarioMeta
}

func (r *Runner) loadScenarioCatalog() (scenarioCatalog, error) {
	root := filepath.Join(r.repoRoot, scenarioRoot)
	matches, err := filepath.Glob(filepath.Join(root, "*", "*.sh"))
	if err != nil {
		return scenarioCatalog{}, err
	}
	sort.Strings(matches)

	manifest, err := readScenarioManifest(filepath.Join(r.repoRoot, scenarioManifestFile))
	if err != nil {
		return scenarioCatalog{}, err
	}
	remaining := map[string]bool{}
	for key := range manifest.Scenarios {
		remaining[key] = true
	}

	catalog := scenarioCatalog{
		byGroup: map[string][]scenarioMeta{},
		byKey:   map[string]scenarioMeta{},
	}
	for _, match := range matches {
		rel, err := filepath.Rel(root, match)
		if err != nil {
			return scenarioCatalog{}, err
		}
		key := filepath.ToSlash(rel)
		meta, err := defaultScenarioMeta(key)
		if err != nil {
			return scenarioCatalog{}, err
		}
		if entry, ok := manifest.Scenarios[key]; ok {
			meta = applyScenarioManifestEntry(meta, entry)
			delete(remaining, key)
		}
		if err := validateScenarioMeta(meta); err != nil {
			return scenarioCatalog{}, err
		}
		catalog.byGroup[meta.Group] = append(catalog.byGroup[meta.Group], meta)
		catalog.byKey[meta.Key] = meta
	}

	if len(remaining) > 0 {
		keys := make([]string, 0, len(remaining))
		for key := range remaining {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		return scenarioCatalog{}, fmt.Errorf("scenario manifest references missing file(s): %s", strings.Join(keys, ", "))
	}
	return catalog, nil
}

func readScenarioManifest(path string) (scenarioManifest, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return scenarioManifest{Scenarios: map[string]scenarioManifestEntry{}}, nil
	}
	if err != nil {
		return scenarioManifest{}, err
	}
	var manifest scenarioManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return scenarioManifest{}, fmt.Errorf("parse %s: %w", scenarioManifestFile, err)
	}
	if manifest.Scenarios == nil {
		manifest.Scenarios = map[string]scenarioManifestEntry{}
	}
	return manifest, nil
}

func defaultScenarioMeta(key string) (scenarioMeta, error) {
	parts := strings.SplitN(key, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return scenarioMeta{}, fmt.Errorf("invalid scenario key %q", key)
	}
	group := parts[0]
	file := parts[1]
	tier := tierExtended
	switch {
	case strings.HasSuffix(file, "-basic.sh"):
		tier = tierBasic
	case strings.HasSuffix(file, "-smoke.sh"):
		tier = tierSmoke
	}
	helper := "api"
	if group == "cli" {
		helper = "cli"
	}

	return scenarioMeta{
		Key:      key,
		Group:    group,
		File:     file,
		Tier:     tier,
		Helper:   helper,
		Services: []string{"pinchtab", "fixtures"},
		Ready:    primaryReady(),
		Tags:     defaultScenarioTags(group, file, tier),
	}, nil
}

func applyScenarioManifestEntry(meta scenarioMeta, entry scenarioManifestEntry) scenarioMeta {
	if entry.Tier != "" {
		meta.Tier = strings.TrimSpace(entry.Tier)
	}
	if entry.Helper != "" {
		meta.Helper = strings.TrimSpace(entry.Helper)
	}
	if len(entry.Services) > 0 {
		meta.Services = normalizeStrings(entry.Services)
	}
	if len(entry.Ready) > 0 {
		meta.Ready = normalizeStrings(entry.Ready)
	}
	meta.Tags = normalizeStrings(append(meta.Tags, entry.Tags...))
	return meta
}

func validateScenarioMeta(meta scenarioMeta) error {
	switch meta.Tier {
	case tierBasic, tierExtended, tierSmoke:
	default:
		return fmt.Errorf("scenario %s has invalid tier %q", meta.Key, meta.Tier)
	}
	switch meta.Helper {
	case "api", "cli":
	default:
		return fmt.Errorf("scenario %s has invalid helper %q", meta.Key, meta.Helper)
	}
	if len(meta.Services) == 0 {
		return fmt.Errorf("scenario %s must declare at least one compose service", meta.Key)
	}
	if len(meta.Ready) == 0 {
		return fmt.Errorf("scenario %s must declare at least one ready target", meta.Key)
	}
	return nil
}

func defaultScenarioTags(group, file, tier string) []string {
	name := strings.TrimSuffix(file, ".sh")
	tags := []string{group, tier}
	for _, part := range strings.Split(name, "-") {
		if part != "" {
			tags = append(tags, part)
		}
	}
	return normalizeStrings(tags)
}

func normalizeStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func (catalog scenarioCatalog) group(group string) []scenarioMeta {
	items := catalog.byGroup[group]
	out := make([]scenarioMeta, len(items))
	copy(out, items)
	return out
}

func (catalog scenarioCatalog) find(group, file string) (scenarioMeta, bool) {
	meta, ok := catalog.byKey[group+"/"+file]
	return meta, ok
}

func scenarioMatchesFilter(meta scenarioMeta, filter string) bool {
	if filter == "" {
		return true
	}
	for _, value := range append([]string{meta.File, meta.Key, meta.Group, meta.Tier, meta.Helper}, meta.Tags...) {
		if strings.Contains(value, filter) {
			return true
		}
	}
	return false
}

func readyTargetsForScenarios(def suiteDef, scenarios []scenarioMeta) []string {
	ready := orderedUnion(readyTargetOrder, collectScenarioValues(scenarios, func(meta scenarioMeta) []string {
		return meta.Ready
	}))
	if len(ready) == 0 {
		return def.Ready
	}
	return ready
}

func servicesForScenarios(scenarios []scenarioMeta) []string {
	return orderedUnion(composeServiceOrder, collectScenarioValues(scenarios, func(meta scenarioMeta) []string {
		return meta.Services
	}))
}

func collectScenarioValues(scenarios []scenarioMeta, values func(scenarioMeta) []string) []string {
	var out []string
	for _, scenario := range scenarios {
		out = append(out, values(scenario)...)
	}
	return out
}

func orderedUnion(order []string, values []string) []string {
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			seen[value] = true
		}
	}

	var out []string
	for _, value := range order {
		if seen[value] {
			out = append(out, value)
			delete(seen, value)
		}
	}

	var rest []string
	for value := range seen {
		rest = append(rest, value)
	}
	sort.Strings(rest)
	out = append(out, rest...)
	return out
}
