package web

import (
	"testing"
)

func TestSafePath(t *testing.T) {
	tests := []struct {
		name    string
		base    string
		path    string
		wantErr bool
	}{
		{"valid relative", "/tmp/state", "pdfs/out.pdf", false},
		{"valid absolute inside", "/tmp/state", "/tmp/state/pdfs/out.pdf", false},
		{"traversal dotdot", "/tmp/state", "../etc/passwd", true},
		{"traversal absolute", "/tmp/state", "/etc/passwd", true},
		{"traversal hidden", "/tmp/state", "pdfs/../../etc/passwd", true},
		{"base itself", "/tmp/state", "/tmp/state", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SafePath(tt.base, tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("SafePath(%q, %q) error = %v, wantErr %v", tt.base, tt.path, err, tt.wantErr)
			}
		})
	}
}
