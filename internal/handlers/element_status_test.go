package handlers

import (
	"errors"
	"fmt"
	"testing"
)

func TestStatusForElementErr(t *testing.T) {
	if got := statusForElementErr(fmt.Errorf("%w: %q", ErrElementNotFound, "#missing")); got != 404 {
		t.Fatalf("selector-not-found should map to 404, got %d", got)
	}
	if got := statusForElementErr(fmt.Errorf("wrapped: %w", ErrElementNotFound)); got != 404 {
		t.Fatalf("wrapped not-found should still map to 404, got %d", got)
	}
	if got := statusForElementErr(errors.New("cdp transport failure")); got != 500 {
		t.Fatalf("a genuine internal error should stay 500, got %d", got)
	}
}
