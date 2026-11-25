package cmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rijojohn85/entra-expiry/internal/provider"
)

type fakeAppProvider struct {
	apps []provider.Application
	err  error
}

func (f fakeAppProvider) ListApplications(ctx context.Context) ([]provider.Application, error) {
	return f.apps, f.err
}

func TestScanRunPrintsValidCredential(t *testing.T) {
	p := fakeAppProvider{apps: []provider.Application{{
		ID:          "app-1",
		DisplayName: "billing-api",
		Credentials: []provider.Credential{{
			Type:    provider.Secret,
			Name:    "prod-secret",
			Expires: time.Now().AddDate(1, 0, 0),
		}},
	}}}
	var buf bytes.Buffer

	err := scanRun(context.Background(), p, &buf, 30)

	if err != nil {
		t.Fatalf("scanRun returned error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "billing-api") {
		t.Errorf("output missing app name:\n%s", out)
	}
	if !strings.Contains(out, "Valid") {
		t.Errorf("output missing Valid status:\n%s", out)
	}
}

func TestScanRunReturnsProviderError(t *testing.T) {
	want := errors.New("graph unavailable")
	p := fakeAppProvider{err: want}
	var buf bytes.Buffer

	err := scanRun(context.Background(), p, &buf, 30)

	if !errors.Is(err, want) {
		t.Fatalf("got error %v, want %v", err, want)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output on error, got:\n%s", buf.String())
	}
}
