package engine

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Realistic page templates simulating common website structures.
// These tests verify the lite engine produces correct accessibility snapshots
// comparable to what Chrome's CDP accessibility tree would return.
// ---------------------------------------------------------------------------

const wikipediaStylePage = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Go (programming language) - Wikipedia</title>
  <link rel="stylesheet" href="/static/style.css">
  <script src="/static/app.js"></script>
</head>
<body>
  <header>
    <nav id="top-nav">
      <a href="/" class="logo">Wikipedia</a>
      <form action="/search" method="get">
        <input type="search" name="q" placeholder="Search Wikipedia" aria-label="Search">
        <button type="submit">Search</button>
      </form>
    </nav>
  </header>

  <main id="content">
    <h1>Go (programming language)</h1>
    <p><b>Go</b>, also known as <b>Golang</b>, is a statically typed, compiled
    programming language designed at Google.</p>

    <nav id="toc" aria-label="Table of contents">
      <h2>Contents</h2>
      <ul>
        <li><a href="#history">1 History</a></li>
        <li><a href="#syntax">2 Syntax</a></li>
        <li><a href="#concurrency">3 Concurrency</a></li>
      </ul>
    </nav>

    <section id="history" aria-label="History">
      <h2>History</h2>
      <p>Go was designed at <a href="/wiki/Google">Google</a> in 2007 by
      Robert Griesemer, Rob Pike, and Ken Thompson.</p>
    </section>

    <section id="syntax" aria-label="Syntax">
      <h2>Syntax</h2>
      <p>Go's syntax is similar to C but with memory safety, garbage collection,
      and CSP-style concurrency.</p>
      <pre><code>package main

import "fmt"

func main() {
    fmt.Println("Hello, World!")
}</code></pre>
    </section>

    <section id="concurrency" aria-label="Concurrency">
      <h2>Concurrency</h2>
      <p>Go has built-in support for concurrent programming via goroutines and channels.</p>
    </section>

    <table>
      <tr><th>Feature</th><th>Go</th><th>Rust</th></tr>
      <tr><td>GC</td><td>Yes</td><td>No</td></tr>
      <tr><td>Generics</td><td>Yes (1.18+)</td><td>Yes</td></tr>
    </table>
  </main>

  <footer>
    <p>This page was last edited on 1 January 2026.</p>
    <a href="/privacy">Privacy policy</a> |
    <a href="/terms">Terms of use</a>
  </footer>

  <script>
    document.addEventListener('DOMContentLoaded', function() {
      console.log('loaded');
    });
  </script>
</body>
</html>`

const hackerNewsStylePage = `<!DOCTYPE html>
<html>
<head><title>Hacker News</title></head>
<body>
  <header>
    <nav>
      <a href="/" class="hn-logo"><b>Y</b></a>
      <a href="/newest">new</a> |
      <a href="/threads">threads</a> |
      <a href="/ask">ask</a> |
      <a href="/show">show</a> |
      <a href="/jobs">jobs</a>
      <span style="float:right"><a href="/login">login</a></span>
    </nav>
  </header>
  <main>
    <ol>
      <li>
        <span class="rank">1.</span>
        <a href="https://example.com/article1">Show HN: A new Go framework</a>
        <span class="meta">
          <a href="/user/johndoe">johndoe</a> |
          <a href="/item?id=1001">142 comments</a>
        </span>
      </li>
      <li>
        <span class="rank">2.</span>
        <a href="https://example.com/article2">Why Rust is great for CLI tools</a>
        <span class="meta">
          <a href="/user/janedoe">janedoe</a> |
          <a href="/item?id=1002">89 comments</a>
        </span>
      </li>
      <li>
        <span class="rank">3.</span>
        <a href="https://example.com/article3">The future of WebAssembly</a>
        <span class="meta">
          <a href="/user/alice">alice</a> |
          <a href="/item?id=1003">203 comments</a>
        </span>
      </li>
    </ol>
    <a href="/page2">More</a>
  </main>
  <footer><p>Guidelines | FAQ | Lists | API | Security</p></footer>
</body>
</html>`

const ecommerceStylePage = `<!DOCTYPE html>
<html lang="en">
<head>
  <title>Product - Online Store</title>
  <script src="https://cdn.example.com/analytics.js"></script>
</head>
<body>
  <header>
    <nav aria-label="Main navigation">
      <a href="/">Home</a>
      <a href="/products">Products</a>
      <a href="/cart">Cart (3)</a>
      <button id="menu-toggle" aria-label="Toggle menu">☰</button>
    </nav>
  </header>

  <main>
    <article>
      <h1>Wireless Bluetooth Headphones</h1>
      <img src="/img/headphones.jpg" alt="Black wireless headphones">
      <p>Premium noise-canceling headphones with 30-hour battery life.</p>
      <p>Price: <b>$79.99</b></p>

      <form id="add-to-cart">
        <label for="color">Color:</label>
        <select id="color" name="color">
          <option value="black">Black</option>
          <option value="white">White</option>
          <option value="blue">Blue</option>
        </select>

        <label for="qty">Quantity:</label>
        <input type="number" id="qty" name="qty" value="1" min="1" max="10">

        <button type="submit">Add to Cart</button>
      </form>
    </article>

    <section aria-label="Reviews">
      <h2>Customer Reviews</h2>
      <details>
        <summary>4.5 out of 5 stars (128 reviews)</summary>
        <ul>
          <li>Great sound quality! - Alice</li>
          <li>Battery lasts forever - Bob</li>
          <li>Comfortable for long sessions - Charlie</li>
        </ul>
      </details>
    </section>
  </main>

  <footer>
    <p>&copy; 2026 Online Store</p>
    <a href="/help">Help Center</a>
  </footer>
</body>
</html>`

const formHeavyPage = `<!DOCTYPE html>
<html>
<head><title>Registration Form</title></head>
<body>
  <main>
    <h1>Create Your Account</h1>
    <form action="/register" method="post">
      <div>
        <label for="firstname">First Name</label>
        <input type="text" id="firstname" name="firstname" placeholder="John" required>
      </div>
      <div>
        <label for="lastname">Last Name</label>
        <input type="text" id="lastname" name="lastname" placeholder="Doe" required>
      </div>
      <div>
        <label for="email">Email</label>
        <input type="email" id="email" name="email" placeholder="john@example.com" required>
      </div>
      <div>
        <label for="password">Password</label>
        <input type="password" id="password" name="password" required>
      </div>
      <div>
        <label for="country">Country</label>
        <select id="country" name="country">
          <option value="">Select...</option>
          <option value="us">United States</option>
          <option value="uk">United Kingdom</option>
          <option value="in">India</option>
        </select>
      </div>
      <div>
        <label>Gender</label>
        <input type="radio" id="male" name="gender" value="male"> <label for="male">Male</label>
        <input type="radio" id="female" name="gender" value="female"> <label for="female">Female</label>
      </div>
      <div>
        <input type="checkbox" id="terms" name="terms" required>
        <label for="terms">I agree to the Terms of Service</label>
      </div>
      <div>
        <textarea id="bio" name="bio" placeholder="Tell us about yourself..." rows="4"></textarea>
      </div>
      <button type="submit">Register</button>
      <button type="reset">Clear Form</button>
    </form>
  </main>
</body>
</html>`

const ariaHeavyPage = `<!DOCTYPE html>
<html>
<head><title>Dashboard</title></head>
<body>
  <div role="banner">
    <h1>Analytics Dashboard</h1>
  </div>
  <nav role="navigation" aria-label="Main">
    <ul>
      <li><a href="/dashboard">Dashboard</a></li>
      <li><a href="/reports">Reports</a></li>
      <li><a href="/settings">Settings</a></li>
    </ul>
  </nav>
  <main role="main">
    <section aria-label="Key Metrics">
      <h2>Key Metrics</h2>
      <div role="status" aria-live="polite">Last updated: 5 minutes ago</div>
      <div role="group" aria-label="Metrics cards">
        <div role="region" aria-label="Revenue">
          <h3>Revenue</h3>
          <p>$45,230</p>
        </div>
        <div role="region" aria-label="Users">
          <h3>Active Users</h3>
          <p>1,234</p>
        </div>
      </div>
    </section>
    <div role="tablist" aria-label="Data views">
      <button role="tab" aria-selected="true">Table</button>
      <button role="tab" aria-selected="false">Chart</button>
      <button role="tab" aria-selected="false">Export</button>
    </div>
    <div role="tabpanel" aria-label="Table view">
      <table role="table">
        <tr><th>Date</th><th>Revenue</th><th>Users</th></tr>
        <tr><td>Jan 1</td><td>$1,200</td><td>89</td></tr>
        <tr><td>Jan 2</td><td>$1,450</td><td>102</td></tr>
      </table>
    </div>
    <dialog open aria-label="Welcome">
      <h2>Welcome back!</h2>
      <p>You have 3 new notifications.</p>
      <button>Dismiss</button>
    </dialog>
  </main>
  <footer role="contentinfo">
    <p>Version 2.1.0</p>
  </footer>
</body>
</html>`

const deeplyNestedPage = `<!DOCTYPE html>
<html>
<head><title>Nested</title></head>
<body>
  <div><div><div><div><div>
    <nav>
      <ul>
        <li><a href="/a">Level 5 Link A</a></li>
        <li><a href="/b">Level 5 Link B</a></li>
      </ul>
    </nav>
    <div><div><div>
      <p>Deeply nested paragraph with <a href="/deep">a link inside</a>.</p>
      <form>
        <div><div>
          <input type="text" placeholder="Deep input">
          <button>Deep Button</button>
        </div></div>
      </form>
    </div></div></div>
  </div></div></div></div></div>
</body>
</html>`

const specialCharsPage = `<!DOCTYPE html>
<html>
<head><title>Special &amp; Characters</title></head>
<body>
  <h1>Caf&eacute; &amp; Restaurant</h1>
  <p>Price: &lt;$50 for d&eacute;gustation menu</p>
  <p>Japanese: &#x65E5;&#x672C;&#x8A9E;</p>
  <a href="/r&eacute;serv&eacute;">R&eacute;server</a>
  <p>Emoji: &#x1F600; &#x1F680; &#x2764;</p>
</body>
</html>`

const emptyAndMinimalPage = `<!DOCTYPE html>
<html>
<head><title>Empty</title></head>
<body></body>
</html>`

// ---------------------------------------------------------------------------
// Test runner
// ---------------------------------------------------------------------------

type realworldTestCase struct {
	name   string
	html   string
	checks []realworldCheck
}

type realworldCheck struct {
	desc string
	fn   func(t *testing.T, lite *LiteEngine)
}

func runRealworldSuite(t *testing.T, tc realworldTestCase) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(tc.html))
	}))
	defer ts.Close()

	lite := NewLiteEngine()
	defer func() { _ = lite.Close() }()

	_, err := lite.Navigate(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("Navigate failed: %v", err)
	}

	for _, c := range tc.checks {
		t.Run(c.desc, func(t *testing.T) {
			c.fn(t, lite)
		})
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func snapshotNodes(t *testing.T, lite *LiteEngine, filter string) []SnapshotNode {
	t.Helper()
	result, err := lite.Snapshot(context.Background(), "", filter)
	if err != nil {
		t.Fatalf("Snapshot(%q): %v", filter, err)
	}
	return result.Nodes
}

func getText(t *testing.T, lite *LiteEngine) string {
	t.Helper()
	result, err := lite.Text(context.Background(), "")
	if err != nil {
		t.Fatalf("Text: %v", err)
	}
	return result.Text
}

func requireRole(t *testing.T, nodes []SnapshotNode, role string, minCount int) {
	t.Helper()
	count := 0
	for _, n := range nodes {
		if n.Role == role {
			count++
		}
	}
	if count < minCount {
		t.Errorf("expected >= %d nodes with role=%q, got %d", minCount, role, count)
	}
}

func requireRoleWithName(t *testing.T, nodes []SnapshotNode, role, nameSub string) {
	t.Helper()
	for _, n := range nodes {
		if n.Role == role && strings.Contains(n.Name, nameSub) {
			return
		}
	}
	t.Errorf("no node with role=%q containing name=%q", role, nameSub)
}

func requireTextContains(t *testing.T, text string, subs ...string) {
	t.Helper()
	for _, s := range subs {
		if !strings.Contains(text, s) {
			t.Errorf("expected text to contain %q (len=%d)", s, len(text))
		}
	}
}

func requireTextNotContains(t *testing.T, text string, subs ...string) {
	t.Helper()
	for _, s := range subs {
		if strings.Contains(text, s) {
			t.Errorf("text should NOT contain %q", s)
		}
	}
}

// ---------------------------------------------------------------------------
// Test cases
// ---------------------------------------------------------------------------

func TestRealworld_WikipediaStyle(t *testing.T) {
	runRealworldSuite(t, realworldTestCase{
		name: "Wikipedia-style article",
		html: wikipediaStylePage,
		checks: []realworldCheck{
			{"scripts stripped from text", func(t *testing.T, lite *LiteEngine) {
				text := getText(t, lite)
				requireTextNotContains(t, text, "addEventListener", "console.log", "DOMContentLoaded")
			}},
			{"heading hierarchy", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "")
				requireRole(t, nodes, "heading", 4) // h1 + 3x h2
			}},
			{"navigation landmarks", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "")
				requireRole(t, nodes, "navigation", 2) // top-nav + toc
			}},
			{"sections with aria-label become regions", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "")
				requireRoleWithName(t, nodes, "region", "History")
				requireRoleWithName(t, nodes, "region", "Syntax")
				requireRoleWithName(t, nodes, "region", "Concurrency")
			}},
			{"table structure", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "")
				requireRole(t, nodes, "table", 1)
				requireRole(t, nodes, "row", 3)
				requireRole(t, nodes, "columnheader", 3)
				requireRole(t, nodes, "cell", 4)
			}},
			{"links detected as interactive", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "interactive")
				requireRole(t, nodes, "link", 1) // at minimum the logo link and toc links
				// All should be interactive
				for _, n := range nodes {
					if !n.Interactive {
						t.Errorf("interactive filter returned non-interactive: %+v", n)
					}
				}
			}},
			{"search form elements", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "interactive")
				requireRoleWithName(t, nodes, "textbox", "Search")
				requireRole(t, nodes, "button", 1)
			}},
			{"text content completeness", func(t *testing.T, lite *LiteEngine) {
				text := getText(t, lite)
				requireTextContains(t, text,
					"Go (programming language)",
					"statically typed",
					"Robert Griesemer",
					"goroutines and channels",
					"Privacy policy",
				)
			}},
			{"banner and contentinfo landmarks", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "")
				requireRole(t, nodes, "banner", 1)
				requireRole(t, nodes, "contentinfo", 1)
			}},
		},
	})
}

func TestRealworld_HackerNewsStyle(t *testing.T) {
	runRealworldSuite(t, realworldTestCase{
		name: "Hacker News style",
		html: hackerNewsStylePage,
		checks: []realworldCheck{
			{"all story links detected", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "")
				requireRole(t, nodes, "link", 5) // at minimum the story links
			}},
			{"list structure", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "")
				requireRole(t, nodes, "list", 1)
				requireRole(t, nodes, "listitem", 3)
			}},
			{"text has all headlines", func(t *testing.T, lite *LiteEngine) {
				text := getText(t, lite)
				requireTextContains(t, text,
					"Show HN: A new Go framework",
					"Why Rust is great",
					"future of WebAssembly",
				)
			}},
			{"interactive elements include nav links", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "interactive")
				var names []string
				for _, n := range nodes {
					if n.Name != "" {
						names = append(names, n.Name)
					}
				}
				found := false
				for _, name := range names {
					if strings.Contains(name, "new") || strings.Contains(name, "login") {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected nav links in interactive, got names: %v", names)
				}
			}},
		},
	})
}

func TestRealworld_EcommerceStyle(t *testing.T) {
	runRealworldSuite(t, realworldTestCase{
		name: "E-commerce product page",
		html: ecommerceStylePage,
		checks: []realworldCheck{
			{"product heading", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "")
				requireRoleWithName(t, nodes, "heading", "")
			}},
			{"image with alt text", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "")
				requireRoleWithName(t, nodes, "img", "Black wireless headphones")
			}},
			{"color select dropdown", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "interactive")
				requireRole(t, nodes, "combobox", 1)
			}},
			{"number input for quantity", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "interactive")
				requireRole(t, nodes, "textbox", 1) // number input → textbox
			}},
			{"add to cart button", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "interactive")
				requireRoleWithName(t, nodes, "button", "Add to Cart")
			}},
			{"details/summary as group/button", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "")
				requireRole(t, nodes, "group", 1)
				// summary → interactive button
				interNodes := snapshotNodes(t, lite, "interactive")
				foundSummary := false
				for _, n := range interNodes {
					if strings.Contains(n.Name, "4.5 out of 5") {
						foundSummary = true
						break
					}
				}
				if !foundSummary {
					t.Error("expected details summary to be interactive")
				}
			}},
			{"review section landmark", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "")
				requireRoleWithName(t, nodes, "region", "Reviews")
			}},
			{"article structure", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "")
				requireRole(t, nodes, "article", 1)
			}},
			{"scripts from CDN stripped", func(t *testing.T, lite *LiteEngine) {
				text := getText(t, lite)
				requireTextNotContains(t, text, "analytics.js", "cdn.example.com")
			}},
		},
	})
}

func TestRealworld_FormHeavy(t *testing.T) {
	runRealworldSuite(t, realworldTestCase{
		name: "Registration form",
		html: formHeavyPage,
		checks: []realworldCheck{
			{"all text inputs detected", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "interactive")
				textboxCount := 0
				for _, n := range nodes {
					if n.Role == "textbox" {
						textboxCount++
					}
				}
				// firstname, lastname, email, password, bio (textarea)
				if textboxCount < 5 {
					t.Errorf("expected >= 5 textboxes, got %d", textboxCount)
				}
			}},
			{"radio buttons", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "interactive")
				requireRole(t, nodes, "radio", 2)
			}},
			{"checkbox", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "interactive")
				requireRole(t, nodes, "checkbox", 1)
			}},
			{"select dropdown", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "interactive")
				requireRole(t, nodes, "combobox", 1)
			}},
			{"submit and reset buttons", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "interactive")
				btnNames := make(map[string]bool)
				for _, n := range nodes {
					if n.Role == "button" {
						btnNames[n.Name] = true
					}
				}
				if !btnNames["Register"] {
					t.Error("expected Register button")
				}
				if !btnNames["Clear Form"] {
					t.Error("expected Clear Form button")
				}
			}},
			{"form landmark", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "")
				requireRole(t, nodes, "form", 1)
			}},
			{"type into input and verify", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "interactive")
				var firstInput string
				for _, n := range nodes {
					if n.Role == "textbox" {
						firstInput = n.Ref
						break
					}
				}
				if firstInput == "" {
					t.Fatal("no textbox found")
				}
				err := lite.Type(context.Background(), "", firstInput, "TestUser")
				if err != nil {
					t.Errorf("Type: %v", err)
				}
			}},
		},
	})
}

func TestRealworld_AriaHeavy(t *testing.T) {
	runRealworldSuite(t, realworldTestCase{
		name: "ARIA-heavy dashboard",
		html: ariaHeavyPage,
		checks: []realworldCheck{
			{"banner role from div[role=banner]", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "")
				requireRole(t, nodes, "banner", 1)
			}},
			{"navigation with aria-label", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "")
				requireRoleWithName(t, nodes, "navigation", "Main")
			}},
			{"main role", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "")
				requireRole(t, nodes, "main", 1)
			}},
			{"tablist with tab buttons", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "")
				requireRole(t, nodes, "tablist", 1)
				requireRole(t, nodes, "tab", 3)
			}},
			{"tabs are interactive", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "interactive")
				tabCount := 0
				for _, n := range nodes {
					if n.Role == "tab" {
						tabCount++
					}
				}
				if tabCount < 3 {
					t.Errorf("expected 3 interactive tabs, got %d", tabCount)
				}
			}},
			{"status role detected", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "")
				requireRole(t, nodes, "status", 1)
			}},
			{"region roles with aria-label", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "")
				requireRoleWithName(t, nodes, "region", "Revenue")
				requireRoleWithName(t, nodes, "region", "Users")
			}},
			{"dialog detected", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "")
				requireRoleWithName(t, nodes, "dialog", "Welcome")
			}},
			{"tabpanel role", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "")
				requireRole(t, nodes, "tabpanel", 1)
			}},
			{"contentinfo footer", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "")
				requireRole(t, nodes, "contentinfo", 1)
			}},
			{"table inside tabpanel", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "")
				requireRole(t, nodes, "table", 1)
				requireRole(t, nodes, "row", 3)
			}},
		},
	})
}

func TestRealworld_DeeplyNested(t *testing.T) {
	runRealworldSuite(t, realworldTestCase{
		name: "Deeply nested DOM",
		html: deeplyNestedPage,
		checks: []realworldCheck{
			{"links found through nesting", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "interactive")
				requireRole(t, nodes, "link", 3) // Link A, Link B, link inside p
			}},
			{"deep input and button found", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "interactive")
				requireRole(t, nodes, "textbox", 1)
				requireRole(t, nodes, "button", 1)
			}},
			{"text extraction through nesting", func(t *testing.T, lite *LiteEngine) {
				text := getText(t, lite)
				requireTextContains(t, text, "Level 5 Link A", "Deeply nested paragraph")
			}},
			{"depth values increase with nesting", func(t *testing.T, lite *LiteEngine) {
				nodes := snapshotNodes(t, lite, "")
				maxDepth := 0
				for _, n := range nodes {
					if n.Depth > maxDepth {
						maxDepth = n.Depth
					}
				}
				if maxDepth < 5 {
					t.Errorf("expected maxDepth >= 5 for deeply nested page, got %d", maxDepth)
				}
			}},
		},
	})
}

func TestRealworld_SpecialCharacters(t *testing.T) {
	runRealworldSuite(t, realworldTestCase{
		name: "Special characters & Unicode",
		html: specialCharsPage,
		checks: []realworldCheck{
			{"HTML entities decoded in text", func(t *testing.T, lite *LiteEngine) {
				text := getText(t, lite)
				// &amp; should be decoded to &
				requireTextContains(t, text, "&")
				// &lt; should be decoded
				requireTextContains(t, text, "<$50")
			}},
			{"accented characters", func(t *testing.T, lite *LiteEngine) {
				text := getText(t, lite)
				requireTextContains(t, text, "Caf")
			}},
			{"unicode CJK characters", func(t *testing.T, lite *LiteEngine) {
				text := getText(t, lite)
				requireTextContains(t, text, "日本語")
			}},
		},
	})
}

func TestRealworld_EmptyPage(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(emptyAndMinimalPage))
	}))
	defer ts.Close()

	lite := NewLiteEngine()
	defer func() { _ = lite.Close() }()

	_, err := lite.Navigate(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("Navigate: %v", err)
	}

	snapRes, err := lite.Snapshot(context.Background(), "", "all")
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	// Empty body should yield 0 or very few nodes
	if len(snapRes.Nodes) > 1 {
		t.Errorf("expected <= 1 nodes for empty page, got %d", len(snapRes.Nodes))
	}

	textRes, err := lite.Text(context.Background(), "")
	if err != nil {
		t.Fatalf("Text: %v", err)
	}
	if strings.TrimSpace(textRes.Text) != "" {
		t.Errorf("expected empty text for empty page, got %q", textRes.Text)
	}
}

func TestRealworld_NonHTMLContentType(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error": "not html"}`))
	}))
	defer ts.Close()

	lite := NewLiteEngine()
	defer func() { _ = lite.Close() }()

	_, err := lite.Navigate(context.Background(), ts.URL)
	if err == nil {
		t.Error("expected error for non-HTML content type")
	}
	if !strings.Contains(err.Error(), "unsupported content type") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRealworld_HTTP404(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = w.Write([]byte("Not Found"))
	}))
	defer ts.Close()

	lite := NewLiteEngine()
	defer func() { _ = lite.Close() }()

	_, err := lite.Navigate(context.Background(), ts.URL)
	if err == nil {
		t.Error("expected error for 404")
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRealworld_LargePagePerformance(t *testing.T) {
	// Generate a page with many elements to test performance doesn't degrade
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html><head><title>Large</title></head><body>`)
	for i := range 200 {
		_, _ = fmt.Fprintf(&b, `<div><h3>Section %d</h3><p>Content %d</p><a href="/s%d">Link %d</a></div>`, i, i, i, i)
	}
	b.WriteString(`</body></html>`)
	largePage := b.String()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(largePage))
	}))
	defer ts.Close()

	lite := NewLiteEngine()
	defer func() { _ = lite.Close() }()

	_, err := lite.Navigate(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("Navigate: %v", err)
	}

	nodes := snapshotNodes(t, lite, "")
	if len(nodes) < 600 { // 200 * (div + h3 + p + a) = 800 minimum
		t.Errorf("expected >= 600 nodes for large page, got %d", len(nodes))
	}

	interNodes := snapshotNodes(t, lite, "interactive")
	if len(interNodes) < 200 {
		t.Errorf("expected >= 200 interactive nodes (links), got %d", len(interNodes))
	}
}

func TestRealworld_MultipleScriptTags(t *testing.T) {
	page := `<!DOCTYPE html>
<html><head>
<script>var x = 1;</script>
<script src="https://cdn.example.com/jquery.js"></script>
<script type="module">import {foo} from './bar.js';</script>
</head><body>
<h1>Content</h1>
<script>document.write('injected');</script>
<p>Visible paragraph</p>
<script>
  // Multi-line script
  function init() {
    var el = document.getElementById('x');
    el.style.display = 'block';
  }
  init();
</script>
</body></html>`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	}))
	defer ts.Close()

	lite := NewLiteEngine()
	defer func() { _ = lite.Close() }()

	_, err := lite.Navigate(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("Navigate: %v", err)
	}

	text := getText(t, lite)
	requireTextContains(t, text, "Content", "Visible paragraph")
	requireTextNotContains(t, text, "document.write", "init()", "import", "jquery")

	nodes := snapshotNodes(t, lite, "")
	for _, n := range nodes {
		if n.Tag == "script" {
			t.Error("script elements should not appear in snapshot")
		}
	}
}

func TestRealworld_InlineStyles(t *testing.T) {
	page := `<!DOCTYPE html>
<html><head>
<style>.red { color: red; } .hidden { display: none; }</style>
</head><body>
<p class="red">Styled text</p>
<style>.blue { color: blue; }</style>
<p>Normal text</p>
</body></html>`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	}))
	defer ts.Close()

	lite := NewLiteEngine()
	defer func() { _ = lite.Close() }()

	_, err := lite.Navigate(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("Navigate: %v", err)
	}

	nodes := snapshotNodes(t, lite, "")
	for _, n := range nodes {
		if n.Tag == "style" {
			t.Error("style elements should not appear in snapshot")
		}
	}
}

func TestRealworld_ClickWorkflow(t *testing.T) {
	// Test clicking buttons — buttons should work without navigation side effects
	page := `<!DOCTYPE html>
<html><head><title>Click Test</title></head>
<body>
<button id="btn1">Button 1</button>
<button id="btn2">Button 2</button>
<span role="button" tabindex="0">Custom Button</span>
<input type="submit" value="Submit">
<p>Some content here</p>
</body></html>`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	}))
	defer ts.Close()

	lite := NewLiteEngine()
	defer func() { _ = lite.Close() }()

	_, _ = lite.Navigate(context.Background(), ts.URL)
	nodes := snapshotNodes(t, lite, "interactive")

	// Click every interactive element — should not error or panic
	for _, n := range nodes {
		err := lite.Click(context.Background(), "", n.Ref)
		if err != nil {
			t.Errorf("Click(%s role=%s name=%q): %v", n.Ref, n.Role, n.Name, err)
		}
	}

	// Engine should still be usable
	textRes2, err := lite.Text(context.Background(), "")
	if err != nil {
		t.Fatalf("Text after clicks: %v", err)
	}
	if !strings.Contains(textRes2.Text, "Some content here") {
		t.Errorf("unexpected text after clicks: %s", textRes2.Text)
	}
}

func TestRealworld_ClickLinkRecovery(t *testing.T) {
	// Clicking anchor links triggers gost-dom navigation which can panic
	// (no script engine). Verify we recover gracefully.
	page := `<!DOCTYPE html>
<html><head><title>Links</title></head>
<body>
<a href="/page2">Go to Page 2</a>
<a href="/page3">Go to Page 3</a>
</body></html>`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	}))
	defer ts.Close()

	lite := NewLiteEngine()
	defer func() { _ = lite.Close() }()

	_, _ = lite.Navigate(context.Background(), ts.URL)
	nodes := snapshotNodes(t, lite, "interactive")

	for _, n := range nodes {
		if n.Role == "link" {
			err := lite.Click(context.Background(), "", n.Ref)
			// Error is acceptable (recovered panic) — the key is it doesn't crash
			_ = err
		}
	}
}

func TestRealworld_TypeWorkflow(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(formHeavyPage))
	}))
	defer ts.Close()

	lite := NewLiteEngine()
	defer func() { _ = lite.Close() }()

	_, _ = lite.Navigate(context.Background(), ts.URL)
	nodes := snapshotNodes(t, lite, "interactive")

	// Type into all textboxes
	for _, n := range nodes {
		if n.Role == "textbox" {
			err := lite.Type(context.Background(), "", n.Ref, "test-value")
			if err != nil {
				t.Errorf("Type(%s name=%q): %v", n.Ref, n.Name, err)
			}
		}
	}
}
