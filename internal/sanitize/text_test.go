package sanitize

import "testing"

func TestCleanErrorRedactsAbsolutePaths(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "short unix path",
			input: "/var/log",
			want:  "[path]",
		},
		{
			name:  "quoted unix path",
			input: `error at "/Users/test/file.txt" failed`,
			want:  `error at "[path]" failed`,
		},
		{
			name:  "mixed unix and windows paths",
			input: `copy /var/log to C:\Users\test\file.txt`,
			want:  `copy [path] to [path]`,
		},
		{
			name:  "colon before unix path",
			input: `error:/Users/test/file.txt`,
			want:  `error:[path]`,
		},
		{
			name:  "colon before windows path",
			input: `error:C:\Users\test\file.txt`,
			want:  `error:[path]`,
		},
		{
			name:  "path-like substring inside word is preserved",
			input: `description/Users/guide`,
			want:  `description/Users/guide`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CleanError(tt.input, 1024); got != tt.want {
				t.Fatalf("CleanError(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
