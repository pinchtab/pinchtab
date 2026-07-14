package runtime

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func testTransactionPolicy() config.TransactionPolicyConfig {
	return config.TransactionPolicyConfig{Enabled: true, Hosts: []string{"supplier.example"}, DenyRules: []config.TransactionPolicyRule{{Method: "*", PathPrefix: "/checkout"}}, AllowRules: []config.TransactionPolicyRule{{Method: "POST", PathPrefix: "/cart"}, {Method: "POST", PathPrefix: "/", QueryParam: "wc-ajax", QueryValue: "add_to_cart"}}}
}

func TestCompileTransactionPolicyPriorityMethodsAndTrailingDot(t *testing.T) {
	manifest, rules, err := compileTransactionPolicy(testTransactionPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ManifestVersion != 3 || len(manifest.HostPermissions) != 4 {
		t.Fatalf("manifest = %#v", manifest)
	}
	if len(rules) != 12 {
		t.Fatalf("rule count = %d", len(rules))
	}
	denies, allows, defaults := rulesAtPriority(rules, 3), rulesAtPriority(rules, 2), rulesAtPriority(rules, 1)
	if len(denies) != 9 || len(allows) != 2 || len(defaults) != 1 || denies[0].Action.Type != "block" || allows[0].Action.Type != "allow" {
		t.Fatalf("priorities/actions = %#v", rules)
	}
	if got := strings.Join(defaults[0].Condition.ExcludedRequestMethods, ","); got != "get,head,options" {
		t.Fatalf("default methods = %v", got)
	}
	for _, group := range [][]dnrRule{denies, defaults} {
		if !anyRuleMatches(group, "https://supplier.example./checkout") {
			t.Fatalf("trailing-dot URL bypasses priority %d rules", group[0].Priority)
		}
	}
}

func TestDenyRegexCoversChromeEncodedPathForms(t *testing.T) {
	_, rules, err := compileTransactionPolicy(testTransactionPolicy())
	if err != nil {
		t.Fatal(err)
	}
	denies := rulesAtPriority(rules, 3)
	for _, url := range []string{"https://supplier.example/checkout", "https://user:password@supplier.example/checkout", "https://supplier.example/chec%6bout", "https://supplier.example/%2Fcheckout", "https://supplier.example./chec%6bout"} {
		if !anyRuleMatches(denies, url) {
			t.Errorf("encoded deny bypass: %s", url)
		}
	}
	allows := rulesAtPriority(rules, 2)
	allow := regexp.MustCompile(allows[1].Condition.RegexFilter)
	for _, url := range []string{"https://supplier.example/?wc-ajax=add_to_cart", "https://supplier.example/?wc-ajax=add_to_cart#cart"} {
		if !allow.MatchString(url) {
			t.Errorf("exact query allow did not match %s", url)
		}
	}
	for _, url := range []string{"https://supplier.example/?x=1&wc-ajax=add_to_cart", "https://supplier.example/?wc-ajax=add_to_cart&x=1", "https://supplier.example/?wc-ajax=add_to_cart&wc-ajax=add_to_cart", "https://supplier.example/?wc-ajax=add_to_cart&wc-ajax=checkout", "https://supplier.example/?action=checkout&wc-ajax=add_to_cart", "https://supplier.example/?wc-ajax="} {
		if allow.MatchString(url) {
			t.Errorf("polluted query allowed: %s", url)
		}
	}
}

func TestDenyQueryMatchesExactPairWithoutBroadeningPath(t *testing.T) {
	policy := testTransactionPolicy()
	policy.DenyRules = []config.TransactionPolicyRule{{Method: "*", PathPrefix: "/", QueryParam: "action", QueryValue: "checkout"}}
	_, rules, err := compileTransactionPolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	denies := rulesAtPriority(rules, 3)
	for _, url := range []string{
		"https://supplier.example/?action=checkout",
		"https://supplier.example/?x=1&action=checkout",
		"https://supplier.example/?action=checkout&x=1",
		"https://supplier.example/?action=cancel&action=checkout",
		"https://supplier.example/?%61ction=checkout",
		"https://supplier.example/?action=%63heckout",
		"wss://supplier.example/?action=checkout",
	} {
		if !anyRuleMatches(denies, url) {
			t.Errorf("deny query bypass: %s", url)
		}
	}
	for _, url := range []string{
		"https://supplier.example/",
		"https://supplier.example/?action=cart",
		"https://supplier.example/?action=checkout-now",
		"https://supplier.example/?xaction=checkout",
		"https://supplier.example/?return=action%3Dcheckout",
	} {
		if anyRuleMatches(denies, url) {
			t.Errorf("deny query broadened to unrelated URL: %s", url)
		}
	}
}

func rulesAtPriority(rules []dnrRule, priority int) []dnrRule {
	var matches []dnrRule
	for _, rule := range rules {
		if rule.Priority == priority {
			matches = append(matches, rule)
		}
	}
	return matches
}

func anyRuleMatches(rules []dnrRule, url string) bool {
	for _, rule := range rules {
		if regexp.MustCompile("(?i)" + rule.Condition.RegexFilter).MatchString(url) {
			return true
		}
	}
	return false
}

func TestPrepareTransactionPolicyExtensionIsPrivateAndGeneratedOnly(t *testing.T) {
	stateDir := t.TempDir()
	if err := os.Chmod(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.RuntimeConfig{StateDir: stateDir, ChromeBinary: "/usr/bin/chromium", TransactionPolicy: testTransactionPolicy()}
	launch, err := PrepareTransactionPolicyExtension(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.ExtensionPaths) != 0 || len(launch.ExtensionPaths) != 1 {
		t.Fatalf("operator=%v launch=%v", cfg.ExtensionPaths, launch.ExtensionPaths)
	}
	if got := filepath.Dir(launch.ExtensionPaths[0]); got != filepath.Join(stateDir, transactionPolicyStateRoot) {
		t.Fatalf("generation escaped policy root: %q", got)
	}
	if info, err := os.Stat(stateDir); err != nil || info.Mode().Perm() != 0755 {
		t.Fatalf("StateDir permissions changed: %v %v", info, err)
	}
	for _, name := range []string{"manifest.json", "rules.json", "background.js"} {
		info, err := os.Stat(filepath.Join(launch.ExtensionPaths[0], name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatalf("%s permissions = %o", name, info.Mode().Perm())
		}
	}
	info, err := os.Stat(filepath.Dir(launch.ExtensionPaths[0]))
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("policy root mode = %v, %v", info.Mode(), err)
	}
	if err := validateTransactionPolicyLaunch(launch); err != nil {
		t.Fatalf("generated launch rejected: %v", err)
	}
	if err := os.WriteFile(filepath.Join(launch.ExtensionPaths[0], "rules.json"), []byte("[]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateTransactionPolicyLaunch(launch); err == nil {
		t.Fatal("tampered generated policy passed launch validation")
	}
}

func TestPrepareTransactionPolicyExtensionRejectsExtensionsAndUnsafeState(t *testing.T) {
	cfg := &config.RuntimeConfig{StateDir: t.TempDir(), ChromeBinary: "/usr/bin/chromium", ExtensionPaths: []string{"/operator-extension"}, TransactionPolicy: testTransactionPolicy()}
	if _, err := PrepareTransactionPolicyExtension(cfg); err == nil {
		t.Fatal("operator extension accepted in policy mode")
	}
	state := t.TempDir()
	link := filepath.Join(t.TempDir(), "state-link")
	if err := os.Symlink(state, link); err != nil {
		t.Fatal(err)
	}
	cfg = &config.RuntimeConfig{StateDir: link, ChromeBinary: "/usr/bin/chromium", TransactionPolicy: testTransactionPolicy()}
	if _, err := PrepareTransactionPolicyExtension(cfg); err == nil {
		t.Fatal("symlink StateDir accepted")
	}
	state = t.TempDir()
	if err := os.Symlink(t.TempDir(), filepath.Join(state, transactionPolicyStateRoot)); err != nil {
		t.Fatal(err)
	}
	cfg = &config.RuntimeConfig{StateDir: state, ChromeBinary: "/usr/bin/chromium", TransactionPolicy: testTransactionPolicy()}
	if _, err := PrepareTransactionPolicyExtension(cfg); err == nil {
		t.Fatal("symlink policy root accepted")
	}
}

func TestTransactionPolicyGenerationsAreDeterministicAndBounded(t *testing.T) {
	state := t.TempDir()
	cfg := &config.RuntimeConfig{StateDir: state, ChromeBinary: "/usr/bin/chromium", TransactionPolicy: testTransactionPolicy()}
	first, err := PrepareTransactionPolicyExtension(cfg)
	if err != nil {
		t.Fatal(err)
	}
	second, err := PrepareTransactionPolicyExtension(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if first.ExtensionPaths[0] != second.ExtensionPaths[0] {
		t.Fatalf("same content generation changed: %q != %q", first.ExtensionPaths[0], second.ExtensionPaths[0])
	}
	for i := 0; i < maxTransactionPolicyGenerations+2; i++ {
		cfg.TransactionPolicy.DenyRules = []config.TransactionPolicyRule{{Method: "*", PathPrefix: "/checkout", PathSegment: strings.Repeat("x", i+1)}}
		if _, err := PrepareTransactionPolicyExtension(cfg); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(state, transactionPolicyStateRoot))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if transactionPolicyGenerationName.MatchString(entry.Name()) {
			count++
		}
	}
	if count > maxTransactionPolicyGenerations {
		t.Fatalf("generations not retained: %d", count)
	}
}

func TestExactTransactionPolicyRulesetState(t *testing.T) {
	if !hasExactTransactionPolicyRulesets([]string{transactionPolicyRulesetID}) {
		t.Fatal("exact generated ruleset was rejected")
	}
	for _, ids := range [][]string{nil, {"other"}, {transactionPolicyRulesetID, "other"}, {"prefix-" + transactionPolicyRulesetID}} {
		if hasExactTransactionPolicyRulesets(ids) {
			t.Fatalf("non-exact ruleset state accepted: %v", ids)
		}
	}
}

func TestValidateTransactionPolicyLaunchRejectsSpoofedPathAndFallback(t *testing.T) {
	cfg := &config.RuntimeConfig{StateDir: t.TempDir(), TransactionPolicy: testTransactionPolicy()}
	if err := validateTransactionPolicyLaunch(cfg); err == nil {
		t.Fatal("missing binary accepted")
	}
	cfg.ChromeBinary = "/Applications/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing"
	cfg.ExtensionPaths = []string{filepath.Join(cfg.StateDir, "transaction-policy-extension-spoof")}
	if err := validateTransactionPolicyLaunch(cfg); err == nil || strings.Contains(err.Error(), "Google Chrome Stable") {
		t.Fatalf("Chrome for Testing was classified as Stable: %v", err)
	}
	cfg.ChromeBinary = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	if err := validateTransactionPolicyLaunch(cfg); err == nil || !strings.Contains(err.Error(), "Google Chrome Stable") {
		t.Fatalf("Google Chrome Stable was accepted or misclassified: %v", err)
	}
	cfg.ChromeBinary = "/usr/bin/chromium"
	cfg.ExtensionPaths = []string{filepath.Join(cfg.StateDir, "transaction-policy-extension-spoof")}
	if err := validateTransactionPolicyLaunch(cfg); err == nil {
		t.Fatal("spoofed generated path accepted")
	}
	if _, _, _, err := startChromeWithRemoteAllocator(t.Context(), cfg, nil, 9222, ""); err == nil {
		t.Fatal("direct fallback bypassed validation")
	}
}
