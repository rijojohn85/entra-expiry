package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/rijojohn85/entra-expiry/internal/graph"
)

// local copy of the graph package's test credential: small, idiomatic, and it
// keeps the two packages' test seams independent.
type fakeCredential struct{}

func (fakeCredential) GetToken(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{Token: "test-token", ExpiresOn: time.Now().Add(time.Hour)}, nil
}

func TestParseApplicationsMergesCredentials(t *testing.T) {
	expires := time.Date(2026, 11, 24, 5, 27, 13, 0, time.UTC)
	in := []graph.GraphApplication{
		{
			AppID:       "a",
			DisplayName: "multi-cred app",
			PasswordCredentials: []graph.GraphPassword{
				{KeyID: "pw-1", DisplayName: "old secret", EndDateTime: expires},
				{KeyID: "pw-2", DisplayName: "new secret", EndDateTime: expires.Add(time.Hour)},
			},
			KeyCredentials: []graph.GraphKey{
				{KeyID: "cert-1", DisplayName: "cert", EndDateTime: expires},
			},
		},
	}
	got := parseApplications(in)
	if len(got) != 1 {
		t.Fatalf("apps = %d, want 1", len(got))
	}
	app := got[0]
	if app.ID != "a" || app.DisplayName != "multi-cred app" {
		t.Errorf("app identity mismatch: %+v", app)
	}
	if len(app.Credentials) != 3 {
		t.Fatalf("credentials = %d, want 3 (2 secrets + 1 cert)", len(app.Credentials))
	}
	// secrets first (passwordCredentials processed first), then certs
	if app.Credentials[0].Type != Secret || app.Credentials[0].KeyID != "pw-1" {
		t.Errorf("credentials[0] = %+v, want secret pw-1", app.Credentials[0])
	}
	if app.Credentials[2].Type != Cert {
		t.Errorf("credentials[2] = %+v, want cert", app.Credentials[2])
	}
	if !app.Credentials[1].Expires.Equal(expires.Add(time.Hour)) {
		t.Errorf("credentials[1].Expires = %v, want %v", app.Credentials[1].Expires, expires.Add(time.Hour))
	}
}

func TestParseApplicationsEmptyAndZeroCreds(t *testing.T) {
	got := parseApplications([]graph.GraphApplication{
		{AppID: "x", DisplayName: "no creds", PasswordCredentials: nil, KeyCredentials: nil},
	})
	if len(got) != 1 {
		t.Fatalf("apps = %d, want 1", len(got))
	}
	if len(got[0].Credentials) != 0 {
		t.Errorf("Credentials = %+v, want empty", got[0].Credentials)
	}
	if got := parseApplications(nil); got == nil || len(got) != 0 {
		t.Errorf("parseApplications(nil) = %v, want empty non-nil", got)
	}
}

// TestGraphProviderListApplications exercises the full provider path — HTTP,
// decode, mapping — against a server shaped like real Graph output, including
// the live-tenant oddities: fractional-second timestamps, null collections.
func TestGraphProviderListApplications(t *testing.T) {
	var hits int
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits == 1 {
			if !strings.Contains(r.URL.RawQuery, "passwordCredentials,keyCredentials") {
				t.Errorf("query missing credential $select: %q", r.URL.RawQuery)
			}
			// shape copied from the live tenant scan output
			fmt.Fprint(w, `{
				"value": [
					{"appId":"app-c","displayName":"fx-c-expiring-cert",
					 "passwordCredentials":null,
					 "keyCredentials":[{"keyId":"cert-1","displayName":"fx-c","endDateTime":"2027-01-08T05:24:25Z"}]},
					{"appId":"app-b","displayName":"fx-b-expiring-secret",
					 "passwordCredentials":[{"keyId":"pw-1","displayName":"fx-b","endDateTime":"2026-12-19T05:25:11Z"}],
					 "keyCredentials":[]}
				],
				"@odata.nextLink": "`+srv.URL+`/page2?`+"$skiptoken=x"+`"}`)
			return
		}
		fmt.Fprint(w, `{"value":[{"appId":"app-e","displayName":"fx-e-healthy","passwordCredentials":[{"keyId":"pw-e","endDateTime":"2027-06-12T05:25:15Z"}],"keyCredentials":null}]}`)
	}))
	defer srv.Close()

	p := &GraphProvider{
		GraphClient:     graph.NewGraphClient(srv.URL, fakeCredential{}, srv.Client()),
		applicationsURL: srv.URL + "/applications?$select=displayName,appId,passwordCredentials,keyCredentials&$top=999",
	}
	apps, err := p.ListApplications(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if hits != 2 {
		t.Errorf("hits = %d, want 2 (paging followed)", hits)
	}
	if len(apps) != 3 {
		t.Fatalf("apps = %d, want 3", len(apps))
	}
	byID := map[string]Application{}
	for _, a := range apps {
		byID[a.ID] = a
	}
	if len(byID["app-c"].Credentials) != 1 || byID["app-c"].Credentials[0].Type != Cert {
		t.Errorf("fx-c credentials = %+v, want 1 cert", byID["app-c"].Credentials)
	}
	if len(byID["app-b"].Credentials) != 1 || byID["app-b"].Credentials[0].Type != Secret {
		t.Errorf("fx-b credentials = %+v, want 1 secret", byID["app-b"].Credentials)
	}
	if len(byID["app-e"].Credentials) != 1 {
		t.Errorf("fx-e credentials = %+v, want 1", byID["app-e"].Credentials)
	}
	if want := time.Date(2027, 1, 8, 5, 24, 25, 0, time.UTC); !byID["app-c"].Credentials[0].Expires.Equal(want) {
		t.Errorf("fx-c expiry = %v, want %v", byID["app-c"].Credentials[0].Expires, want)
	}
}
