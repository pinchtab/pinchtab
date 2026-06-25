package ghostchrome

import (
	"strings"
	"testing"
)

func TestGhostResult_ShouldAccept(t *testing.T) {
	tests := []struct {
		name   string
		result GhostResult
		want   bool
	}{
		{"good static content", GhostResult{OK: true, Quality: 80, PageClass: "static"}, true},
		{"at threshold", GhostResult{OK: true, Quality: 60, PageClass: "static"}, true},
		{"below threshold", GhostResult{OK: true, Quality: 50, PageClass: "static"}, false},
		{"not ok", GhostResult{OK: false, Quality: 80, PageClass: "static"}, false},
		{"needs browser", GhostResult{OK: true, Quality: 80, NeedsBrowser: true, PageClass: "static"}, false},
		{"is blocked", GhostResult{OK: true, Quality: 80, IsBlocked: true, PageClass: "static"}, false},
		{"is thin", GhostResult{OK: true, Quality: 80, IsThin: true, PageClass: "static"}, false},
		{"spa page class", GhostResult{OK: true, Quality: 80, PageClass: "spa"}, false},
		{"dynamic page class", GhostResult{OK: true, Quality: 80, PageClass: "dynamic"}, false},
		{"blocked page class", GhostResult{OK: true, Quality: 80, PageClass: "blocked"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.result.ShouldAccept(); got != tt.want {
				t.Errorf("ShouldAccept() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGhostResult_FormatReason(t *testing.T) {
	t.Run("skip reason", func(t *testing.T) {
		r := &GhostResult{SkipReason: "no static browser available"}
		if got := r.FormatReason(); got != "no static browser available" {
			t.Errorf("FormatReason() = %q, want skip reason", got)
		}
	})
	t.Run("signal format", func(t *testing.T) {
		r := &GhostResult{OK: true, Quality: 82, PageClass: "static"}
		got := r.FormatReason()
		if got != "quality=82 needsBrowser=false pageClass=static" {
			t.Errorf("FormatReason() = %q", got)
		}
	})
}

func TestLooksLikeSPA(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"nextjs shell", `Loading <div id="__next"></div>`, true},
		{"react root", `<div id="root"></div>`, true},
		{"vue app", `<div id="app"></div>`, true},
		{"noscript tag", `<noscript>Enable JS</noscript>`, true},
		{"static content", strings.Repeat("word ", 200), false},
		{"spa marker with enough content", strings.Repeat("word ", 200) + `<div id="__next"></div>`, false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LooksLikeSPA(tt.content); got != tt.want {
				t.Errorf("LooksLikeSPA() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEstimateQuality(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantMin int
		wantMax int
	}{
		{"empty", "", 0, 0},
		{"single word", "hello", 1, 20},
		{"short", "one two three four five six seven eight nine ten", 20, 50},
		{"medium", strings.Repeat("word ", 100), 50, 80},
		{"long", strings.Repeat("word ", 300), 80, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EstimateQuality(tt.content)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("EstimateQuality() = %d, want [%d, %d]", got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestAssessContent(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		wantAccept bool
		wantThin   bool
		wantSPA    bool
		wantMinQ   int
	}{
		{
			name:       "rich static article",
			content:    strings.Repeat("word ", 250),
			wantAccept: true,
			wantMinQ:   60,
		},
		{
			name:       "below threshold but not thin",
			content:    "hello world",
			wantAccept: false,
		},
		{
			name:       "empty content",
			content:    "",
			wantAccept: false,
			wantThin:   true,
		},
		{
			name:       "SPA shell with next marker",
			content:    `Loading... <div id="__next"></div>`,
			wantAccept: false,
			wantSPA:    true,
		},
		{
			name:       "medium content below threshold",
			content:    strings.Repeat("word ", 80),
			wantAccept: false,
			wantMinQ:   50,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gr := AssessContent(tt.content)
			if !gr.OK {
				t.Fatal("AssessContent should always return OK=true")
			}
			if got := gr.ShouldAccept(); got != tt.wantAccept {
				t.Errorf("ShouldAccept() = %v, want %v (quality=%d, thin=%v, spa=%v)",
					got, tt.wantAccept, gr.Quality, gr.IsThin, gr.NeedsBrowser)
			}
			if tt.wantThin && !gr.IsThin {
				t.Error("expected IsThin=true")
			}
			if tt.wantSPA && !gr.NeedsBrowser {
				t.Error("expected NeedsBrowser=true")
			}
			if tt.wantSPA && gr.PageClass != "spa" {
				t.Errorf("PageClass = %q, want spa", gr.PageClass)
			}
			if tt.wantMinQ > 0 && gr.Quality < tt.wantMinQ {
				t.Errorf("Quality = %d, want >= %d", gr.Quality, tt.wantMinQ)
			}
		})
	}
}

func TestAssessSnapshot(t *testing.T) {
	tests := []struct {
		name  string
		nodes []SnapshotNode
		want  bool
	}{
		{
			name:  "empty nodes",
			nodes: nil,
			want:  false,
		},
		{
			name:  "fewer than 3 nodes",
			nodes: []SnapshotNode{{Role: "link", Name: "Home"}, {Role: "button", Name: "Click"}},
			want:  false,
		},
		{
			name: "only generic containers",
			nodes: []SnapshotNode{
				{Role: "generic", Name: ""},
				{Role: "generic", Name: ""},
				{Role: "generic", Name: ""},
				{Role: "", Name: ""},
			},
			want: false,
		},
		{
			name: "only none roles",
			nodes: []SnapshotNode{
				{Role: "none", Name: ""},
				{Role: "none", Name: ""},
				{Role: "none", Name: ""},
			},
			want: false,
		},
		{
			name: "diverse roles accepted",
			nodes: []SnapshotNode{
				{Role: "navigation", Name: "Main nav"},
				{Role: "link", Name: "Home"},
				{Role: "heading", Name: "Welcome"},
				{Role: "button", Name: "Submit"},
			},
			want: true,
		},
		{
			name: "mixed generic and semantic",
			nodes: []SnapshotNode{
				{Role: "generic", Name: ""},
				{Role: "link", Name: "About"},
				{Role: "generic", Name: ""},
				{Role: "heading", Name: "Title"},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AssessSnapshot(tt.nodes); got != tt.want {
				t.Errorf("AssessSnapshot() = %v, want %v", got, tt.want)
			}
		})
	}
}
