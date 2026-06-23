package browsers

import "testing"

func TestResolveLaunchMode(t *testing.T) {
	cases := []struct {
		in   LaunchMode
		want LaunchMode
	}{
		{LaunchModeChrome, LaunchModeChrome},
		{LaunchModeLite, LaunchModeLite},
		{LaunchModeAuto, LaunchModeChrome},
		{"", LaunchModeChrome},
		{"unknown", LaunchModeChrome},
	}
	for _, c := range cases {
		t.Run(string(c.in), func(t *testing.T) {
			if got := ResolveLaunchMode(c.in); got != c.want {
				t.Errorf("ResolveLaunchMode(%q) = %q; want %q", c.in, got, c.want)
			}
		})
	}
}
