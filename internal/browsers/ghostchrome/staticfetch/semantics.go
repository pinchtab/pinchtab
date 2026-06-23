package staticfetch

import (
	"strings"

	"github.com/gost-dom/browser/dom"
	"github.com/gost-dom/browser/html"
)

func (l *Browser) getTitle(win html.Window) string {
	if win == nil {
		return ""
	}
	doc := win.Document()
	if doc == nil {
		return ""
	}
	titleEl, err := doc.QuerySelector("title")
	if err != nil || titleEl == nil {
		return ""
	}
	return strings.TrimSpace(titleEl.TextContent())
}

func getRole(el dom.Element) string {
	if role, ok := el.GetAttribute("role"); ok {
		return role
	}

	switch strings.ToLower(el.TagName()) {
	case "a":
		if _, has := el.GetAttribute("href"); has {
			return "link"
		}
	case "button":
		return "button"
	case "input":
		t, _ := el.GetAttribute("type")
		switch t {
		case "submit", "button":
			return "button"
		case "checkbox":
			return "checkbox"
		case "radio":
			return "radio"
		default:
			return "textbox"
		}
	case "textarea":
		return "textbox"
	case "select":
		return "combobox"
	case "img":
		return "img"
	case "nav":
		return "navigation"
	case "main":
		return "main"
	case "header":
		return "banner"
	case "footer":
		return "contentinfo"
	case "aside":
		return "complementary"
	case "form":
		return "form"
	case "h1", "h2", "h3", "h4", "h5", "h6":
		return "heading"
	case "ul", "ol":
		return "list"
	case "li":
		return "listitem"
	case "table":
		return "table"
	case "tr":
		return "row"
	case "td":
		return "cell"
	case "th":
		return "columnheader"
	case "section":
		if _, has := el.GetAttribute("aria-label"); has {
			return "region"
		}
		if _, has := el.GetAttribute("aria-labelledby"); has {
			return "region"
		}
	case "details":
		return "group"
	case "summary":
		return "button"
	case "dialog":
		return "dialog"
	case "article":
		return "article"
	case "p", "div", "span":
		return "generic"
	}
	return "generic"
}

func getAccessibleName(el dom.Element) string {
	if label, ok := el.GetAttribute("aria-label"); ok {
		return label
	}
	if title, ok := el.GetAttribute("title"); ok {
		return title
	}
	tag := strings.ToLower(el.TagName())
	if tag == "img" {
		if alt, ok := el.GetAttribute("alt"); ok {
			return alt
		}
	}
	if tag == "input" || tag == "textarea" {
		if ph, ok := el.GetAttribute("placeholder"); ok {
			return ph
		}
	}
	if isInteractive(el) {
		text := strings.TrimSpace(el.TextContent())
		if len(text) > 100 {
			text = text[:100] + "..."
		}
		return text
	}
	return ""
}

func isInteractive(el dom.Element) bool {
	switch strings.ToLower(el.TagName()) {
	case "a":
		_, has := el.GetAttribute("href")
		return has
	case "button", "input", "textarea", "select", "summary":
		return true
	}
	if _, ok := el.GetAttribute("onclick"); ok {
		return true
	}
	if idx, ok := el.GetAttribute("tabindex"); ok && idx != "-1" {
		return true
	}
	if role, ok := el.GetAttribute("role"); ok {
		switch role {
		case "button", "link", "tab", "menuitem", "switch", "checkbox", "radio":
			return true
		}
	}
	return false
}

// normalizeWhitespace collapses runs of whitespace (including blank lines)
// into single spaces while trimming leading/trailing space.
func normalizeWhitespace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prev := true // treat start as whitespace
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !prev {
				b.WriteByte(' ')
				prev = true
			}
			continue
		}
		b.WriteRune(r)
		prev = false
	}
	return strings.TrimSpace(b.String())
}
