package handlers

import (
	"context"
	"time"

	"github.com/pinchtab/pinchtab/internal/readiness"
)

func pollUntil(ctx context.Context, interval time.Duration, check func() (bool, error)) error {
	return readiness.Poll(ctx, interval, check)
}
