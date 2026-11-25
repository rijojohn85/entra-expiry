package report

import (
	"bytes"
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
