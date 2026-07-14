package runtime

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/pinchtab/pinchtab/internal/config"
)

const (
	transactionPolicyRulesetID   = "pinchtab_transaction_policy"
	transactionPolicyExtensionID = "amadgaedoaaekpjejecafmbdhacgmlig"
	transactionPolicyManifestKey = "MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAuinfEBuVQwc6FKF/tVRTn6rfITooe1jQaIMYk/rTRwYg4Pe1GqvFafjT7ghbL58Tjf55M+VhVhvEMSIzCPzjRfp7m8cUgm8/Qz7b86DRkyBzz+ovEMvtZLtN8f8xLIaR1dWVt++Lti1QOMBDNB6DVdGIDGUJm5xXVWhQTh1pRRBY2Fcbg5BgH4SG8/VAOXbVnXuXvniKf1skZlO3lUZeTVBlF8Rly4Drgep/B5wQEVhIiiomm+LV6saes6nKHbMysFlgfAOxL7wiEt6oqtFvGfh+QiVe8pazpA1N6Xf8Q47ljG2oTEtdGaPouHdOcnNwC0WtZ8p8LfhL7zqVqpCkpQIDAQAB"
	maxTransactionPolicyRules    = 25000
)

type transactionPolicyManifest struct {
	ManifestVersion       int           `json:"manifest_version"`
	Name                  string        `json:"name"`
	Version               string        `json:"version"`
	Key                   string        `json:"key"`
	Permissions           []string      `json:"permissions"`
	HostPermissions       []string      `json:"host_permissions"`
	DeclarativeNetRequest dnrSpec       `json:"declarative_net_request"`
	Background            dnrBackground `json:"background"`
}

type dnrBackground struct {
	ServiceWorker string `json:"service_worker"`
}
type dnrSpec struct {
	RuleResources []dnrResource `json:"rule_resources"`
}
type dnrResource struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
	Path    string `json:"path"`
}
type dnrRule struct {
	ID        int          `json:"id"`
	Priority  int          `json:"priority"`
	Action    dnrAction    `json:"action"`
	Condition dnrCondition `json:"condition"`
}
type dnrAction struct {
	Type string `json:"type"`
}
type dnrCondition struct {
	RegexFilter              string   `json:"regexFilter"`
	IsURLFilterCaseSensitive bool     `json:"isUrlFilterCaseSensitive"`
	RequestMethods           []string `json:"requestMethods,omitempty"`
	ExcludedRequestMethods   []string `json:"excludedRequestMethods,omitempty"`
}

// PrepareTransactionPolicyExtension compiles the configured policy into an
// unpacked MV3 extension. Policy mode deliberately loads no operator supplied
// extensions: an additional extension could spoof activation or change DNR.
func PrepareTransactionPolicyExtension(cfg *config.RuntimeConfig) (*config.RuntimeConfig, error) {
	if cfg == nil || !cfg.TransactionPolicy.Enabled {
		return cfg, nil
	}
	if strings.TrimSpace(cfg.StateDir) == "" {
		return nil, fmt.Errorf("transaction policy requires server.stateDir")
	}
	if len(cfg.ExtensionPaths) != 0 {
		return nil, fmt.Errorf("transaction policy cannot be used with browser.extensionPaths")
	}
	manifest, rules, err := compileTransactionPolicy(cfg.TransactionPolicy)
	if err != nil {
		return nil, fmt.Errorf("compile transaction policy: %w", err)
	}
	path, err := writeTransactionPolicyExtension(cfg.StateDir, manifest, rules)
	if err != nil {
		return nil, fmt.Errorf("write transaction policy extension: %w", err)
	}
	launch := *cfg
	launch.ExtensionPaths = []string{path}
	return &launch, nil
}

func compileTransactionPolicy(policy config.TransactionPolicyConfig) (transactionPolicyManifest, []dnrRule, error) {
	if !policy.Enabled {
		return transactionPolicyManifest{}, nil, nil
	}
	if errs := config.ValidateFileConfig(&config.FileConfig{Security: config.SecurityConfig{TransactionPolicy: policy}}); len(errs) != 0 {
		return transactionPolicyManifest{}, nil, fmt.Errorf("invalid policy: %v", errs[0])
	}
	hosts := append([]string(nil), policy.Hosts...)
	for i := range hosts {
		hosts[i] = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(hosts[i]), "."))
	}
	sort.Strings(hosts)
	hosts = compactStrings(hosts)
	manifest := transactionPolicyManifest{ManifestVersion: 3, Name: "PinchTab Transaction Policy", Version: "1.0", Key: transactionPolicyManifestKey, Permissions: []string{"declarativeNetRequest"}, HostPermissions: transactionHostPermissions(hosts), DeclarativeNetRequest: dnrSpec{RuleResources: []dnrResource{{ID: transactionPolicyRulesetID, Enabled: true, Path: "rules.json"}}}, Background: dnrBackground{ServiceWorker: "background.js"}}
	rules := make([]dnrRule, 0, len(hosts)*(len(policy.DenyRules)+len(policy.AllowRules)+1))
	id := 1
	appendRules := func(source []config.TransactionPolicyRule, priority int, action string, encodePath bool) error {
		for _, rule := range source {
			for _, host := range hosts {
				regexes, err := transactionRuleRegexes(host, rule, encodePath)
				if err != nil {
					return err
				}
				for _, regex := range regexes {
					if err := validateDNRRegex(regex); err != nil {
						return err
					}
					condition := dnrCondition{RegexFilter: regex, IsURLFilterCaseSensitive: false}
					if method := strings.ToLower(strings.TrimSpace(rule.Method)); method != "*" {
						condition.RequestMethods = []string{method}
					}
					rules = append(rules, dnrRule{ID: id, Priority: priority, Action: dnrAction{Type: action}, Condition: condition})
					id++
				}
			}
		}
		return nil
	}
	// Denies use an encoding-tolerant path representation. This only broadens a
	// block, never an allow, so URL serialization ambiguities fail closed.
	if err := appendRules(policy.DenyRules, 3, "block", true); err != nil {
		return transactionPolicyManifest{}, nil, err
	}
	if err := appendRules(policy.AllowRules, 2, "allow", false); err != nil {
		return transactionPolicyManifest{}, nil, err
	}
	for _, host := range hosts {
		regex := "^(https?|wss?)://([^/?#@]*@)?" + regexp.QuoteMeta(host) + "(\\.)?(:[0-9]+)?(/|$)"
		if err := validateDNRRegex(regex); err != nil {
			return transactionPolicyManifest{}, nil, err
		}
		rules = append(rules, dnrRule{ID: id, Priority: 1, Action: dnrAction{Type: "block"}, Condition: dnrCondition{RegexFilter: regex, IsURLFilterCaseSensitive: false, ExcludedRequestMethods: []string{"get", "head", "options"}}})
		id++
	}
	if len(rules) > maxTransactionPolicyRules {
		return transactionPolicyManifest{}, nil, fmt.Errorf("policy produces %d rules, maximum is %d", len(rules), maxTransactionPolicyRules)
	}
	return manifest, rules, nil
}

func validateDNRRegex(regex string) error {
	if len(regex) > 2000 {
		return fmt.Errorf("DNR regex exceeds the 2000 byte limit")
	}
	if _, err := regexp.Compile(regex); err != nil {
		return fmt.Errorf("invalid DNR regex: %w", err)
	}
	return nil
}
func transactionHostPermissions(hosts []string) []string {
	permissions := make([]string, 0, len(hosts)*4)
	for _, host := range hosts {
		for _, name := range []string{host, host + "."} {
			permissions = append(permissions, "http://"+name+"/*", "https://"+name+"/*")
		}
	}
	return permissions
}

// transactionRuleRegexes describes the raw network URL. Denies get one raw
// regex plus one variant for each single percent-encoded unreserved byte.
// Separate simple regexes stay within Chrome DNR's RE2 memory limit; a single
// regex with an alternation at every byte is rejected for ordinary route names.
func transactionRuleRegexes(host string, rule config.TransactionPolicyRule, encodePath bool) ([]string, error) {
	base, positions, err := transactionRuleRegexVariant(host, rule, encodePath, -1)
	if err != nil {
		return nil, err
	}
	regexes := []string{base}
	if !encodePath {
		return regexes, nil
	}
	for position := 0; position < positions; position++ {
		variant, _, err := transactionRuleRegexVariant(host, rule, true, position)
		if err != nil {
			return nil, err
		}
		regexes = append(regexes, variant)
	}
	return regexes, nil
}

func transactionRuleRegexVariant(host string, rule config.TransactionPolicyRule, encodePath bool, encodeAt int) (string, int, error) {
	prefix := strings.TrimSuffix(strings.TrimSpace(rule.PathPrefix), "/")
	if prefix == "" {
		prefix = "/"
	}
	segment := strings.TrimSpace(rule.PathSegment)
	position := 0
	literal := func(value string) string {
		if !encodePath {
			return regexp.QuoteMeta(value)
		}
		var b strings.Builder
		for i := 0; i < len(value); i++ {
			c := value[i]
			if c == '/' {
				// A run covers the serialized root slash followed by an encoded
				// separator without adding per-byte regex alternations.
				b.WriteString("(/|%2F)+")
				continue
			}
			if position == encodeAt {
				fmt.Fprintf(&b, "%%%02X", c)
			} else {
				b.WriteString(regexp.QuoteMeta(string(c)))
			}
			position++
		}
		return b.String()
	}
	separator := "/"
	if encodePath {
		separator = "(/|%2F)"
	}
	var pathPart string
	switch {
	case segment == "" && prefix == "/":
		pathPart = "/[^?]*"
	case segment == "":
		pathPart = literal(prefix) + "(" + separator + "[^?]*)?"
	case prefix == "/":
		pathPart = separator + "([^?]*" + separator + ")?" + literal(segment) + "(" + separator + "[^?]*)?"
	case pathPrefixHasSegment(prefix, segment):
		pathPart = literal(prefix) + "(" + separator + "[^?]*)?"
	default:
		pathPart = literal(prefix) + "(" + separator + "[^?]*)?" + separator + literal(segment) + "(" + separator + "[^?]*)?"
	}
	query := "(\\?[^#]*)?(#.*)?$"
	if rule.QueryParam != "" {
		if encodePath {
			// Denies match the configured pair anywhere in the query. Extra or
			// conflicting parameters cannot turn a forbidden action into an allow.
			pair := literal(rule.QueryParam) + "=" + literal(rule.QueryValue)
			query = "\\?([^#&]*&)*" + pair + "(&[^#]*)?(#.*)?$"
		} else {
			// Allows must have exactly this whole query. A query condition is not a
			// permission to add another action, duplicate, or empty value.
			query = "\\?" + regexp.QuoteMeta(rule.QueryParam) + "=" + regexp.QuoteMeta(rule.QueryValue) + "(#.*)?$"
		}
	}
	regex := "^(https?|wss?)://([^/?#@]*@)?" + regexp.QuoteMeta(host) + "(\\.)?(:[0-9]+)?" + pathPart + query
	return regex, position, nil
}
func pathPrefixHasSegment(prefix, segment string) bool {
	for _, part := range strings.Split(strings.Trim(prefix, "/"), "/") {
		if strings.EqualFold(part, segment) {
			return true
		}
	}
	return false
}
func compactStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	out := values[:1]
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}
