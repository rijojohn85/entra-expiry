package graph

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

// fakeCredential satisfies azcore.TokenCredential without any network calls,
// so tests run in CI with no az login.
type fakeCredential struct{}

func (fakeCredential) GetToken(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{Token: "test-token", ExpiresOn: time.Now().Add(time.Hour)}, nil
}

// newTestClient returns a GraphClient pointed at the given base URL, with a
// fast retry policy so retry tests don't sleep meaningfully.
func newTestClient(t *testing.T, serverURL string) *GraphClient {
	t.Helper()
	httpClient := &http.Client{Timeout: 5 * time.Second}
	if serverURL == "" {
		httpClient = nil
	}
	return &GraphClient{
		scopeURL:   serverURL,
		credential: fakeCredential{},
		httpClient: httpClient,
		retryPolicy: RetryPolicy{
			MaxRetries: 2,
			BaseDelay:  1 * time.Millisecond,
			MaxDelay:   5 * time.Millisecond,
		},
	}
}

func TestGetRetryAfter(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   time.Duration
	}{
		{"seconds value", "10", 10 * time.Second},
		{"fractional seconds", "1.5", 1500 * time.Millisecond},
		{"zero", "0", 0},
		{"missing", "", 0},
		{"malformed", "not-a-duration", 0}, // falls back to backoff, per docs
		{"http-date form", "Wed, 25 Nov 2026 10:00:00 GMT", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{Header: http.Header{}}
			if tt.header != "" {
				resp.Header.Set("Retry-After", tt.header)
			}
			if got := getRetryAfter(resp); got != tt.want {
				t.Errorf("getRetryAfter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDoRequestNonOKReturnsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":{"code":"Authorization_RequestDenied","message":"insufficient privileges"}}`)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	req, err := c.createGetRequest(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.doRequest(req)

	var apiErr APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("want APIError, got %T: %v", err, err)
	}
	if apiErr.Status != http.StatusForbidden {
		t.Errorf("Status = %d, want %d", apiErr.Status, http.StatusForbidden)
	}
	if !strings.Contains(apiErr.Error(), "Authorization_RequestDenied") {
		t.Errorf("error should carry the Graph error body, got: %v", apiErr)
	}
	// 403 must not be retryable
	if _, ok := retryHint(err); ok {
		t.Error("403 must not be retryable")
	}
}

func TestAPIErrorUnwrap(t *testing.T) {
	inner := fmt.Errorf("request failed with status: 429")
	apiErr := NewAPIError(429, "https://graph.microsoft.com/v1.0/applications", inner, 0)

	// Unwrap must expose the inner error through the chain
	if !errors.Is(apiErr, inner) {
		t.Error("errors.Is should find the inner error through Unwrap")
	}
	// ...and errors.As must find APIError when it is itself wrapped later
	wrapped := fmt.Errorf("fetching page 2: %w", apiErr)
	var back APIError
	if !errors.As(wrapped, &back) || back.Status != 429 {
		t.Error("errors.As should recover APIError through %w wrapping")
	}
}

func TestDoRequestWithPagingMultiPage(t *testing.T) {
	var hits int
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Errorf("request %d: missing Authorization header", hits)
		}
		switch hits {
		case 1:
			// nextLink must be followed verbatim, query params intact
			fmt.Fprintf(w, `{"value":[{"appId":"app-1","displayName":"a"}],"@odata.nextLink":%q}`,
				srv.URL+"/v1.0/applications?$select=displayName&$top=999&$skiptoken=abc")
		case 2:
			if r.URL.RawQuery != "$select=displayName&$top=999&$skiptoken=abc" {
				t.Errorf("page 2 raw query = %q, want nextLink params preserved", r.URL.RawQuery)
			}
			fmt.Fprint(w, `{"value":[{"appId":"app-2","displayName":"b"}]}`)
		default:
			t.Errorf("unexpected request %d — paging loop did not stop", hits)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	apps, err := c.DoRequestWithPaging(context.Background(), srv.URL+"/v1.0/applications")
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 2 || apps[0].AppID != "app-1" || apps[1].AppID != "app-2" {
		t.Errorf("apps = %+v, want 2 apps in order", apps)
	}
	if hits != 2 {
		t.Errorf("hits = %d, want 2", hits)
	}
}

func TestDoRequestWithPagingNullCredentials(t *testing.T) {
	// Graph sends explicit nulls for credential collections on credential-less apps
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"value":[{"appId":"app-1","displayName":"nocreds","passwordCredentials":null,"keyCredentials":null}]}`)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	apps, err := c.DoRequestWithPaging(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 {
		t.Fatalf("apps = %+v, want 1", apps)
	}
	if apps[0].PasswordCredentials != nil || apps[0].KeyCredentials != nil {
		t.Errorf("null collections should decode to nil, got %+v", apps[0])
	}
}

func TestDecodeFractionalSeconds(t *testing.T) {
	// real Graph timestamps carry 7-digit fractions (jopackblr from the live tenant)
	body := `{"value":[{"appId":"x","displayName":"jopackblr","passwordCredentials":[{"keyId":"k","endDateTime":"2022-06-03T22:42:00.7023813Z"}]}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	apps, err := c.DoRequestWithPaging(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2022, 6, 3, 22, 42, 0, 702381300, time.UTC)
	if !apps[0].PasswordCredentials[0].EndDateTime.Equal(want) {
		t.Errorf("EndDateTime = %v, want %v", apps[0].PasswordCredentials[0].EndDateTime, want)
	}
}

func TestDoRequestWithPagingStatusError(t *testing.T) {
	// A 401 on page 1 must surface as an error, not decode into an empty result
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"code":"InvalidAuthenticationToken"}}`)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.DoRequestWithPaging(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("want error for 401, got nil")
	}
	var apiErr APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusUnauthorized {
		t.Errorf("want APIError 401, got %v", err)
	}
	if hits != 1 {
		t.Errorf("401 must not be retried, hits = %d", hits)
	}
}
