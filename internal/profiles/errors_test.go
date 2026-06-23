package profiles

import (
	"fmt"
	"net/http"
	"testing"
)

func TestProfileMutationStatus(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, http.StatusOK},
		{"invalid name", tagged(ErrInvalidProfileName, "profile name cannot be empty"), http.StatusBadRequest},
		{"exists", tagged(ErrProfileExists, `profile "x" already exists`), http.StatusConflict},
		{"dir exists", tagged(ErrProfileDirExists, `profile directory for "x" already exists`), http.StatusConflict},
		{"not found", tagged(ErrProfileNotFound, `profile "x" not found`), http.StatusInternalServerError},
		{"other", fmt.Errorf("boom"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := profileMutationStatus(tc.err); got != tc.want {
				t.Fatalf("profileMutationStatus(%v) = %d, want %d", tc.err, got, tc.want)
			}
		})
	}
}
