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
	// Chrome accepts at most 1,000 regex rules in one static ruleset. Exceeding
	// that limit makes Chrome ignore later rules, so reject instead of launching
	// a partially enforced transaction policy.
	maxTransactionPolicyRegexRules = 1000
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
	RegexFilter              string   `json:"regexFilter,omitempty"`
	URLFilter                string   `json:"urlFilter,omitempty"`
	IsURLFilterCaseSensitive bool     `json:"isUrlFilterCaseSensitive"`
	RequestDomains           []string `json:"requestDomains,omitempty"`
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
	rules := make([]dnrRule, 0, len(policy.DenyRules)+len(policy.AllowRules)+1)
	id := 1
	appendRules := func(source []config.TransactionPolicyRule, priority int, action string, encodePath bool) error {
		type group struct {
			condition dnrCondition
			wildcard  bool
			methods   map[string]struct{}
		}
		groups := make([]group, 0, len(source))
		byCondition := make(map[string]int, len(source))
		domainScoped := action == "block"
		hostPatterns := []string{"[^/?#]+"}
		if !domainScoped {
			hostPatterns = make([]string, len(hosts))
			for i, host := range hosts {
				hostPatterns[i] = regexp.QuoteMeta(host)
			}
		}
		for _, rule := range source {
			method := strings.ToLower(strings.TrimSpace(rule.Method))
			for _, pattern := range hostPatterns {
				conditions, err := transactionRuleConditions(pattern, hosts, rule, encodePath, domainScoped)
				if err != nil {
					return err
				}
				for _, condition := range conditions {
					key := condition.RegexFilter + "\x00" + condition.URLFilter + "\x00" + strings.Join(condition.RequestDomains, "\x00")
					index, ok := byCondition[key]
					if !ok {
						index = len(groups)
						byCondition[key] = index
						groups = append(groups, group{condition: condition, methods: make(map[string]struct{})})
					}
					if method == "*" {
						groups[index].wildcard = true
					} else {
						groups[index].methods[method] = struct{}{}
					}
				}
			}
		}
		for _, group := range groups {
			condition := group.condition
			if !group.wildcard {
				condition.RequestMethods = make([]string, 0, len(group.methods))
				for method := range group.methods {
					condition.RequestMethods = append(condition.RequestMethods, method)
				}
				sort.Strings(condition.RequestMethods)
			}
			rules = append(rules, dnrRule{ID: id, Priority: priority, Action: dnrAction{Type: action}, Condition: condition})
			id++
		}
		return nil
	}
	// Unsafe requests with percent escapes are ambiguous at the DNR/raw-URL
	// boundary. Block them before explicit allows; callers can use canonical raw
	// unreserved paths/queries and put arbitrary data in the request body.
	encodedUnsafeRegex := "^(https?|wss?)://([^/?#@]*@)?[^/?#]+(:[0-9]+)?[^#]*%[0-9A-F]{2}"
	if err := validateDNRRegex(encodedUnsafeRegex); err != nil {
		return transactionPolicyManifest{}, nil, err
	}
	rules = append(rules, dnrRule{ID: id, Priority: 3, Action: dnrAction{Type: "block"}, Condition: dnrCondition{RegexFilter: encodedUnsafeRegex, IsURLFilterCaseSensitive: false, RequestDomains: append([]string(nil), hosts...), RequestMethods: []string{"connect", "delete", "other", "patch", "post", "put"}}})
	id++
	// Denies use an encoding-tolerant path representation. This only broadens a
	// block, never an allow, so URL serialization ambiguities fail closed.
	if err := appendRules(policy.DenyRules, 3, "block", true); err != nil {
		return transactionPolicyManifest{}, nil, err
	}
	if err := appendRules(policy.AllowRules, 2, "allow", false); err != nil {
		return transactionPolicyManifest{}, nil, err
	}
	regex := "^(https?|wss?)://([^/?#@]*@)?[^/?#]+(:[0-9]+)?(/|$)"
	if err := validateDNRRegex(regex); err != nil {
		return transactionPolicyManifest{}, nil, err
	}
	rules = append(rules, dnrRule{ID: id, Priority: 1, Action: dnrAction{Type: "block"}, Condition: dnrCondition{RegexFilter: regex, IsURLFilterCaseSensitive: false, RequestDomains: append([]string(nil), hosts...), ExcludedRequestMethods: []string{"get", "head", "options"}}})
	if err := validateTransactionPolicyRuleCounts(rules); err != nil {
		return transactionPolicyManifest{}, nil, err
	}
	return manifest, rules, nil
}

func validateTransactionPolicyRuleCounts(rules []dnrRule) error {
	regexRules := 0
	for _, rule := range rules {
		if rule.Condition.RegexFilter != "" {
			regexRules++
		}
	}
	if regexRules > maxTransactionPolicyRegexRules {
		return fmt.Errorf("policy produces %d static regex rules, Chrome maximum is %d", regexRules, maxTransactionPolicyRegexRules)
	}
	return nil
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

func transactionRuleConditions(hostPattern string, hosts []string, rule config.TransactionPolicyRule, encodePath bool, domainScoped bool) ([]dnrCondition, error) {
	method := strings.ToUpper(strings.TrimSpace(rule.Method))
	guardedUnsafeMethod := method == "CONNECT" || method == "DELETE" || method == "OTHER" || method == "PATCH" || method == "POST" || method == "PUT"
	rootSegment := strings.TrimSpace(rule.PathPrefix) == "/" && strings.TrimSpace(rule.PathSegment) != "" && strings.TrimSpace(rule.QueryParam) == ""

	regexEncodePath := encodePath && !guardedUnsafeMethod && !(method == "*" && rootSegment)
	regexes, err := transactionRuleRegexes(hostPattern, rule, regexEncodePath)
	if err != nil {
		return nil, err
	}
	conditions := make([]dnrCondition, 0, len(regexes))
	for _, regex := range regexes {
		if err := validateDNRRegex(regex); err != nil {
			return nil, err
		}
		condition := dnrCondition{RegexFilter: regex, IsURLFilterCaseSensitive: false}
		if domainScoped {
			condition.RequestDomains = append([]string(nil), hosts...)
		}
		conditions = append(conditions, condition)
	}
	if encodePath && method == "*" && rootSegment {
		for _, filter := range transactionRuleEncodedSegmentURLFilters(rule) {
			conditions = append(conditions, dnrCondition{
				URLFilter:                filter,
				IsURLFilterCaseSensitive: false,
				RequestDomains:           append([]string(nil), hosts...),
			})
		}
	}
	return conditions, nil
}

func transactionRuleEncodedSegmentURLFilters(rule config.TransactionPolicyRule) []string {
	segment := strings.TrimSpace(rule.PathSegment)
	filters := make([]string, 0, len(segment))
	for position := range len(segment) {
		var b strings.Builder
		b.WriteString("*")
		for index := range len(segment) {
			if index == position {
				fmt.Fprintf(&b, "%%%02X", segment[index])
			} else {
				b.WriteByte(segment[index])
			}
		}
		b.WriteString("*")
		filters = append(filters, b.String())
	}
	return filters
}

// transactionRuleRegexes describes the raw network URL. Denies get the raw
// spelling plus one variant for each single percent-encoded unreserved byte.
// transactionRuleConditions moves only root-segment wildcard variants into
// block-only URL filters and relies on the unsafe percent guard for explicit
// mutation methods, keeping the static regex ruleset below Chrome's hard cap.
func transactionRuleRegexes(hostPattern string, rule config.TransactionPolicyRule, encodePath bool) ([]string, error) {
	base, positions, err := transactionRuleRegexVariant(hostPattern, rule, encodePath, -1)
	if err != nil {
		return nil, err
	}
	regexes := []string{base}
	if !encodePath {
		return regexes, nil
	}
	for position := 0; position < positions; position++ {
		variant, _, err := transactionRuleRegexVariant(hostPattern, rule, true, position)
		if err != nil {
			return nil, err
		}
		regexes = append(regexes, variant)
	}
	return regexes, nil
}

func transactionRuleRegexVariant(hostPattern string, rule config.TransactionPolicyRule, encodePath bool, encodeAt int) (string, int, error) {
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
	regex := "^(https?|wss?)://([^/?#@]*@)?" + hostPattern + "(\\.)?(:[0-9]+)?" + pathPart + query
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
