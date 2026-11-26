package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/rijojohn85/entra-expiry/internal/provider"
)

func TestReportClassifiesCredentialStatuses(t *testing.T) {
	now := time.Now()
	apps := []provider.Application{
		{
			ID:          "app-1",
			DisplayName: "expired-app",
			Credentials: []provider.Credential{
				{Type: provider.Secret, KeyID: "k1", Name: "old-secret", Expires: now.AddDate(-1, 0, 0)},
			},
		},
		{
			ID:          "app-2",
			DisplayName: "expiring-app",
			Credentials: []provider.Credential{
				{Type: provider.Cert, KeyID: "k2", Name: "soon-cert", Expires: now.AddDate(0, 0, 25)},
			},
		},
		{
			ID:          "app-3",
			DisplayName: "valid-app",
			Credentials: []provider.Credential{
				{Type: provider.Secret, KeyID: "k3", Name: "healthy-secret", Expires: now.AddDate(0, 0, 200)},
			},
		},
		{ID: "app-4", DisplayName: "empty-app"},
	}

	var buf bytes.Buffer
	NewReport(apps, 30, &buf).Generate()
	out := buf.String()

	lower := strings.ToLower(out)
	for _, want := range []string{
		"AppName", "Cred Kind", "Name", "Expires", "IN(days)", "STATUS",
		"expired-app", "old-secret", "Expired",
		"expiring-app", "soon-cert", "Cert", "Expiring Soon",
		"valid-app", "healthy-secret", "Valid",
	} {
		if !strings.Contains(lower, strings.ToLower(want)) {
			t.Errorf("report missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "empty-app") {
		t.Errorf("app with no credentials should not appear in the report:\n%s", out)
	}
}

func TestClassifyWindowBoundary(t *testing.T) {
	now := time.Now()

	status, rank := classify(now.AddDate(0, 0, 29), now, 30)
	if status != "Expiring Soon" || rank != 1 {
		t.Errorf("29 days out = (%q, %d), want (Expiring Soon, 1)", status, rank)
	}
	// Exactly at the boundary counts as Valid — Before is strict.
	status, rank = classify(now.Add(time.Duration(30) * 24 * time.Hour), now, 30)
	if status != "Valid" || rank != 2 {
		t.Errorf("exactly at boundary = (%q, %d), want (Valid, 2)", status, rank)
	}
}

func TestBuildClassifiesCountsAndSorts(t *testing.T) {
	now := time.Now()
	apps := []provider.Application{
		{ID: "a1", DisplayName: "valid-app", Credentials: []provider.Credential{
			{Type: provider.Secret, KeyID: "k1", Name: "healthy-secret", Expires: now.AddDate(0, 0, 200)},
		}},
		{ID: "a2", DisplayName: "expired-app", Credentials: []provider.Credential{
			{Type: provider.Cert, KeyID: "k2", Name: "old-cert", Expires: now.AddDate(-1, 0, 0)},
		}},
		{ID: "a3", DisplayName: "expiring-app", Credentials: []provider.Credential{
			{Type: provider.Secret, KeyID: "k3", Name: "soon-secret", Expires: now.AddDate(0, 0, 25)},
		}},
		{ID: "a4", DisplayName: "empty-app"},
	}

	rows, s := build(apps, 30, now)

	if s.Scanned != 4 || s.Total != 3 || s.Expired != 1 || s.Expiring != 1 || s.Valid != 1 {
		t.Errorf("summary = %+v, want scanned 4, total 3, 1 expired / 1 expiring / 1 valid", s)
	}
	wantOrder := []string{"old-cert", "soon-secret", "healthy-secret"}
	for i, want := range wantOrder {
		if rows[i].name != want {
			t.Errorf("row %d = %q, want %q (worst-first order)", i, rows[i].name, want)
		}
	}
}

func TestJSONReportOutputsParseablePayload(t *testing.T) {
	now := time.Now()
	apps := []provider.Application{
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

	var buf bytes.Buffer
	if err := NewJSONReport(apps, 30, &buf).Generate(); err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// Unmarshalling into time.Time enforces RFC3339, so a wrong Expires
	// format (e.g. Go's String() output) fails right here.
	var payload struct {
		Scanned     int `json:"scanned"`
		Total       int `json:"total"`
		Expired     int `json:"expired"`
		Expiring    int `json:"expiring"`
		Valid       int `json:"valid"`
		Credentials []struct {
			App          string    `json:"app"`
			Kind         string    `json:"kind"`
			Name         string    `json:"name"`
			Expires      time.Time `json:"expires"`
			Status       string    `json:"status"`
			DaysToExpire int       `json:"days_to_expire"`
		} `json:"credentials"`
	}
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if payload.Scanned != 4 || payload.Total != 3 || payload.Expired != 1 || payload.Expiring != 1 || payload.Valid != 1 {
		t.Errorf("counts = scanned %d, total %d, %d/%d/%d, want 4, 3, 1/1/1",
			payload.Scanned, payload.Total, payload.Expired, payload.Expiring, payload.Valid)
	}
	if len(payload.Credentials) != 3 {
		t.Fatalf("got %d credentials, want 3 (empty-app must not appear)", len(payload.Credentials))
	}
	first, last := payload.Credentials[0], payload.Credentials[2]
	if first.Status != "Expired" || first.App != "expired-app" || first.Kind != "Secret" {
		t.Errorf("first credential = %+v, want worst-first expired secret", first)
	}
	if last.Status != "Valid" || last.App != "valid-app" || last.Kind != "Secret" {
		t.Errorf("last credential = %+v, want valid secret last", last)
	}
	if _, err := time.Parse(time.RFC3339, first.Expires.Format(time.RFC3339)); err != nil {
		t.Errorf("expires not RFC3339: %v", first.Expires)
	}
}

func TestReportRespectsDaysWindow(t *testing.T) {
	now := time.Now()
	apps := []provider.Application{
		{
			ID:          "app-1",
			DisplayName: "boundary-app",
			Credentials: []provider.Credential{
				{Type: provider.Secret, KeyID: "k1", Name: "inside-window", Expires: now.AddDate(0, 0, 29)},
				{Type: provider.Secret, KeyID: "k2", Name: "outside-window", Expires: now.AddDate(0, 0, 31)},
			},
		},
	}

	var buf bytes.Buffer
	NewReport(apps, 30, &buf).Generate()
	out := buf.String()

	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.Contains(line, "inside-window"):
			if !strings.Contains(line, "Expiring Soon") {
				t.Errorf("credential 29 days out with --days 30 should be Expiring Soon, got: %s", line)
			}
		case strings.Contains(line, "outside-window"):
			if !strings.Contains(line, "Valid") {
				t.Errorf("credential 31 days out with --days 30 should be Valid, got: %s", line)
			}
		}
	}
}
