package graph

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// statusScript serves the given statuses in order; the last status repeats
// forever. A handler cannot serve two statuses to one request, so each entry
// is consumed by exactly one call.
func statusScript(t *testing.T, statuses ...int) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		code := statuses[len(statuses)-1]
		if int(n) <= len(statuses) {
			code = statuses[n-1]
		}
		if code == http.StatusTooManyRequests {
			w.Header().Set("Retry-After", "1") // capped by test MaxDelay, fast
		}
		w.WriteHeader(code)
		fmt.Fprint(w, `{"error":{"code":"scripted"}}`)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func newTestClientForServer(t *testing.T, srv *httptest.Server) *GraphClient {
	t.Helper()
	c := newTestClient(t, srv.URL)
	c.httpClient = srv.Client()
	return c
}

func TestRetryRetriesOn429ThenSucceeds(t *testing.T) {
	srv, hits := statusScript(t, http.StatusTooManyRequests, http.StatusOK)
	c := newTestClientForServer(t, srv)

	page, err := c.fetchPage(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("want success after retry, got: %v", err)
	}
	if page.NextLink != "" || page.Value != nil {
		t.Logf("page = %+v", page)
	}
	if hits.Load() != 2 {
		t.Errorf("hits = %d, want 2 (one 429, one retry)", hits.Load())
	}
}

func TestRetryDoesNotRetry4xx(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusUnauthorized, http.StatusBadRequest, http.StatusNotFound} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			srv, hits := statusScript(t, status)
			c := newTestClientForServer(t, srv)

			_, err := c.fetchPage(context.Background(), srv.URL)
			if err == nil {
				t.Fatal("want error, got nil")
			}
			if hits.Load() != 1 {
				t.Errorf("status %d retried: hits = %d, want exactly 1", status, hits.Load())
			}
		})
	}
}

func TestRetryGivesUpAfterMaxRetries(t *testing.T) {
	srv, hits := statusScript(t, http.StatusTooManyRequests) // forever
	c := newTestClientForServer(t, srv)
	c.retryPolicy.MaxRetries = 2

	_, err := c.fetchPage(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("want error after retries exhausted, got nil")
	}
	var apiErr APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusTooManyRequests {
		t.Errorf("want APIError 429, got %v", err)
	}
	if hits.Load() != 3 { // 1 initial + 2 retries
		t.Errorf("hits = %d, want 3", hits.Load())
	}
}

func TestRetryRetriesOn5xx(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			srv, hits := statusScript(t, status, http.StatusOK)
			c := newTestClientForServer(t, srv)

			_, err := c.fetchPage(context.Background(), srv.URL)
			if err != nil {
				t.Fatalf("want success after retry, got: %v", err)
			}
			if hits.Load() != 2 {
				t.Errorf("hits = %d, want 2", hits.Load())
			}
		})
	}
}

func TestRetryHonorsServerHint(t *testing.T) {
	p := RetryPolicy{MaxRetries: 3, BaseDelay: 100 * time.Millisecond, MaxDelay: 10 * time.Second}

	// server hint wins over backoff
	if got := p.delay(0, 3*time.Second); got != 3*time.Second {
		t.Errorf("delay(0, 3s) = %v, want 3s", got)
	}
	// ...but never above MaxDelay
	if got := p.delay(0, time.Hour); got != p.MaxDelay {
		t.Errorf("delay(0, 1h) = %v, want %v", got, p.MaxDelay)
	}
	// zero hint → exponential backoff with jitter, within bounds
	for attempt := range 3 {
		max := min(p.BaseDelay<<attempt, p.MaxDelay)
		got := p.delay(attempt, 0)
		if got < max/2 || got >= max {
			t.Errorf("delay(%d, 0) = %v, want in [%v, %v)", attempt, got, max/2, max)
		}
	}
}

func TestRetryContextCancelDuringSleep(t *testing.T) {
	srv, _ := statusScript(t, http.StatusTooManyRequests) // forever
	c := newTestClientForServer(t, srv)
	c.retryPolicy = RetryPolicy{MaxRetries: 5, BaseDelay: time.Hour, MaxDelay: time.Hour}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := c.fetchPage(ctx, srv.URL)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("want context.DeadlineExceeded, got %v", err)
	}
}
