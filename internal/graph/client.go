package graph

import (
	"context"
	"encoding/json"
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

func (c *GraphClient) DoInitialRequest(ctx context.Context, url string) (*http.Response, error) {
	req, err := c.CreateGetRequest(ctx, url)
	if err != nil {
		return nil, err
	}
	return c.DoRequest(req)
}

func (c *GraphClient) CreateGetRequest(ctx context.Context, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	return req, nil
}

func (c *GraphClient) DoRequest(req *http.Request) (*http.Response, error) {
	return c.httpClient.Do(req)
}

func (c *GraphClient) DoRequestWithPaging(ctx context.Context, url string) ([]GraphApplication, error) {
	apps := []GraphApplication{}
	res, err := c.DoInitialRequest(ctx, url)
	if err != nil {
		return nil, err
	}
	var appResponse ApplicationsPage
	err = json.NewDecoder(res.Body).Decode(&appResponse)
	if err != nil {
		return nil, err
	}
	res.Body.Close()
	apps = append(apps, appResponse.Value...)
	for appResponse.NextLink != "" {
		req, err := c.CreateGetRequest(ctx, appResponse.NextLink)
		if err != nil {
			return nil, err
		}
		res, err = c.DoRequest(req)
		if err != nil {
			return nil, err
		}
		err = json.NewDecoder(res.Body).Decode(&appResponse)
		if err != nil {
			return nil, err
		}
		apps = append(apps, appResponse.Value...)
		res.Body.Close()
	}

	return apps, nil
}
