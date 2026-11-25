package graph

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"time"
)

type RetryPolicy struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
}

func retryHint(err error) (time.Duration, bool) {
	var apiError APIError
	if !errors.As(err, &apiError) {
		return time.Duration(0), false
	}
	if apiError.Status == http.StatusTooManyRequests || apiError.Status >= 500 {
		// Retry-After (seconds) is parsed in getRetryAfter and carried on the APIError.
		return apiError.RetryAfter, true
	}
	return time.Duration(0), false
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p RetryPolicy) delay(attempt int, serverHint time.Duration) time.Duration {
	if serverHint > 0 {
		return min(serverHint, p.MaxDelay)
	}
	d := min(p.BaseDelay<<attempt, p.MaxDelay)
	return d/2 + rand.N(d/2)
}

func withRetry[T any](ctx context.Context, policy RetryPolicy, operation func() (T, error)) (T, error) {
	var zero T
	for attempt := 0; ; attempt++ {
		v, err := operation()
		if err == nil {
			return v, nil
		}
		slog.Error("error calling graph API:", "err", err.Error())
		hint, retryable := retryHint(err)
		if !retryable || attempt >= policy.MaxRetries {
			return zero, err
		}
		slog.Debug("retrying operation", "attempt", attempt, "delay", policy.delay(attempt, hint))
		if err := sleep(ctx, policy.delay(attempt, hint)); err != nil {
			return zero, err
		}
	}
}
