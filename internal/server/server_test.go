package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rijojohn85/entra-expiry/internal/provider"
	"github.com/rijojohn85/entra-expiry/internal/report"
)

type fakeAppProvider struct {
	apps []provider.Application
	err  error
}

func (f fakeAppProvider) ListApplications(ctx context.Context) ([]provider.Application, error) {
	return f.apps, f.err
}

func fixtureApps(now time.Time) []provider.Application {
	return []provider.Application{
		{ID: "a1", DisplayName: "expired-app", Credentials: []provider.Credential{
			{Type: provider.Secret, KeyID: "k1", Name: "old-secret", Expires: now.AddDate(-1, 0, 0)},
		}},
		{ID: "a2", DisplayName: "expiring-app", Credentials: []provider.Credential{
			{Type: provider.Cert, KeyID: "k2", Name: "soon-cert", Expires: now.AddDate(0, 0, 25)},
		}},
		{ID: "a3", DisplayName: "valid-app", Credentials: []provider.Credential{
			{Type: provider.Secret, KeyID: "k3", Name: "healthy-secret", Expires: now.AddDate(0, 0, 200)},
		}},
		{ID: "a4", DisplayName: "empty-app"},
	}
}

// newTestServer returns a server wired to a fake provider with an interval
// long enough that the ticker never fires during a test.
func newTestServer(t *testing.T, apps []provider.Application, err error) *Server {
	t.Helper()
	return NewServer(0, 30, time.Hour, fakeAppProvider{apps: apps, err: err})
}

func get(t *testing.T, handler http.HandlerFunc, path string) (*http.Response, []byte) {
	t.Helper()
	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Result(), rec.Body.Bytes()
}

func TestMetricsReportsSummaryCounts(t *testing.T) {
	s := newTestServer(t, fixtureApps(time.Now()), nil)
	s.refresh(context.Background())

	resp, body := get(t, s.metrics, "/metrics")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q, want application/json", ct)
	}

	var summary report.Summary
	if err := json.Unmarshal(body, &summary); err != nil {
		t.Fatalf("metrics body is not valid JSON: %v\n%s", err, body)
	}
	if summary.Scanned != 4 || summary.Total != 3 || summary.Expired != 1 || summary.Expiring != 1 || summary.Valid != 1 {
		t.Errorf("summary = %+v, want scanned 4, total 3, 1 expired / 1 expiring / 1 valid", summary)
	}
}

func TestReportOutputsSortedSnapshotWithoutRank(t *testing.T) {
	s := newTestServer(t, fixtureApps(time.Now()), nil)
	s.refresh(context.Background())

	resp, body := get(t, s.report, "/report")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	var snap Snapshot
	if err := json.Unmarshal(body, &snap); err != nil {
		t.Fatalf("report body is not valid JSON: %v\n%s", err, body)
	}
	if snap.Timestamp.IsZero() {
		t.Error("snapshot timestamp is zero, want the refresh time")
	}
	if len(snap.Rows) != 3 {
		t.Fatalf("got %d rows, want 3 (empty-app must not appear)", len(snap.Rows))
	}
	wantStatuses := []string{"Expired", "Expiring Soon", "Valid"}
	for i, want := range wantStatuses {
		if snap.Rows[i].Status != want {
			t.Errorf("row %d status = %q, want %q (worst-first order)", i, snap.Rows[i].Status, want)
		}
	}
	// Rank is an internal sort key and must not leak into the payload.
	if strings.Contains(string(body), `"rank"`) {
		t.Errorf("report payload leaks rank:\n%s", body)
	}
}

func TestMetricsWithoutSnapshotReturns503(t *testing.T) {
	s := newTestServer(t, nil, nil)

	resp, _ := get(t, s.metrics, "/metrics")
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 before the first successful scan", resp.StatusCode)
	}
}

func TestScanFailureKeepsPreviousSnapshot(t *testing.T) {
	s := newTestServer(t, fixtureApps(time.Now()), nil)
	s.refresh(context.Background())

	// Provider starts failing: the old snapshot must survive until a
	// later scan succeeds.
	s.apps = fakeAppProvider{err: errors.New("graph is down")}
	s.refresh(context.Background())

	_, body := get(t, s.metrics, "/metrics")
	var summary report.Summary
	if err := json.Unmarshal(body, &summary); err != nil {
		t.Fatalf("metrics body is not valid JSON: %v\n%s", err, body)
	}
	if summary.Scanned != 4 {
		t.Errorf("summary = %+v, want the previous snapshot (scanned 4)", summary)
	}
}

// TestConcurrentRefreshAndMetrics exists for the race detector: refresh
// swaps the snapshot while handlers read it.
func TestConcurrentRefreshAndMetrics(t *testing.T) {
	now := time.Now()
	s := newTestServer(t, fixtureApps(now), nil)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			s.refresh(context.Background())
		}()
		go func(i int) {
			defer wg.Done()
			resp, body := get(t, s.metrics, "/metrics")
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusServiceUnavailable {
				t.Errorf("unexpected status %d: %s", resp.StatusCode, body)
			}
		}(i)
	}
	wg.Wait()
}

func TestServeOnLocalHTTPServer(t *testing.T) {
	s := newTestServer(t, fixtureApps(time.Now()), nil)
	s.refresh(context.Background())

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthz)
	mux.HandleFunc("/metrics", s.metrics)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("healthz status = %d, want 200", resp.StatusCode)
	}

	resp, err = http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var summary report.Summary
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		t.Fatalf("metrics body is not valid JSON: %v", err)
	}
	if summary.Total != 3 {
		t.Errorf("total = %d, want 3", summary.Total)
	}
}
