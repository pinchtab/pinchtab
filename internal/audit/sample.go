package audit

import (
	"net/url"
	"regexp"
	"sort"
)

// PageGroup is a set of URLs sharing a path template. Groups with a single
// member are ordinary pages; only groups of two or more are treated as
// template groups by SamplePages.
type PageGroup struct {
	// Template is the digit-normalized URL template shared by the members.
	Template string
	// URLs are the member URLs in first-occurrence order.
	URLs []string
}

var digitRuns = regexp.MustCompile(`\d+`)

// templateKey normalizes a URL to its path template: scheme, host, and path
// with digit runs collapsed to '#'. Query and fragment are ignored, so
// /products/p1.html and /products/p2.html share one template.
func templateKey(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return digitRuns.ReplaceAllString(raw, "#")
	}
	return u.Scheme + "://" + u.Host + digitRuns.ReplaceAllString(u.Path, "#")
}

// GroupURLs groups URLs by path template, deterministically: groups appear
// in first-occurrence order and members keep input order.
func GroupURLs(urls []string) []PageGroup {
	index := map[string]int{}
	var groups []PageGroup
	for _, u := range urls {
		key := templateKey(u)
		i, ok := index[key]
		if !ok {
			i = len(groups)
			index[key] = i
			groups = append(groups, PageGroup{Template: key})
		}
		groups[i].URLs = append(groups[i].URLs, u)
	}
	return groups
}

// SamplePages reduces urls to a bounded, prioritized audit plan:
//
//   - the entry URL (urls[0]) is always first
//   - ungrouped pages (unique templates) are all kept, in input order
//   - each template group (>= 2 members) contributes at most sampleSize
//     pages, picked in lexical order for stability; an entry URL belonging
//     to a group counts toward its quota
//   - template-group pages come after ungrouped pages (homepage and nav
//     pages ahead of deep template pages)
//
// sampleSize <= 0 keeps every page (prioritized ordering still applies).
// groups may be supplied externally (e.g. from a SeaPortal SiteReport); nil
// computes them locally from URL structure. Fully deterministic: identical
// inputs always yield the identical plan.
func SamplePages(urls []string, sampleSize int, groups []PageGroup) []string {
	if len(urls) == 0 {
		return nil
	}
	if groups == nil {
		groups = GroupURLs(urls)
	}

	groupOf := map[string]string{}
	for _, g := range groups {
		if len(g.URLs) < 2 {
			continue
		}
		for _, u := range g.URLs {
			groupOf[u] = g.Template
		}
	}

	entry := urls[0]
	plan := []string{entry}
	seen := map[string]bool{entry: true}
	picked := map[string]int{}
	if t, ok := groupOf[entry]; ok {
		picked[t] = 1
	}

	for _, u := range urls {
		if seen[u] || groupOf[u] != "" {
			continue
		}
		seen[u] = true
		plan = append(plan, u)
	}

	for _, g := range groups {
		if len(g.URLs) < 2 {
			continue
		}
		members := append([]string(nil), g.URLs...)
		sort.Strings(members)
		for _, u := range members {
			if sampleSize > 0 && picked[g.Template] >= sampleSize {
				break
			}
			if seen[u] {
				continue
			}
			seen[u] = true
			plan = append(plan, u)
			picked[g.Template]++
		}
	}
	return plan
}
