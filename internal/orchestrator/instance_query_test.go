package orchestrator

import "testing"

func TestEffectiveInstanceStatus(t *testing.T) {
	cases := []struct {
		name   string
		status string
		active bool
		want   string
	}{
		{"live but stored stopped -> running", "stopped", true, "running"},
		{"dead starting -> stopped", "starting", false, "stopped"},
		{"dead running -> stopped", "running", false, "stopped"},
		{"dead stopping -> stopped", "stopping", false, "stopped"},
		{"live running passes through", "running", true, "running"},
		{"dead stopped passes through", "stopped", false, "stopped"},
		{"unknown status passes through (live)", "errored", true, "errored"},
		{"unknown status passes through (dead)", "errored", false, "errored"},
	}
	for _, c := range cases {
		if got := effectiveInstanceStatus(c.status, c.active); got != c.want {
			t.Errorf("%s: effectiveInstanceStatus(%q, %v) = %q, want %q", c.name, c.status, c.active, got, c.want)
		}
	}
}
