package handlers

import ("errors"; "testing"; "github.com/pinchtab/pinchtab/internal/bridge/cdpops")

func TestPointerPreconditionIsNotRetryable(t *testing.T) {
 for _, err := range []error{cdpops.ErrElementHidden, cdpops.ErrElementBlocked, cdpops.ErrElementOccluded, cdpops.ErrElementOffscreen} {if actionFailureIsRetryable(err,false) {t.Errorf("pointer precondition marked retryable: %v",err)}}
 if !actionFailureIsRetryable(errors.New("transient"),false) {t.Error("unrelated failure changed")}
}
