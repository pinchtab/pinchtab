package runtime

import (
	"fmt"
	"net/url"
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
	if len(rules) != 13 {
		t.Fatalf("rule count = %d", len(rules))
	}
	denies, allows, defaults := semanticDenyRules(rules), rulesAtPriority(rules, 2), rulesAtPriority(rules, 1)
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

func TestCompileTransactionPolicyConsolidatesHostsWithFailClosedDenyScope(t *testing.T) {
	policy := config.TransactionPolicyConfig{Enabled: true, Hosts: []string{"supplier.example", "other.example", "supplier.example"}, DenyRules: []config.TransactionPolicyRule{{Method: "*", PathPrefix: "/x"}}}
	manifest, rules, err := compileTransactionPolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 4 { // raw /x, one encoded URL filter, the encoded-unsafe guard, and the non-read default.
		t.Fatalf("multiple hosts multiplied rule count: %d", len(rules))
	}
	if len(manifest.HostPermissions) != 8 {
		t.Fatalf("exact host permissions = %v", manifest.HostPermissions)
	}
	for _, url := range []string{
		"https://supplier.example/x",
		"https://user:password@supplier.example:8443/x",
		"https://supplier.example./x",
		"https://other.example/x",
		"https://other.example./x",
	} {
		if !anyRuleMatches(rules, url) {
			t.Errorf("configured exact host did not match: %s", url)
		}
	}
	if !anyRuleMatches(rulesAtPriority(rules, 3), "https://sub.supplier.example/x") {
		t.Error("deny scope did not fail closed over a configured host subdomain")
	}
	if anyRuleMatches(rules, "https://supplier.example.evil/x") {
		t.Error("sibling domain matched")
	}
}

func TestCompileTransactionPolicyGroupsMethods(t *testing.T) {
	policy := config.TransactionPolicyConfig{Enabled: true, Hosts: []string{"supplier.example"}, DenyRules: []config.TransactionPolicyRule{
		{Method: "GET", PathPrefix: "/orders"},
		{Method: "POST", PathPrefix: "/orders"},
		{Method: "POST", PathPrefix: "/orders"},
		{Method: "GET", PathPrefix: "/cart"},
		{Method: "*", PathPrefix: "/cart"},
	}}
	_, rules, err := compileTransactionPolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	denies := semanticDenyRules(rules)
	if len(denies) != 13 {
		t.Fatalf("method-equivalent rules were not grouped: %d", len(denies))
	}
	var encodedRead, unsafeMutation, wildcard int
	for _, rule := range denies {
		switch got := strings.Join(rule.Condition.RequestMethods, ","); got {
		case "get":
			encodedRead++
		case "post":
			unsafeMutation++
		case "":
			wildcard++
		default:
			t.Errorf("unexpected grouped methods %q", got)
		}
	}
	if encodedRead != 7 || unsafeMutation != 1 || wildcard != 5 {
		t.Fatalf("grouped variants encoded-read=%d unsafe=%d wildcard=%d", encodedRead, unsafeMutation, wildcard)
	}
}

func TestDenyPathSegmentKeepsSegmentBoundaries(t *testing.T) {
	policy := config.TransactionPolicyConfig{Enabled: true, Hosts: []string{"supplier.example"}, DenyRules: []config.TransactionPolicyRule{{Method: "POST", PathPrefix: "/cart", PathSegment: "order"}}}
	_, rules, err := compileTransactionPolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	denies := semanticDenyRules(rules)
	if !anyRuleMatches(denies, "https://supplier.example/cart/123/order") {
		t.Fatal("path segment deny did not match /cart/123/order")
	}
	if anyRuleMatches(denies, "https://supplier.example/cart/preorder") {
		t.Fatal("path segment deny broadened to /cart/preorder")
	}
	for _, url := range []string{"https://supplier.example/cart%2F123/order", "https://supplier.example/cart/123/%6Frder"} {
		if !anyRuleMatches(rulesAtPriority(rules, 3), url) {
			t.Errorf("non-root encoded path-segment deny was dropped: %s", url)
		}
	}
}

func TestDenyPathSegmentRegexCoversArbitraryPercentEncoding(t *testing.T) {
	policy := config.TransactionPolicyConfig{Enabled: true, Hosts: []string{"supplier.example"}, DenyRules: []config.TransactionPolicyRule{{Method: "POST", PathPrefix: "/", PathSegment: "checkout"}}}
	_, rules, err := compileTransactionPolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	denies := semanticDenyRules(rules)
	if len(denies) != 1 {
		t.Fatalf("unsafe deny compiled to %d semantic rules", len(denies))
	}
	if rawURL := "https://supplier.example/a/checkout"; !anyRuleMatches(denies, rawURL) {
		t.Errorf("raw deny bypass: %s", rawURL)
	}
	for _, rawURL := range []string{
		"https://supplier.example/a/chec%6Bout",
		"https://supplier.example/a%2Fcheckout",
		"https://supplier.example./a/%63heckout",
		"https://supplier.example/a/%63%68%65%63%6B%6F%75%74",
		"https://supplier.example/a%2F%63h%65ckout",
		"https://supplier.example./a/%63%68eckout",
	} {
		if !anyRuleMatches(rulesAtPriority(rules, 3), rawURL) {
			t.Errorf("encoded deny bypass: %s", rawURL)
		}
	}
	guardedPOST := false
	for _, rule := range rulesAtPriority(rules, 3) {
		if strings.Contains(rule.Condition.RegexFilter, "%[0-9A-F]{2}") && strings.Contains(strings.Join(rule.Condition.RequestMethods, ","), "post") {
			guardedPOST = true
		}
	}
	if !guardedPOST {
		t.Fatal("encoded-unsafe guard does not cover POST")
	}
	for _, rawURL := range []string{
		"https://supplier.example.evil/a/checkout",
		"https://supplier.example/a/precheckout",
		"https://supplier.example/cart/preparation?return=%2Fcheckout",
	} {
		if anyRuleMatches(denies, rawURL) {
			t.Errorf("encoded deny broadened to %s", rawURL)
		}
	}
}

func TestDenyPathSegmentRegexGroupsMethods(t *testing.T) {
	policy := config.TransactionPolicyConfig{Enabled: true, Hosts: []string{"supplier.example"}, DenyRules: []config.TransactionPolicyRule{{Method: "GET", PathPrefix: "/", PathSegment: "pay"}, {Method: "POST", PathPrefix: "/", PathSegment: "pay"}, {Method: "*", PathPrefix: "/", PathSegment: "pay"}}}
	_, rules, err := compileTransactionPolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range semanticDenyRules(rules) {
		if got := strings.Join(rule.Condition.RequestMethods, ","); got != "" && got != "get" {
			t.Errorf("wildcard did not subsume specific methods: %q", got)
		}
	}
	policy.DenyRules = policy.DenyRules[:2]
	_, rules, err = compileTransactionPolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range semanticDenyRules(rules) {
		if got := strings.Join(rule.Condition.RequestMethods, ","); got != "get" && got != "post" {
			t.Errorf("regex methods = %q", got)
		}
	}
}

func TestTransactionPolicyRuleCountAccounting(t *testing.T) {
	regexRules := make([]dnrRule, maxTransactionPolicyRegexRules+1)
	for i := range regexRules {
		regexRules[i].Condition.RegexFilter = "x"
	}
	if err := validateTransactionPolicyRuleCounts(regexRules); err == nil || !strings.Contains(err.Error(), "static regex rules") {
		t.Fatalf("regex accounting error = %v", err)
	}
}

func TestCompileTransactionPolicyRejectsChromeRegexRuleLimit(t *testing.T) {
	policy := config.TransactionPolicyConfig{Enabled: true, Hosts: []string{"supplier.example"}}
	for i := 0; i < maxTransactionPolicyRegexRules; i++ {
		policy.DenyRules = append(policy.DenyRules, config.TransactionPolicyRule{Method: "*", PathPrefix: fmt.Sprintf("/r%d", i)})
	}
	_, _, err := compileTransactionPolicy(policy)
	if err == nil {
		t.Fatalf("policy exceeding Chrome's %d static regex-rule limit compiled", maxTransactionPolicyRegexRules)
	}
	if !strings.Contains(err.Error(), "static regex rules, Chrome maximum is 1000") {
		t.Fatalf("limit error = %v", err)
	}
}

func TestDenyRegexCoversChromeEncodedPathForms(t *testing.T) {
	_, rules, err := compileTransactionPolicy(testTransactionPolicy())
	if err != nil {
		t.Fatal(err)
	}
	denies := semanticDenyRules(rules)
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
	denies := semanticDenyRules(rules)
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

func semanticDenyRules(rules []dnrRule) []dnrRule {
	var matches []dnrRule
	for _, rule := range rulesAtPriority(rules, 3) {
		if !strings.Contains(rule.Condition.RegexFilter, "%[0-9A-F]{2}") {
			matches = append(matches, rule)
		}
	}
	return matches
}

func anyRuleMatches(rules []dnrRule, rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		panic(err)
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	for _, rule := range rules {
		domainMatched := len(rule.Condition.RequestDomains) == 0
		for _, domain := range rule.Condition.RequestDomains {
			domain = strings.ToLower(domain)
			if host == domain || strings.HasSuffix(host, "."+domain) {
				domainMatched = true
			}
		}
		if !domainMatched {
			continue
		}
		if rule.Condition.RegexFilter != "" && regexp.MustCompile("(?i)"+rule.Condition.RegexFilter).MatchString(rawURL) {
			return true
		}
		if rule.Condition.URLFilter == "" {
			continue
		}
		needle := strings.Trim(strings.ToLower(rule.Condition.URLFilter), "*")
		if strings.Contains(strings.ToLower(rawURL), needle) {
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
