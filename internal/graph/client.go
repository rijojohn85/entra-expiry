package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

type GraphClient struct {
	scopeURL    string
	credential  azcore.TokenCredential
	token       string
	httpClient  *http.Client
	retryPolicy RetryPolicy
}

// NewGraphClient builds a client for the Microsoft Graph API.
// The credential is injected so callers (and tests) control authentication.
func NewGraphClient(scopeURL string, credential azcore.TokenCredential, httpClient *http.Client) *GraphClient {
	return &GraphClient{
		scopeURL:   scopeURL,
		credential: credential,
		httpClient: httpClient,
		retryPolicy: RetryPolicy{
			MaxRetries: 3,
			BaseDelay:  1 * time.Second,
			MaxDelay:   10 * time.Second,
		},
	}
}

func (c *GraphClient) createGetRequest(ctx context.Context, url string) (*http.Request, error) {
	token, err := c.credential.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{c.scopeURL},
	})
	if err != nil {
		return nil, err
	}
	c.token = token.Token
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	return req, nil
}

func (c *GraphClient) doRequest(req *http.Request) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		// Read the response body for the Graph error details, and keep the
		// URL in the error so logs say *which* endpoint failed.
		body := readResponseBody(resp)
		retryAfter := getRetryAfter(resp)
		return nil, NewAPIError(resp.StatusCode, req.URL.String(),
			fmt.Errorf("request failed with status: %s, body: %s", resp.Status, body), retryAfter)
	}
	return resp, nil
}

func readResponseBody(resp *http.Response) string {
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return string(body)
}

func getRetryAfter(resp *http.Response) time.Duration {
	retryAfter := time.Duration(0)
	if resp.Header.Get("Retry-After") != "" {
		if ra, err := time.ParseDuration(resp.Header.Get("Retry-After") + "s"); err == nil {
			retryAfter = ra
		}
	}
	return retryAfter
}

func (c *GraphClient) DoRequestWithPaging(ctx context.Context, url string) ([]GraphApplication, error) {
	apps := []GraphApplication{}
	page, err := c.fetchPage(ctx, url)
	if err != nil {
		return nil, err
	}
	apps = append(apps, page.Value...)
	for page.NextLink != "" {
		page, err = c.fetchPage(ctx, page.NextLink)
		if err != nil {
			return nil, err
		}
		apps = append(apps, page.Value...)
	}

	return apps, nil
}

// getPage GETs one page of results and decodes it. The response body is
// always closed before returning — defers inside a paging loop would keep
// every page's connection open until the outer function returns.
func (c *GraphClient) getPage(ctx context.Context, url string) (ApplicationsPage, error) {
	var page ApplicationsPage
	req, err := c.createGetRequest(ctx, url)
	if err != nil {
		return page, err
	}
	res, err := c.doRequest(req)
	if err != nil {
		return page, err
	}
	defer res.Body.Close()
	if err := json.NewDecoder(res.Body).Decode(&page); err != nil {
		return page, fmt.Errorf("decode page %q: %w", url, err)
	}
	return page, nil
}

// fetchPage wraps getPage in the retry policy: transient failures
// (429 / 5xx) are retried with backoff, honoring the server's Retry-After.
func (c *GraphClient) fetchPage(ctx context.Context, url string) (ApplicationsPage, error) {
	return withRetry(ctx, c.retryPolicy, func() (ApplicationsPage, error) {
		return c.getPage(ctx, url)
	})
}
