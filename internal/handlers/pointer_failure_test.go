package handlers

import (
	"errors"
	"github.com/pinchtab/pinchtab/internal/bridge/cdpops"
	"testing"
)

func TestPointerPreconditionIsNotRetryable(t *testing.T) {
	for _, err := range []error{cdpops.ErrElementHidden, cdpops.ErrElementBlocked, cdpops.ErrElementOccluded, cdpops.ErrElementOffscreen} {
		if actionFailureIsRetryable(err, false) {
			t.Errorf("pointer precondition marked retryable: %v", err)
		}
	}
	if !actionFailureIsRetryable(errors.New("transient"), false) {
		t.Error("unrelated failure changed")
	}
}

func TestPointerFailedStepsPreserveCodeAndNonRetryableDetails(t *testing.T) {
	h := &Handlers{}
	for _, tc := range []struct {
		err  error
		code string
	}{
		{cdpops.ErrElementHidden, "element_hidden"},
		{cdpops.ErrElementBlocked, "element_blocked"},
		{cdpops.ErrElementOccluded, "element_occluded"},
		{cdpops.ErrElementOffscreen, "element_offscreen"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			result := h.failedStep(2, "click", "", tc.err, "pointer refused")
			if result.Success || result.Index != 2 || result.Code != tc.code {
				t.Fatalf("unexpected failed step: %+v", result)
			}
			if result.Details["retryable"] != false || result.Details["doNotRetry"] != true {
				t.Fatalf("missing non-retryable guidance: %+v", result.Details)
			}
		})
	}
}
