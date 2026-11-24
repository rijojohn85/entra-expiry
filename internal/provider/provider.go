package provider

import (
	"context"
	"net/http"

	"github.com/rijojohn85/entra-expiry/internal/graph"
)

type GraphProvider struct {
	GraphClient *graph.GraphClient
}

func NewGraphProvider(ctx context.Context, scopeURL string, httpClient *http.Client) (*GraphProvider, error) {
	client, err := graph.NewGraphClient(ctx, scopeURL, httpClient)
	if err != nil {
		return nil, err
	}
	return &GraphProvider{
		GraphClient: client,
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
	graphApps, err := p.GraphClient.DoRequestWithPaging(ctx, "https://graph.microsoft.com/v1.0/applications?$select=displayName,appId,passwordCredentials,keyCredentials&$top=100")
	if err != nil {
		return nil, err
	}
	return parseApplications(graphApps), nil
}
