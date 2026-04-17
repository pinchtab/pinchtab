package engine

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/netguard"
)

func newTestServer(body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}))
}

const testPage = `<!DOCTYPE html>
<html>
<head><title>Test Page</title></head>
<body>
	<h1>Hello World</h1>
	<nav><a href="/about">About</a></nav>
	<main>
		<p>Some content here.</p>
		<button id="btn">Click Me</button>
		<input type="text" placeholder="Enter name">
		<textarea placeholder="Description"></textarea>
		<form><select><option>One</option></select></form>
	</main>
	<footer>Footer text</footer>
</body>
</html>`

func TestLiteEngine_Navigate(t *testing.T) {
	ts := newTestServer(testPage)
	defer ts.Close()

	lite := NewLiteEngine()
	defer func() { _ = lite.Close() }()

	result, err := lite.Navigate(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("Navigate: %v", err)
	}
	if result.TabID == "" {
		t.Error("expected non-empty tabId")
	}
	if result.URL != ts.URL {
		t.Errorf("URL = %q, want %q", result.URL, ts.URL)
	}
}

func TestLiteEngine_Snapshot_All(t *testing.T) {
	ts := newTestServer(testPage)
	defer ts.Close()

	lite := NewLiteEngine()
	defer func() { _ = lite.Close() }()

	_, err := lite.Navigate(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("Navigate: %v", err)
	}

	result, err := lite.Snapshot(context.Background(), "", "all")
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(result.Nodes) == 0 {
		t.Fatal("expected nodes in snapshot")
	}

	// Verify roles are assigned
	roleSet := make(map[string]bool)
	for _, n := range result.Nodes {
		roleSet[n.Role] = true
	}
	for _, expected := range []string{"heading", "link", "button", "textbox", "combobox"} {
		if !roleSet[expected] {
			t.Errorf("expected role %q in snapshot, roles found: %v", expected, roleSet)
		}
	}
}

func TestLiteEngine_Snapshot_Interactive(t *testing.T) {
	ts := newTestServer(testPage)
	defer ts.Close()

	lite := NewLiteEngine()
	defer func() { _ = lite.Close() }()

	_, _ = lite.Navigate(context.Background(), ts.URL)

	result, err := lite.Snapshot(context.Background(), "", "interactive")
	if err != nil {
		t.Fatalf("Snapshot interactive: %v", err)
	}

	for _, n := range result.Nodes {
		if !n.Interactive {
			t.Errorf("interactive filter returned non-interactive node: %+v", n)
		}
	}

	if len(result.Nodes) < 3 {
		t.Errorf("expected at least 3 interactive nodes (link, button, input), got %d", len(result.Nodes))
	}
}

func TestLiteEngine_Text(t *testing.T) {
	ts := newTestServer(testPage)
	defer ts.Close()

	lite := NewLiteEngine()
	defer func() { _ = lite.Close() }()

	_, _ = lite.Navigate(context.Background(), ts.URL)

	result, err := lite.Text(context.Background(), "")
	if err != nil {
		t.Fatalf("Text: %v", err)
	}

	for _, want := range []string{"Hello World", "About", "Click Me", "Some content here"} {
		if !strings.Contains(result.Text, want) {
			t.Errorf("text should contain %q, got: %s", want, result.Text)
		}
	}
}

func TestLiteEngine_Click(t *testing.T) {
	ts := newTestServer(testPage)
	defer ts.Close()

	lite := NewLiteEngine()
	defer func() { _ = lite.Close() }()

	_, _ = lite.Navigate(context.Background(), ts.URL)

	result, _ := lite.Snapshot(context.Background(), "", "interactive")
	var buttonRef string
	for _, n := range result.Nodes {
		if n.Role == "button" {
			buttonRef = n.Ref
			break
		}
	}
	if buttonRef == "" {
		t.Fatal("no button found in snapshot")
	}

	if err := lite.Click(context.Background(), "", buttonRef); err != nil {
		t.Errorf("Click: %v", err)
	}
}

func TestLiteEngine_Type(t *testing.T) {
	ts := newTestServer(testPage)
	defer ts.Close()

	lite := NewLiteEngine()
	defer func() { _ = lite.Close() }()

	_, _ = lite.Navigate(context.Background(), ts.URL)

	result, _ := lite.Snapshot(context.Background(), "", "interactive")
	var inputRef string
	for _, n := range result.Nodes {
		if n.Role == "textbox" {
			inputRef = n.Ref
			break
		}
	}
	if inputRef == "" {
		t.Fatal("no textbox found in snapshot")
	}

	if err := lite.Type(context.Background(), "", inputRef, "hello"); err != nil {
		t.Errorf("Type: %v", err)
	}
}

func TestLiteEngine_RefNotFound(t *testing.T) {
	lite := NewLiteEngine()
	defer func() { _ = lite.Close() }()

	// No page loaded
	_, err := lite.Snapshot(context.Background(), "", "all")
	if err == nil {
		t.Error("expected error for snapshot without navigate")
	}

	// After navigate, bad ref
	ts := newTestServer(testPage)
	defer ts.Close()

	_, _ = lite.Navigate(context.Background(), ts.URL)
	_, _ = lite.Snapshot(context.Background(), "", "all")

	if err := lite.Click(context.Background(), "", "nonexistent"); err == nil {
		t.Error("expected error for bad ref")
	}
}

func TestLiteEngine_ScriptStyleSkipped(t *testing.T) {
	page := `<!DOCTYPE html>
<html><head><title>T</title></head>
<body>
<script>var x = 1;</script>
<style>.red { color: red; }</style>
<p>Visible</p>
</body></html>`

	ts := newTestServer(page)
	defer ts.Close()

	lite := NewLiteEngine()
	defer func() { _ = lite.Close() }()

	_, _ = lite.Navigate(context.Background(), ts.URL)
	result, _ := lite.Snapshot(context.Background(), "", "all")

	for _, n := range result.Nodes {
		if n.Tag == "script" || n.Tag == "style" {
			t.Errorf("snapshot should skip %s elements", n.Tag)
		}
	}
}

func TestLiteEngine_NavigateBlocksRedirectToPrivateIP(t *testing.T) {
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
	}))
	defer redirector.Close()

	oldDial := dialLiteAddress
	dialLiteAddress = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, redirector.Listener.Addr().String())
	}
	t.Cleanup(func() { dialLiteAddress = oldDial })

	oldResolve := netguard.ResolveHostIPs
	netguard.ResolveHostIPs = func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	}
	t.Cleanup(func() { netguard.ResolveHostIPs = oldResolve })

	lite := NewLiteEngine()
	defer func() { _ = lite.Close() }()

	ctx := WithNavigateNetworkPolicy(context.Background(), &NavigateNetworkPolicy{MaxRedirects: -1})
	_, err := lite.Navigate(ctx, "http://safe.example/index.html")
	if err == nil {
		t.Fatal("expected redirect to private IP to be blocked")
	}
	if !IsNetworkPolicyBlocked(err) {
		t.Fatalf("expected network policy block, got %v", err)
	}
	if !strings.Contains(err.Error(), "redirect to blocked") {
		t.Fatalf("expected redirect block error, got %v", err)
	}
}

func TestLiteEngine_NavigateAllowsTrustedResolvedPrivateIP(t *testing.T) {
	page := newTestServer(`<html><head><title>Trusted</title></head><body>ok</body></html>`)
	defer page.Close()

	oldDial := dialLiteAddress
	dialLiteAddress = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, page.Listener.Addr().String())
	}
	t.Cleanup(func() { dialLiteAddress = oldDial })

	oldResolve := netguard.ResolveHostIPs
	netguard.ResolveHostIPs = func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("10.0.0.5")}, nil
	}
	t.Cleanup(func() { netguard.ResolveHostIPs = oldResolve })

	lite := NewLiteEngine()
	defer func() { _ = lite.Close() }()

	ctx := WithNavigateNetworkPolicy(context.Background(), &NavigateNetworkPolicy{
		TrustedResolvedIP: []netip.Addr{netip.MustParseAddr("10.0.0.5")},
		MaxRedirects:      -1,
	})
	result, err := lite.Navigate(ctx, "http://trusted.example/index.html")
	if err != nil {
		t.Fatalf("expected trusted resolved IP to be allowed, got %v", err)
	}
	if result.Title != "Trusted" {
		t.Fatalf("title = %q, want Trusted", result.Title)
	}
}

func TestLiteEngine_AriaAttributes(t *testing.T) {
	page := `<!DOCTYPE html>
<html><head><title>Aria</title></head>
<body>
<div role="navigation" aria-label="Main Nav">content</div>
<span role="button" tabindex="0">Custom Btn</span>
</body></html>`

	ts := newTestServer(page)
	defer ts.Close()

	lite := NewLiteEngine()
	defer func() { _ = lite.Close() }()

	_, _ = lite.Navigate(context.Background(), ts.URL)
	result, _ := lite.Snapshot(context.Background(), "", "all")

	foundNav := false
	foundBtn := false
	for _, n := range result.Nodes {
		if n.Role == "navigation" && n.Name == "Main Nav" {
			foundNav = true
		}
		if n.Role == "button" && n.Name == "Custom Btn" {
			foundBtn = true
		}
	}
	if !foundNav {
		t.Error("expected to find navigation role with aria-label")
	}
	if !foundBtn {
		t.Error("expected to find button role from role attribute")
	}
}

func TestLiteEngine_MultiTab(t *testing.T) {
	page1 := `<!DOCTYPE html><html><head><title>Page 1</title></head><body><p>First</p></body></html>`
	page2 := `<!DOCTYPE html><html><head><title>Page 2</title></head><body><p>Second</p></body></html>`

	ts1 := newTestServer(page1)
	defer ts1.Close()
	ts2 := newTestServer(page2)
	defer ts2.Close()

	lite := NewLiteEngine()
	defer func() { _ = lite.Close() }()

	res1, _ := lite.Navigate(context.Background(), ts1.URL)
	res2, _ := lite.Navigate(context.Background(), ts2.URL)

	if res1.TabID == res2.TabID {
		t.Error("tab IDs should be different")
	}

	// Current tab is the most recent (page2)
	result, _ := lite.Text(context.Background(), "")
	if !strings.Contains(result.Text, "Second") {
		t.Errorf("expected page 2 text, got: %s", result.Text)
	}

	result2, _ := lite.Text(context.Background(), res1.TabID)
	if !strings.Contains(result2.Text, "First") {
		t.Errorf("expected page 1 text via tab ID, got: %s", result2.Text)
	}
}

func TestLiteEngine_Close(t *testing.T) {
	ts := newTestServer(testPage)
	defer ts.Close()

	lite := NewLiteEngine()
	_, _ = lite.Navigate(context.Background(), ts.URL)

	if err := lite.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}

	// After close, operations should fail
	_, err := lite.Snapshot(context.Background(), "", "all")
	if err == nil {
		t.Error("expected error after close")
	}
}

func TestLiteEngine_Capabilities(t *testing.T) {
	lite := NewLiteEngine()
	defer func() { _ = lite.Close() }()

	caps := lite.Capabilities()
	if len(caps) != 5 {
		t.Errorf("expected 5 capabilities, got %d", len(caps))
	}
}

func TestLiteEngine_Name(t *testing.T) {
	lite := NewLiteEngine()
	defer func() { _ = lite.Close() }()

	if lite.Name() != "lite" {
		t.Errorf("Name() = %q, want %q", lite.Name(), "lite")
	}
}

func TestNormalizeWhitespace(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"hello world", "hello world"},
		{"  hello   world  ", "hello world"},
		{"line1\n\n\nline2", "line1 line2"},
		{"\t  tabs \t and  \t spaces", "tabs and spaces"},
		{"", ""},
	}
	for _, tt := range tests {
		got := normalizeWhitespace(tt.in)
		if got != tt.want {
			t.Errorf("normalizeWhitespace(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
