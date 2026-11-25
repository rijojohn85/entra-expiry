package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/rijojohn85/entra-expiry/internal/graph"
)

type GraphProvider struct {
	GraphClient *graph.GraphClient
	// applicationsURL is the Graph endpoint for listing applications;
	// tests override it to point at a local test server.
	applicationsURL string
}

const defaultApplicationsURL = "https://graph.microsoft.com/v1.0/applications?$select=displayName,appId,passwordCredentials,keyCredentials&$top=999"

func NewGraphProvider(scopeURL string, httpClient *http.Client) (*GraphProvider, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("build azure credential: %w", err)
	}
	client := graph.NewGraphClient(scopeURL, cred, httpClient)
	return &GraphProvider{
		GraphClient:     client,
		applicationsURL: defaultApplicationsURL,
	}, nil
}

func parseApplications(graphApps []graph.GraphApplication) []Application {
	apps := make([]Application, 0, len(graphApps))
	for _, ga := range graphApps {
		creds := make([]Credential, 0, len(ga.PasswordCredentials)+len(ga.KeyCredentials))
		for _, pc := range ga.PasswordCredentials {
			creds = append(creds, Credential{
				Type:    Secret,
				KeyID:   pc.KeyID,
				Name:    pc.DisplayName,
				Expires: pc.EndDateTime,
			})
		}
		for _, kc := range ga.KeyCredentials {
			creds = append(creds, Credential{
				Type:    Cert,
				KeyID:   kc.KeyID,
				Name:    kc.DisplayName,
				Expires: kc.EndDateTime,
			})
		}
		apps = append(apps, Application{
			ID:          ga.AppID,
			DisplayName: ga.DisplayName,
			Credentials: creds,
		})
	}
	return apps
}

func (p *GraphProvider) ListApplications(ctx context.Context) ([]Application, error) {
	graphApps, err := p.GraphClient.DoRequestWithPaging(ctx, p.applicationsURL)
	if err != nil {
		return nil, fmt.Errorf("list applications: %w", err)
	}
	return parseApplications(graphApps), nil
}
