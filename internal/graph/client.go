package graph

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

type GraphClient struct {
	scopeURL   string
	credential *azidentity.DefaultAzureCredential
	token      string
	httpClient *http.Client
}

func NewGraphClient(ctx context.Context, baseURL string, httpClient *http.Client) (*GraphClient, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}
	token, err := cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{baseURL},
	})
	if err != nil {
		slog.Error("Failed to get token", "error", err)
		return nil, err
	}
	slog.Debug("Successfully obtained Azure credential and token")
	return &GraphClient{
		scopeURL:   baseURL,
		credential: cred,
		token:      token.Token,
		httpClient: httpClient,
	}, nil
}

func (c *GraphClient) DoRequest(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	return c.httpClient.Do(req)
}
